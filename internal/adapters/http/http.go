package http

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type DefaultHTTPClient struct {
	client *http.Client
}

func NewHTTPClient(timeout time.Duration) ports.HTTPClient {
	return &DefaultHTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *DefaultHTTPClient) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create get request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http error status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (c *DefaultHTTPClient) DownloadFile(
	ctx context.Context,
	url string,
	destPath string,
	expectedSHA1 string,
	onProgress func(bytesRead, totalBytes int64),
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	partPath := destPath + ".part"
	file, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("create destination part file: %w", err)
	}
	defer file.Close()

	hasher := sha1.New()
	writer := io.MultiWriter(file, hasher)

	totalBytes := resp.ContentLength
	var bytesRead int64
	buf := make([]byte, 32*1024)

	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := writer.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write download chunk: %w", wErr)
			}
			bytesRead += int64(n)
			if onProgress != nil {
				onProgress(bytesRead, totalBytes)
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return fmt.Errorf("read download stream: %w", rErr)
		}
	}

	if expectedSHA1 != "" {
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != expectedSHA1 {
			_ = os.Remove(partPath)
			return fmt.Errorf("sha1 mismatch: expected %s, got %s", expectedSHA1, actualHash)
		}
	}

	file.Close()
	if err := os.Rename(partPath, destPath); err != nil {
		return fmt.Errorf("atomic rename downloaded file: %w", err)
	}

	return nil
}
