package downloader_test

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/downloader"
)

func computeSHA1(data []byte) string {
	h := sha1.Sum(data)
	return hex.EncodeToString(h[:])
}

func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestDownloader_ParallelDownloadWithChecksum(t *testing.T) {
	tempDir := t.TempDir()

	fileData := make(map[string][]byte)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file_%d.jar", i)
		data := []byte(fmt.Sprintf("content_of_minecraft_asset_%d_with_padding_data_%s", i, strings.Repeat("x", 5000)))
		fileData[name] = data
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		data, exists := fileData[name]
		if !exists {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer server.Close()

	cfg := downloader.DefaultConfig()
	cfg.MaxWorkers = 4
	disp := downloader.NewDispatcher(cfg, server.Client())

	var tasks []*downloader.DownloadTask
	for name, data := range fileData {
		tasks = append(tasks, &downloader.DownloadTask{
			ID:             name,
			URL:            server.URL + "/" + name,
			DestPath:       filepath.Join(tempDir, name),
			ExpectedSHA1:   computeSHA1(data),
			ExpectedSHA256: computeSHA256(data),
			ExpectedSize:   int64(len(data)),
			Priority:       downloader.PriorityNormal,
		})
	}

	var progressReports []downloader.BatchProgress
	var mu sync.Mutex

	err := disp.DownloadBatch(context.Background(), tasks, func(bp downloader.BatchProgress) {
		mu.Lock()
		progressReports = append(progressReports, bp)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("DownloadBatch failed: %v", err)
	}

	// Verify all files written correctly and match hash
	for name, expectedData := range fileData {
		dest := filepath.Join(tempDir, name)
		content, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("failed to read downloaded file %s: %v", name, err)
		}
		if string(content) != string(expectedData) {
			t.Fatalf("file %s content mismatch", name)
		}
	}

	// Verify progress was recorded
	mu.Lock()
	defer mu.Unlock()
	if len(progressReports) == 0 {
		t.Fatal("expected at least one progress update")
	}
	last := progressReports[len(progressReports)-1]
	if last.CompletedTasks != 10 {
		t.Fatalf("expected 10 completed tasks, got %d", last.CompletedTasks)
	}
	if last.FailedTasks != 0 {
		t.Fatalf("expected 0 failed tasks, got %d", last.FailedTasks)
	}
}

func TestDownloader_RangeResumeAfterDisconnect(t *testing.T) {
	tempDir := t.TempDir()
	fullContent := []byte(strings.Repeat("0123456789ABCDEF", 1000)) // 16000 bytes
	sha1Expected := computeSHA1(fullContent)

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		rangeHeader := r.Header.Get("Range")

		if count == 1 {
			// First request: simulate disconnect after delivering first 4000 bytes
			w.Header().Set("Content-Length", strconv.Itoa(len(fullContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fullContent[:4000])
			// Close/abort connection abruptly by not sending remaining bytes
			return
		}

		// Second request: must have Range: bytes=4000-
		if !strings.HasPrefix(rangeHeader, "bytes=4000-") {
			t.Errorf("expected Range header bytes=4000-, got %q", rangeHeader)
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes 4000-%d/%d", len(fullContent)-1, len(fullContent)))
		w.Header().Set("Content-Length", strconv.Itoa(len(fullContent)-4000))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(fullContent[4000:])
	}))
	defer server.Close()

	cfg := downloader.DefaultConfig()
	cfg.MaxRetries = 2
	cfg.BaseBackoff = 20 * time.Millisecond
	disp := downloader.NewDispatcher(cfg, server.Client())

	destFile := filepath.Join(tempDir, "resumable.bin")
	task := &downloader.DownloadTask{
		ID:           "resume-task",
		URL:          server.URL + "/resumable",
		DestPath:     destFile,
		ExpectedSHA1: sha1Expected,
		ExpectedSize: int64(len(fullContent)),
	}

	err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task}, nil)
	if err != nil {
		t.Fatalf("DownloadBatch with resume failed: %v", err)
	}

	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if len(data) != len(fullContent) {
		t.Fatalf("expected length %d, got %d", len(fullContent), len(data))
	}
	if string(data) != string(fullContent) {
		t.Fatal("content mismatch after range resumption")
	}

	if requestCount.Load() < 2 {
		t.Fatalf("expected at least 2 requests (initial + resume), got %d", requestCount.Load())
	}
}

func TestDownloader_MirrorFailover(t *testing.T) {
	tempDir := t.TempDir()
	expectedContent := []byte("content_from_healthy_backup_mirror")
	sha1Val := computeSHA1(expectedContent)

	// Broken primary server (HTTP 500)
	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer brokenServer.Close()

	// Healthy mirror
	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(expectedContent)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedContent)
	}))
	defer mirrorServer.Close()

	cfg := downloader.DefaultConfig()
	cfg.MaxRetries = 1
	cfg.BaseBackoff = 10 * time.Millisecond
	disp := downloader.NewDispatcher(cfg, mirrorServer.Client())

	destPath := filepath.Join(tempDir, "mirrored.jar")
	task := &downloader.DownloadTask{
		ID:           "mirror-test",
		URL:          brokenServer.URL + "/client.jar",
		Mirrors:      []string{mirrorServer.URL + "/client.jar"},
		DestPath:     destPath,
		ExpectedSHA1: sha1Val,
	}

	err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task}, nil)
	if err != nil {
		t.Fatalf("expected mirror failover to succeed, got error: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read mirrored file: %v", err)
	}
	if string(data) != string(expectedContent) {
		t.Fatalf("expected %q, got %q", expectedContent, data)
	}
}

func TestDownloader_CorruptedHashDetectionAndCleanup(t *testing.T) {
	tempDir := t.TempDir()
	corruptPayload := []byte("completely_wrong_bytes")
	expectedHash := "0123456789abcdef0123456789abcdef01234567" // Mismatch

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(corruptPayload)
	}))
	defer server.Close()

	cfg := downloader.DefaultConfig()
	cfg.MaxRetries = 0 // Fail immediately without retry
	disp := downloader.NewDispatcher(cfg, server.Client())

	destPath := filepath.Join(tempDir, "corrupt.jar")
	task := &downloader.DownloadTask{
		ID:           "corrupt-test",
		URL:          server.URL + "/file",
		DestPath:     destPath,
		ExpectedSHA1: expectedHash,
	}

	err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task}, nil)
	if err == nil {
		t.Fatal("expected error due to hash mismatch, got nil")
	}

	// Verify that neither final nor .part file remained on disk
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Fatal("expected destination file to NOT exist")
	}
	if _, err := os.Stat(destPath + ".part"); !os.IsNotExist(err) {
		t.Fatal("expected .part file to be purged after checksum mismatch")
	}
}

func TestDownloader_ExistingFileCacheBypass(t *testing.T) {
	tempDir := t.TempDir()
	existingContent := []byte("cached_minecraft_library_v1")
	destPath := filepath.Join(tempDir, "cached_lib.jar")
	if err := os.WriteFile(destPath, existingContent, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	var hitCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		_, _ = w.Write([]byte("different_content"))
	}))
	defer server.Close()

	cfg := downloader.DefaultConfig()
	disp := downloader.NewDispatcher(cfg, server.Client())

	task := &downloader.DownloadTask{
		ID:           "cached-task",
		URL:          server.URL + "/lib",
		DestPath:     destPath,
		ExpectedSHA1: computeSHA1(existingContent),
		ExpectedSize: int64(len(existingContent)),
	}

	err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task}, nil)
	if err != nil {
		t.Fatalf("expected bypass, got: %v", err)
	}

	if hitCount.Load() != 0 {
		t.Fatalf("expected 0 HTTP requests due to cache hit, got %d", hitCount.Load())
	}
}

func TestDownloader_PerHostConcurrencyLimiting(t *testing.T) {
	tempDir := t.TempDir()
	maxConns := 2
	var currentActive atomic.Int32
	var maxObserved atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := currentActive.Add(1)
		for {
			oldMax := maxObserved.Load()
			if cur <= oldMax || maxObserved.CompareAndSwap(oldMax, cur) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond) // Hold connection open
		currentActive.Add(-1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	cfg := downloader.DefaultConfig()
	cfg.MaxWorkers = 10
	cfg.MaxConnsPerHost = maxConns
	disp := downloader.NewDispatcher(cfg, server.Client())

	var tasks []*downloader.DownloadTask
	for i := 0; i < 8; i++ {
		tasks = append(tasks, &downloader.DownloadTask{
			ID:       fmt.Sprintf("limit-%d", i),
			URL:      server.URL + fmt.Sprintf("/file_%d", i),
			DestPath: filepath.Join(tempDir, fmt.Sprintf("limit_%d.bin", i)),
		})
	}

	err := disp.DownloadBatch(context.Background(), tasks, nil)
	if err != nil {
		t.Fatalf("DownloadBatch failed: %v", err)
	}

	if maxObserved.Load() > int32(maxConns) {
		t.Fatalf("exceeded maxConnsPerHost! Expected <= %d, observed %d", maxConns, maxObserved.Load())
	}
}

func TestPriorityQueue_HeapOperations(t *testing.T) {
	var pq downloader.TaskPriorityQueue
	now := time.Now()

	t1 := &downloader.DownloadTask{ID: "low", Priority: downloader.PriorityLow, CreatedAt: now}
	t2 := &downloader.DownloadTask{ID: "high", Priority: downloader.PriorityHigh, CreatedAt: now}
	t3 := &downloader.DownloadTask{ID: "critical", Priority: downloader.PriorityCritical, CreatedAt: now}
	t4 := &downloader.DownloadTask{ID: "high-later", Priority: downloader.PriorityHigh, CreatedAt: now.Add(1 * time.Second)}

	pq.Push(t1)
	pq.Push(t2)
	pq.Push(t3)
	pq.Push(t4)

	if pq.Len() != 4 {
		t.Fatalf("expected len 4, got %d", pq.Len())
	}

	// Test Less
	// t3 (critical) vs t2 (high)
	if !pq.Less(2, 1) { // index 2 is critical, index 1 is high
		t.Errorf("critical should be less (higher priority) than high")
	}
	// t2 (high, earlier) vs t4 (high, later)
	if !pq.Less(1, 3) {
		t.Errorf("earlier task should have priority over later task with same priority")
	}

	popped := pq.Pop().(*downloader.DownloadTask)
	if popped.ID != "high-later" {
		t.Errorf("expected high-later from raw slice pop, got %s", popped.ID)
	}
}

func TestDispatcher_DefaultConfigFallbacks(t *testing.T) {
	disp := downloader.NewDispatcher(downloader.Config{}, nil)
	if disp == nil {
		t.Fatalf("expected non-nil dispatcher")
	}

	// Invalid URL test in download
	tempDir := t.TempDir()
	task := &downloader.DownloadTask{
		ID:       "bad-url",
		URL:      "://invalid-url",
		DestPath: filepath.Join(tempDir, "bad.bin"),
	}
	err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task}, nil)
	if err == nil {
		t.Fatalf("expected error on invalid URL, got nil")
	}

	// Server 500 error test
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	failTask := &downloader.DownloadTask{
		ID:       "fail-500",
		URL:      server.URL + "/error",
		DestPath: filepath.Join(tempDir, "fail.bin"),
	}
	err = disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{failTask}, nil)
	if err == nil {
		t.Fatalf("expected error on 500 download, got nil")
	}

	// Empty batch test
	if err := disp.DownloadBatch(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected nil on empty task list, got %v", err)
	}

	// Existing file without hash test
	noHashFile := filepath.Join(tempDir, "nohash.bin")
	_ = os.WriteFile(noHashFile, []byte("some-data"), 0644)
	noHashTask := &downloader.DownloadTask{
		ID:           "nohash-task",
		URL:          "http://example.com/file",
		DestPath:     noHashFile,
		ExpectedSize: 9, // len("some-data")
	}
	if err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{noHashTask}, nil); err != nil {
		t.Fatalf("expected skipped download for matching size without hash: %v", err)
	}

	// Test Status 403 / unexpected code
	forbiddenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbiddenServer.Close()

	forbiddenTask := &downloader.DownloadTask{
		ID:       "forbidden",
		URL:      forbiddenServer.URL,
		DestPath: filepath.Join(tempDir, "forbidden.bin"),
	}
	if err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{forbiddenTask}, nil); err == nil {
		t.Fatalf("expected error on 403 response, got nil")
	}

	// Test Checksum mismatch failure
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("some-corrupt-data"))
	}))
	defer dataServer.Close()

	badChecksumTask := &downloader.DownloadTask{
		ID:             "bad-checksum",
		URL:            dataServer.URL,
		DestPath:       filepath.Join(tempDir, "bad-checksum.bin"),
		ExpectedSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	if err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{badChecksumTask}, nil); err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}

	// Test Status 416 Range Not Satisfiable fallback
	rangeAttempts := 0
	rangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeAttempts++
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fresh-recovered-data"))
	}))
	defer rangeServer.Close()

	dest416 := filepath.Join(tempDir, "file416.bin")
	_ = os.WriteFile(dest416+".part", []byte("invalid-stale-partial-bytes"), 0644)

	task416 := &downloader.DownloadTask{
		ID:       "task-416",
		URL:      rangeServer.URL,
		DestPath: dest416,
	}
	if err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{task416}, nil); err != nil {
		t.Fatalf("expected 416 retry to recover and succeed, got error: %v", err)
	}
	recoveredBytes, _ := os.ReadFile(dest416)
	if string(recoveredBytes) != "fresh-recovered-data" {
		t.Fatalf("expected fresh-recovered-data, got %s", string(recoveredBytes))
	}
}