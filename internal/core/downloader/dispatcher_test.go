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