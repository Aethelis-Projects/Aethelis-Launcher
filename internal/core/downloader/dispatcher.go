package downloader

import (
	"container/heap"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Config configures the Dispatcher worker pool and network resilience.
type Config struct {
	MaxWorkers      int           // Maximum concurrent download workers
	MaxConnsPerHost int           // Maximum concurrent connections to a single domain host
	HTTPTimeout     time.Duration // Per-request HTTP timeout
	MaxRetries      int           // Number of retries per mirror/URL
	BaseBackoff     time.Duration // Base duration for exponential backoff with jitter
	BufferSize      int           // Streaming buffer size in bytes
}

// DefaultConfig provides production-tuned defaults.
func DefaultConfig() Config {
	return Config{
		MaxWorkers:      16,
		MaxConnsPerHost: 4,
		HTTPTimeout:     30 * time.Second,
		MaxRetries:      3,
		BaseBackoff:     150 * time.Millisecond,
		BufferSize:      64 * 1024, // 64 KB
	}
}

// BatchProgress reports aggregate download metrics for high-frequency UI updates.
type BatchProgress struct {
	TotalTasks      int     `json:"total_tasks"`
	CompletedTasks  int     `json:"completed_tasks"`
	FailedTasks     int     `json:"failed_tasks"`
	TotalBytes      int64   `json:"total_bytes"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	SpeedBPS        int64   `json:"speed_bps"`
	CurrentFile     string  `json:"current_file"`
	Percentage      float64 `json:"percentage"`
}

// Dispatcher coordinates parallel, prioritized downloads with resilience,
// rate-limiting per host, HTTP range resume, and hash verification.
type Dispatcher struct {
	config     Config
	httpClient *http.Client

	hostMu   sync.Mutex
	hostSems map[string]chan struct{}
}

// NewDispatcher creates an initialized parallel downloader dispatcher.
func NewDispatcher(cfg Config, client *http.Client) *Dispatcher {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 16
	}
	if cfg.MaxConnsPerHost <= 0 {
		cfg.MaxConnsPerHost = 4
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 64 * 1024
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}

	if client == nil {
		client = &http.Client{
			Timeout: cfg.HTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	return &Dispatcher{
		config:     cfg,
		httpClient: client,
		hostSems:   make(map[string]chan struct{}),
	}
}

// acquireHost limits concurrent active HTTP requests to each domain host.
func (d *Dispatcher) acquireHost(ctx context.Context, rawURL string) (func(), error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return func() {}, nil
	}
	host := u.Hostname()
	if host == "" {
		return func() {}, nil
	}

	d.hostMu.Lock()
	sem, exists := d.hostSems[host]
	if !exists {
		sem = make(chan struct{}, d.config.MaxConnsPerHost)
		d.hostSems[host] = sem
	}
	d.hostMu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	}
}

// DownloadBatch runs all provided tasks through the priority queue and worker pool.
func (d *Dispatcher) DownloadBatch(
	ctx context.Context,
	tasks []*DownloadTask,
	onProgress func(BatchProgress),
) error {
	if len(tasks) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Prepare priority queue
	pq := make(TaskPriorityQueue, 0, len(tasks))
	var totalExpectedBytes int64
	for _, t := range tasks {
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		totalExpectedBytes += t.ExpectedSize
		pq = append(pq, t)
	}
	heap.Init(&pq)

	queueMu := sync.Mutex{}
	popNextTask := func() *DownloadTask {
		queueMu.Lock()
		defer queueMu.Unlock()
		if pq.Len() == 0 {
			return nil
		}
		return heap.Pop(&pq).(*DownloadTask)
	}

	var (
		completedTasks  atomic.Int64
		failedTasks     atomic.Int64
		downloadedBytes atomic.Int64
		currentFileName atomic.Value
	)
	currentFileName.Store("")

	var taskErrorsMu sync.Mutex
	var taskErrors []error

	// Progress reporter loop
	ctxWithCancel, cancel := context.WithCancel(ctx)
	defer cancel()

	var reporterWG sync.WaitGroup
	if onProgress != nil {
		reporterWG.Add(1)
		go func() {
			defer reporterWG.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			var lastBytes int64
			var lastTime = time.Now()

			for {
				select {
				case <-ctxWithCancel.Done():
					return
				case now := <-ticker.C:
					curBytes := downloadedBytes.Load()
					deltaBytes := curBytes - lastBytes
					deltaTime := now.Sub(lastTime).Seconds()
					var speed int64
					if deltaTime > 0 {
						speed = int64(float64(deltaBytes) / deltaTime)
					}
					lastBytes = curBytes
					lastTime = now

					done := int(completedTasks.Load())
					failed := int(failedTasks.Load())
					var pct float64
					if totalExpectedBytes > 0 {
						pct = float64(curBytes) / float64(totalExpectedBytes) * 100.0
						if pct > 100.0 {
							pct = 100.0
						}
					} else if len(tasks) > 0 {
						pct = float64(done) / float64(len(tasks)) * 100.0
					}

					curName, _ := currentFileName.Load().(string)

					onProgress(BatchProgress{
						TotalTasks:      len(tasks),
						CompletedTasks:  done,
						FailedTasks:     failed,
						TotalBytes:      totalExpectedBytes,
						DownloadedBytes: curBytes,
						SpeedBPS:        speed,
						CurrentFile:     curName,
						Percentage:      pct,
					})
				}
			}
		}()
	}

	// Worker pool
	numWorkers := d.config.MaxWorkers
	if numWorkers > len(tasks) {
		numWorkers = len(tasks)
	}

	var workersWG sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for {
				select {
				case <-ctxWithCancel.Done():
					return
				default:
					task := popNextTask()
					if task == nil {
						return // queue exhausted
					}

					currentFileName.Store(filepath.Base(task.DestPath))

					err := d.executeTask(ctxWithCancel, task, &downloadedBytes)
					if err != nil {
						failedTasks.Add(1)
						taskErrorsMu.Lock()
						taskErrors = append(taskErrors, fmt.Errorf("task %s (%s): %w", task.ID, task.DestPath, err))
						taskErrorsMu.Unlock()
					} else {
						completedTasks.Add(1)
					}
				}
			}
		}()
	}

	workersWG.Wait()
	cancel() // Stop progress reporter
	reporterWG.Wait()

	// Final progress update
	if onProgress != nil {
		done := int(completedTasks.Load())
		failed := int(failedTasks.Load())
		curBytes := downloadedBytes.Load()
		pct := 100.0
		if totalExpectedBytes > 0 && curBytes < totalExpectedBytes {
			pct = float64(curBytes) / float64(totalExpectedBytes) * 100.0
		}
		onProgress(BatchProgress{
			TotalTasks:      len(tasks),
			CompletedTasks:  done,
			FailedTasks:     failed,
			TotalBytes:      totalExpectedBytes,
			DownloadedBytes: curBytes,
			SpeedBPS:        0,
			CurrentFile:     "",
			Percentage:      pct,
		})
	}

	if len(taskErrors) > 0 {
		return errors.Join(taskErrors...)
	}
	return nil
}

// executeTask attempts to download a file with mirror failover and exponential backoff.
func (d *Dispatcher) executeTask(ctx context.Context, task *DownloadTask, totalBytesCounter *atomic.Int64) error {
	// 1. If file already exists and matches expected hash, skip!
	if d.verifyExistingFile(task) {
		if task.ExpectedSize > 0 {
			totalBytesCounter.Add(task.ExpectedSize)
		}
		return nil
	}

	// Build candidate URLs: primary URL followed by mirrors
	candidateURLs := make([]string, 0, 1+len(task.Mirrors))
	if task.URL != "" {
		candidateURLs = append(candidateURLs, task.URL)
	}
	candidateURLs = append(candidateURLs, task.Mirrors...)

	if len(candidateURLs) == 0 {
		return fmt.Errorf("%w: no URLs provided for task %s", ErrAllMirrorsFailed, task.ID)
	}

	var lastErr error
	for _, rawURL := range candidateURLs {
		for attempt := 0; attempt <= d.config.MaxRetries; attempt++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if attempt > 0 {
				// Exponential backoff with jitter
				backoff := d.config.BaseBackoff * time.Duration(1<<(attempt-1))
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				time.Sleep(backoff + jitter)
			}

			err := d.downloadSingleURL(ctx, rawURL, task, totalBytesCounter)
			if err == nil {
				return nil // Succeeded!
			}

			lastErr = err
		}
	}

	return fmt.Errorf("%w: last error: %v", ErrAllMirrorsFailed, lastErr)
}

// downloadSingleURL handles Range resumption, downloading, checksum verification, and atomic swap.
func (d *Dispatcher) downloadSingleURL(
	ctx context.Context,
	rawURL string,
	task *DownloadTask,
	totalBytesCounter *atomic.Int64,
) error {
	releaseHost, err := d.acquireHost(ctx, rawURL)
	if err != nil {
		return err
	}
	defer releaseHost()

	if err := os.MkdirAll(filepath.Dir(task.DestPath), 0755); err != nil {
		return fmt.Errorf("create dest directory: %w", err)
	}

	partPath := task.DestPath + ".part"

	var existingSize int64
	if stat, err := os.Stat(partPath); err == nil {
		existingSize = stat.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http execute: %w", err)
	}
	defer resp.Body.Close()

	var file *os.File
	var bytesToCount = existingSize

	switch resp.StatusCode {
	case http.StatusPartialContent: // 206: Resuming existing part file
		file, err = os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open part file for resume: %w", err)
		}
		// Count the pre-existing bytes towards batch progress if not yet counted
		totalBytesCounter.Add(existingSize)

	case http.StatusOK: // 200: Server does not support Range or started fresh
		file, err = os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("open part file for write: %w", err)
		}
		bytesToCount = 0

	case http.StatusRequestedRangeNotSatisfiable: // 416: Existing part is invalid
		_ = os.Remove(partPath) // slop:ok purge corrupt or mismatched part file before fresh download
		// Retry fresh request without range
		file, err = os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("recreate part file after 416: %w", err)
		}
		reqFresh, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			file.Close()
			return err
		}
		respFresh, err := d.httpClient.Do(reqFresh)
		if err != nil {
			file.Close()
			return err
		}
		defer respFresh.Body.Close()
		if respFresh.StatusCode != http.StatusOK {
			file.Close()
			return fmt.Errorf("%w: status %d", ErrInvalidStatusCode, respFresh.StatusCode)
		}
		resp = respFresh
		bytesToCount = 0

	default:
		return fmt.Errorf("%w: HTTP %d", ErrInvalidStatusCode, resp.StatusCode)
	}

	buf := make([]byte, d.config.BufferSize)
	var streamErr error
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := file.Write(buf[:n]); wErr != nil {
				file.Close()
				return fmt.Errorf("write to part file: %w", wErr)
			}
			totalBytesCounter.Add(int64(n))
		}
		if rErr != nil {
			if rErr != io.EOF {
				streamErr = rErr
			}
			break
		}
	}
	file.Close()

	if streamErr != nil {
		// Network dropped mid-stream; keep .part file intact so next attempt can Range-resume
		return fmt.Errorf("stream read error: %w", streamErr)
	}

	// Verify checksum of completed .part file
	if err := d.verifyPartChecksum(partPath, task); err != nil {
		_ = os.Remove(partPath) // slop:ok Checksum failed; purge corrupt file
		if bytesToCount > 0 {
			totalBytesCounter.Add(-bytesToCount)
		}
		return err
	}

	// Atomically promote .part to final destination
	if err := atomicReplace(partPath, task.DestPath); err != nil {
		return fmt.Errorf("atomic rename part to final: %w", err)
	}

	return nil
}

// verifyExistingFile checks if destination already exists and passes hash verification.
func (d *Dispatcher) verifyExistingFile(task *DownloadTask) bool {
	info, err := os.Stat(task.DestPath)
	if err != nil || info.IsDir() {
		return false
	}
	if task.ExpectedSize > 0 && info.Size() != task.ExpectedSize {
		return false
	}
	if task.ExpectedSHA1 == "" && task.ExpectedSHA256 == "" {
		return true // No hash provided, size matches or exists
	}
	return d.verifyPartChecksum(task.DestPath, task) == nil
}

// verifyPartChecksum calculates SHA-1 / SHA-256 on a file and compares against expected.
func (d *Dispatcher) verifyPartChecksum(path string, task *DownloadTask) error {
	if task.ExpectedSHA1 == "" && task.ExpectedSHA256 == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h1 := sha1.New()
	h256 := sha256.New()
	var writer io.Writer

	if task.ExpectedSHA1 != "" && task.ExpectedSHA256 != "" {
		writer = io.MultiWriter(h1, h256)
	} else if task.ExpectedSHA1 != "" {
		writer = h1
	} else {
		writer = h256
	}

	if _, err := io.Copy(writer, f); err != nil {
		return fmt.Errorf("hash compute error: %w", err)
	}

	if task.ExpectedSHA1 != "" {
		actual := hex.EncodeToString(h1.Sum(nil))
		if actual != task.ExpectedSHA1 {
			return &ChecksumMismatchError{
				Algorithm: "SHA-1",
				Expected:  task.ExpectedSHA1,
				Actual:    actual,
				FilePath:  path,
			}
		}
	}

	if task.ExpectedSHA256 != "" {
		actual := hex.EncodeToString(h256.Sum(nil))
		if actual != task.ExpectedSHA256 {
			return &ChecksumMismatchError{
				Algorithm: "SHA-256",
				Expected:  task.ExpectedSHA256,
				Actual:    actual,
				FilePath:  path,
			}
		}
	}

	return nil
}

// atomicReplace replaces dst with src on both Unix and Windows.
func atomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		// On Windows, Rename fails if dst already exists.
		_ = os.Remove(dst) // slop:ok remove existing destination file on Windows before rename
		return os.Rename(src, dst)
	}
	return nil
}