package java

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/netutil"
)

type JavaDownloadStatusDTO struct {
	TaskID     string  `json:"task_id"`
	Major      int     `json:"major"`
	Status     string  `json:"status"` // "idle", "downloading", "extracting", "ready", "failed"
	BytesRead  int64   `json:"bytes_read"`
	TotalBytes int64   `json:"total_bytes"`
	Percentage float64 `json:"percentage"`
	Error      string  `json:"error,omitempty"`
}

type AdoptiumRuntimeService struct {
	client     *AdoptiumClient
	httpClient *http.Client
	managedDir string
	mu         sync.RWMutex
	status     JavaDownloadStatusDTO
	cancelFn   context.CancelFunc
}

func NewAdoptiumRuntimeService(managedDir string, client *AdoptiumClient, httpClient *http.Client) *AdoptiumRuntimeService {
	if httpClient == nil {
		httpClient = netutil.NewHTTPClient("0.6.1", 15*time.Minute)
	} else {
		httpClient.Transport = netutil.NewTransport("0.6.1", httpClient.Transport)
	}
	if client == nil {
		client = NewAdoptiumClient(DefaultAdoptiumBaseURL, httpClient)
	}
	return &AdoptiumRuntimeService{
		client:     client,
		httpClient: httpClient,
		managedDir: managedDir,
		status: JavaDownloadStatusDTO{
			Status: "idle",
		},
	}
}

func (s *AdoptiumRuntimeService) Client() *AdoptiumClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *AdoptiumRuntimeService) SetClient(c *AdoptiumClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = c
}

func (s *AdoptiumRuntimeService) GetDownloadStatus() JavaDownloadStatusDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *AdoptiumRuntimeService) setStatus(status JavaDownloadStatusDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *AdoptiumRuntimeService) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
	s.status = JavaDownloadStatusDTO{
		Status: "failed",
		Error:  "download cancelled",
	}
}

// Download provisions an Adoptium JDK runtime for the requested Java major version.
func (s *AdoptiumRuntimeService) Download(ctx context.Context, major int) (string, error) {
	if major <= 0 {
		return "", fmt.Errorf("invalid major version: %d", major)
	}

	taskID := fmt.Sprintf("adoptium-%d-%d", major, time.Now().UnixNano())
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelFn = cancel
	s.status = JavaDownloadStatusDTO{
		TaskID: taskID,
		Major:  major,
		Status: "downloading",
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.cancelFn = nil
		s.mu.Unlock()
	}()

	// 1. Fetch release metadata from Adoptium
	asset, err := s.client.GetLatestRelease(ctx, major)
	if err != nil {
		s.setStatus(JavaDownloadStatusDTO{
			TaskID: taskID,
			Major:  major,
			Status: "failed",
			Error:  err.Error(),
		})
		return "", fmt.Errorf("fetch adoptium release: %w", err)
	}

	s.setStatus(JavaDownloadStatusDTO{
		TaskID:     taskID,
		Major:      major,
		Status:     "downloading",
		TotalBytes: asset.Size,
	})

	// 2. Download package to temporary file
	if err := os.MkdirAll(s.managedDir, 0755); err != nil {
		return "", fmt.Errorf("create runtimes dir: %w", err)
	}

	tempFile, err := os.CreateTemp(s.managedDir, "adoptium-download-*")
	if err != nil {
		return "", fmt.Errorf("create temp download file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok best-effort temp file cleanup
		_ = os.Remove(tempPath) // errcheck:ok best-effort temp file cleanup
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", netutil.FormatUserAgent(""))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.setStatus(JavaDownloadStatusDTO{
			TaskID: taskID,
			Major:  major,
			Status: "failed",
			Error:  err.Error(),
		})
		return "", fmt.Errorf("download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("download returned status %d", resp.StatusCode)
		s.setStatus(JavaDownloadStatusDTO{
			TaskID: taskID,
			Major:  major,
			Status: "failed",
			Error:  err.Error(),
		})
		return "", err
	}

	totalBytes := asset.Size
	if resp.ContentLength > 0 {
		totalBytes = resp.ContentLength
	}

	var bytesRead int64
	buf := make([]byte, 64*1024)
	lastUpdate := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tempFile.Write(buf[:n]); writeErr != nil {
				return "", fmt.Errorf("write temp download file: %w", writeErr)
			}
			bytesRead += int64(n)

			if time.Since(lastUpdate) >= 100*time.Millisecond || readErr != nil {
				var pct float64
				if totalBytes > 0 {
					pct = float64(bytesRead) / float64(totalBytes) * 100
					if pct > 100 {
						pct = 100
					}
				}
				s.setStatus(JavaDownloadStatusDTO{
					TaskID:     taskID,
					Major:      major,
					Status:     "downloading",
					BytesRead:  bytesRead,
					TotalBytes: totalBytes,
					Percentage: pct,
				})
				lastUpdate = time.Now()
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			s.setStatus(JavaDownloadStatusDTO{
				TaskID: taskID,
				Major:  major,
				Status: "failed",
				Error:  readErr.Error(),
			})
			return "", fmt.Errorf("stream download package: %w", readErr)
		}
	}
	_ = tempFile.Close() // errcheck:ok close before hash check and extract

	// 3. Verify SHA-256
	s.setStatus(JavaDownloadStatusDTO{
		TaskID:     taskID,
		Major:      major,
		Status:     "extracting",
		BytesRead:  bytesRead,
		TotalBytes: totalBytes,
		Percentage: 100,
	})

	if asset.SHA256 != "" {
		if err := verifySHA256(tempPath, asset.SHA256); err != nil {
			s.setStatus(JavaDownloadStatusDTO{
				TaskID: taskID,
				Major:  major,
				Status: "failed",
				Error:  err.Error(),
			})
			return "", fmt.Errorf("verify checksum: %w", err)
		}
	}

	// 4. Extract archive to temporary staging folder
	stagingDir, err := os.MkdirTemp(s.managedDir, "adoptium-extract-*")
	if err != nil {
		return "", fmt.Errorf("create staging extract dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir) // errcheck:ok cleanup staging directory
	}()

	isZip := strings.HasSuffix(strings.ToLower(asset.Name), ".zip")
	if isZip {
		if err := extractZip(tempPath, stagingDir); err != nil {
			s.setStatus(JavaDownloadStatusDTO{
				TaskID: taskID,
				Major:  major,
				Status: "failed",
				Error:  err.Error(),
			})
			return "", fmt.Errorf("extract zip: %w", err)
		}
	} else {
		if err := extractTarGz(tempPath, stagingDir); err != nil {
			s.setStatus(JavaDownloadStatusDTO{
				TaskID: taskID,
				Major:  major,
				Status: "failed",
				Error:  err.Error(),
			})
			return "", fmt.Errorf("extract tar.gz: %w", err)
		}
	}

	// 5. Discover JDK root (handling depth-1 top-level directory like jdk-21.0.2+13 per R8)
	jdkRoot, javaBinaryPath, err := discoverJDKRoot(stagingDir)
	if err != nil {
		s.setStatus(JavaDownloadStatusDTO{
			TaskID: taskID,
			Major:  major,
			Status: "failed",
			Error:  err.Error(),
		})
		return "", fmt.Errorf("discover jdk root in extracted archive: %w", err)
	}

	// 6. Move to final managed folder: <managedDir>/adoptium-<major>
	cleanVersion := strings.ReplaceAll(asset.Version, "+", "-")
	targetDirName := fmt.Sprintf("adoptium-%d-%s", major, cleanVersion)
	finalDir := filepath.Join(s.managedDir, targetDirName)

	_ = os.RemoveAll(finalDir) // errcheck:ok remove any prior incomplete installation
	if err := os.Rename(jdkRoot, finalDir); err != nil {
		// Fallback to copy if cross-device or permission rename failure
		if copyErr := copyDirectory(jdkRoot, finalDir); copyErr != nil {
			s.setStatus(JavaDownloadStatusDTO{
				TaskID: taskID,
				Major:  major,
				Status: "failed",
				Error:  copyErr.Error(),
			})
			return "", fmt.Errorf("move extracted jdk to %s: %w", finalDir, copyErr)
		}
	}

	relJavaBin, _ := filepath.Rel(jdkRoot, javaBinaryPath)
	finalJavaBin := filepath.Join(finalDir, relJavaBin)

	// Ensure executable permissions on POSIX
	_ = os.Chmod(finalJavaBin, 0755) // errcheck:ok set executable permissions

	s.setStatus(JavaDownloadStatusDTO{
		TaskID:     taskID,
		Major:      major,
		Status:     "ready",
		BytesRead:  totalBytes,
		TotalBytes: totalBytes,
		Percentage: 100,
	})

	return finalJavaBin, nil
}

func verifySHA256(filePath, expectedSHA string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for sha256 check: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("calculate sha256: %w", err)
	}
	actualSHA := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actualSHA, strings.TrimSpace(expectedSHA)) {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA, actualSHA)
	}
	return nil
}

func discoverJDKRoot(baseDir string) (rootDir string, javaBinPath string, err error) {
	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// 1. Direct check at baseDir
	directBin := filepath.Join(baseDir, "bin", javaExe)
	if _, err := os.Stat(directBin); err == nil {
		return baseDir, directBin, nil
	}

	// 2. Recursive depth-1 check in subdirectories (per R8: Temurin top-level jdk-XX+YY/)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", "", fmt.Errorf("read staging directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(baseDir, entry.Name())
			subBin := filepath.Join(subDir, "bin", javaExe)
			if _, err := os.Stat(subBin); err == nil {
				return subDir, subBin, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not find bin/%s within depth 1 of extracted archive", javaExe)
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	cleanDest := filepath.Clean(destDir)
	for _, f := range r.File {
		targetPath := filepath.Join(destDir, f.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) && cleanTarget != cleanDest {
			return fmt.Errorf("illegal file path in zip archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("create zip dir: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("create zip parent dir: %w", err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("create zip destination file: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close() // errcheck:ok cleanup on error
			return fmt.Errorf("open zip file entry: %w", err)
		}

		_, copyErr := io.Copy(outFile, rc)
		_ = rc.Close()      // errcheck:ok close entry
		_ = outFile.Close() // errcheck:ok close target file

		if copyErr != nil {
			return fmt.Errorf("write zip file content: %w", copyErr)
		}
	}
	return nil
}

func extractTarGz(tarGzPath, destDir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	cleanDest := filepath.Clean(destDir)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) && cleanTarget != cleanDest {
			return fmt.Errorf("illegal file path in tar archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("create tar dir: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("create tar parent dir: %w", err)
			}
			outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create tar destination file: %w", err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				_ = outFile.Close() // errcheck:ok close on error
				return fmt.Errorf("write tar file content: %w", err)
			}
			_ = outFile.Close() // errcheck:ok close target file
		}
	}
	return nil
}

func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
