package java

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

type mockProvisionerDetector struct {
	installs []ports.JavaInstallation
}

func (m *mockProvisionerDetector) DetectInstallations(ctx context.Context) ([]ports.JavaInstallation, error) {
	return m.installs, nil
}

func (m *mockProvisionerDetector) FindSuitableJava(ctx context.Context, requiredMajor int) (*ports.JavaInstallation, error) {
	for _, inst := range m.installs {
		if inst.MajorVersion == requiredMajor {
			return &inst, nil
		}
	}
	return nil, nil
}

func (m *mockProvisionerDetector) ValidateJava(ctx context.Context, path string) (*ports.JavaInstallation, error) {
	for _, inst := range m.installs {
		if inst.Path == path {
			return &inst, nil
		}
	}
	return nil, fmt.Errorf("java not found")
}

type mockProvisionerInstanceRepo struct {
	instances []*domain.Instance
}

func (m *mockProvisionerInstanceRepo) Save(ctx context.Context, inst *domain.Instance) error {
	m.instances = append(m.instances, inst)
	return nil
}

func (m *mockProvisionerInstanceRepo) GetByID(ctx context.Context, id string) (*domain.Instance, error) {
	for _, inst := range m.instances {
		if inst.ID == id {
			return inst, nil
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func (m *mockProvisionerInstanceRepo) ListAll(ctx context.Context) ([]*domain.Instance, error) {
	return m.instances, nil
}

func (m *mockProvisionerInstanceRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockProvisionerInstanceRepo) UpdateState(ctx context.Context, id string, state domain.InstanceState) error {
	return nil
}

func createTestZipArchive(t *testing.T, subDir, javaExe string) ([]byte, string) {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	binPath := filepath.ToSlash(filepath.Join(subDir, "bin", javaExe))
	f, err := zw.Create(binPath)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := f.Write([]byte("mock java binary content")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}

	relPath := filepath.ToSlash(filepath.Join(subDir, "release"))
	relFile, err := zw.Create(relPath)
	if err != nil {
		t.Fatalf("create zip release entry: %v", err)
	}
	releaseContent := fmt.Sprintf("JAVA_VERSION=\"21.0.2\"\nIMPLEMENTOR=\"Eclipse Adoptium\"\n")
	if _, err := relFile.Write([]byte(releaseContent)); err != nil {
		t.Fatalf("write zip release entry: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	bytesData := buf.Bytes()
	h := sha256.Sum256(bytesData)
	return bytesData, hex.EncodeToString(h[:])
}

func createTestTarGzArchive(t *testing.T, subDir, javaExe string) ([]byte, string) {
	t.Helper()
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	binPath := filepath.ToSlash(filepath.Join(subDir, "bin", javaExe))
	binContent := []byte("mock java binary content")
	hdr := &tar.Header{
		Name: binPath,
		Mode: 0755,
		Size: int64(len(binContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(binContent); err != nil {
		t.Fatalf("write tar content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	bytesData := buf.Bytes()
	h := sha256.Sum256(bytesData)
	return bytesData, hex.EncodeToString(h[:])
}

func TestAdoptiumRuntimeService_StatusAndCancel(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewAdoptiumRuntimeService(tempDir, nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	initStatus := svc.GetDownloadStatus()
	if initStatus.Status != "idle" {
		t.Fatalf("expected initial status 'idle', got %q", initStatus.Status)
	}

	svc.setStatus(JavaDownloadStatusDTO{
		Status:     "downloading",
		Major:      21,
		BytesRead:  500,
		TotalBytes: 1000,
		Percentage: 50.0,
	})

	updated := svc.GetDownloadStatus()
	if updated.Status != "downloading" || updated.Major != 21 || updated.Percentage != 50.0 {
		t.Fatalf("unexpected updated status: %+v", updated)
	}

	svc.Cancel()
	cancelled := svc.GetDownloadStatus()
	if cancelled.Status != "failed" || cancelled.Error != "download cancelled" {
		t.Fatalf("unexpected status after cancel: %+v", cancelled)
	}
}

func TestVerifySHA256(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.bin")
	content := []byte("sample hash verification data")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	h := sha256.Sum256(content)
	validHash := hex.EncodeToString(h[:])

	if err := verifySHA256(filePath, validHash); err != nil {
		t.Fatalf("expected valid sha256, got error: %v", err)
	}

	if err := verifySHA256(filePath, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected error on sha256 mismatch, got nil")
	}

	if err := verifySHA256(filepath.Join(tempDir, "nonexistent.bin"), validHash); err == nil {
		t.Fatal("expected error on nonexistent file, got nil")
	}
}

func TestDiscoverJDKRoot(t *testing.T) {
	javaExe := "java"
	if runtime.GOOS == "windows" {
		javaExe = "java.exe"
	}

	t.Run("direct bin directory", func(t *testing.T) {
		tempDir := t.TempDir()
		binDir := filepath.Join(tempDir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		binPath := filepath.Join(binDir, javaExe)
		if err := os.WriteFile(binPath, []byte("stub"), 0755); err != nil {
			t.Fatalf("write stub: %v", err)
		}

		rootDir, foundBin, err := discoverJDKRoot(tempDir)
		if err != nil {
			t.Fatalf("discoverJDKRoot failed: %v", err)
		}
		if rootDir != tempDir {
			t.Fatalf("expected rootDir %s, got %s", tempDir, rootDir)
		}
		if foundBin != binPath {
			t.Fatalf("expected foundBin %s, got %s", binPath, foundBin)
		}
	})

	t.Run("depth-1 subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		subDir := filepath.Join(tempDir, "jdk-21.0.2+13")
		binDir := filepath.Join(subDir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		binPath := filepath.Join(binDir, javaExe)
		if err := os.WriteFile(binPath, []byte("stub"), 0755); err != nil {
			t.Fatalf("write stub: %v", err)
		}

		rootDir, foundBin, err := discoverJDKRoot(tempDir)
		if err != nil {
			t.Fatalf("discoverJDKRoot failed: %v", err)
		}
		if rootDir != subDir {
			t.Fatalf("expected rootDir %s, got %s", subDir, rootDir)
		}
		if foundBin != binPath {
			t.Fatalf("expected foundBin %s, got %s", binPath, foundBin)
		}
	})

	t.Run("missing java binary", func(t *testing.T) {
		tempDir := t.TempDir()
		_, _, err := discoverJDKRoot(tempDir)
		if err == nil {
			t.Fatal("expected error on empty staging directory, got nil")
		}
	})
}

func TestExtractZipAndTarGz(t *testing.T) {
	javaExe := "java"
	if runtime.GOOS == "windows" {
		javaExe = "java.exe"
	}

	t.Run("extract zip archive", func(t *testing.T) {
		tempDir := t.TempDir()
		zipBytes, _ := createTestZipArchive(t, "jdk-21.0.2", javaExe)
		zipPath := filepath.Join(tempDir, "test.zip")
		if err := os.WriteFile(zipPath, zipBytes, 0644); err != nil {
			t.Fatalf("write zip: %v", err)
		}

		destDir := filepath.Join(tempDir, "extracted_zip")
		if err := extractZip(zipPath, destDir); err != nil {
			t.Fatalf("extractZip failed: %v", err)
		}

		expectedBin := filepath.Join(destDir, "jdk-21.0.2", "bin", javaExe)
		if _, err := os.Stat(expectedBin); err != nil {
			t.Fatalf("expected extracted binary at %s, error: %v", expectedBin, err)
		}

		// Invalid zip test
		if err := extractZip(filepath.Join(tempDir, "nonexistent.zip"), destDir); err == nil {
			t.Fatal("expected error on nonexistent zip, got nil")
		}
	})

	t.Run("extract tar.gz archive", func(t *testing.T) {
		tempDir := t.TempDir()
		tarBytes, _ := createTestTarGzArchive(t, "jdk-21.0.2", javaExe)
		tarPath := filepath.Join(tempDir, "test.tar.gz")
		if err := os.WriteFile(tarPath, tarBytes, 0644); err != nil {
			t.Fatalf("write tar: %v", err)
		}

		destDir := filepath.Join(tempDir, "extracted_tar")
		if err := extractTarGz(tarPath, destDir); err != nil {
			t.Fatalf("extractTarGz failed: %v", err)
		}

		expectedBin := filepath.Join(destDir, "jdk-21.0.2", "bin", javaExe)
		if _, err := os.Stat(expectedBin); err != nil {
			t.Fatalf("expected extracted binary at %s, error: %v", expectedBin, err)
		}

		// Invalid tar.gz test
		if err := extractTarGz(filepath.Join(tempDir, "nonexistent.tar.gz"), destDir); err == nil {
			t.Fatal("expected error on nonexistent tar.gz, got nil")
		}
	})
}

func TestCopyDirectoryAndFile(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	fileA := filepath.Join(srcDir, "fileA.txt")
	fileB := filepath.Join(srcDir, "sub", "fileB.txt")
	if err := os.WriteFile(fileA, []byte("aaa"), 0644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("bbb"), 0644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}

	dstDir := filepath.Join(tempDir, "dst")
	if err := copyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("copyDirectory failed: %v", err)
	}

	dstFileA := filepath.Join(dstDir, "fileA.txt")
	dstFileB := filepath.Join(dstDir, "sub", "fileB.txt")
	dataA, err := os.ReadFile(dstFileA)
	if err != nil || string(dataA) != "aaa" {
		t.Fatalf("copy verification failed for fileA: %v", err)
	}
	dataB, err := os.ReadFile(dstFileB)
	if err != nil || string(dataB) != "bbb" {
		t.Fatalf("copy verification failed for fileB: %v", err)
	}

	if err := copyFile(filepath.Join(tempDir, "ghost.txt"), filepath.Join(tempDir, "out.txt"), 0644); err == nil {
		t.Fatal("expected error on missing source file, got nil")
	}
}

func TestAdoptiumRuntimeService_Download_Success_Zip(t *testing.T) {
	javaExe := "java"
	if runtime.GOOS == "windows" {
		javaExe = "java.exe"
	}

	zipBytes, validHash := createTestZipArchive(t, "jdk-21.0.2+13", javaExe)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipBytes) // errcheck:ok test server response
			return
		}

		// Adoptium release endpoint
		release := []adoptiumReleaseItem{
			{
				Binaries: []struct {
					ImageType string `json:"image_type"`
					OS        string `json:"os"`
					Arch      string `json:"architecture"`
					Package   struct {
						Name     string `json:"name"`
						Link     string `json:"link"`
						Checksum string `json:"checksum"`
						Size     int64  `json:"size"`
					} `json:"package"`
				}{
					{
						ImageType: "jdk",
						Package: struct {
							Name     string `json:"name"`
							Link     string `json:"link"`
							Checksum string `json:"checksum"`
							Size     int64  `json:"size"`
						}{
							Name:     "OpenJDK21U-jdk_x64_test.zip",
							Link:     server.URL + "/download",
							Size:     int64(len(zipBytes)),
							Checksum: validHash,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release) // errcheck:ok test server response
	}))
	defer server.Close()

	tempDir := t.TempDir()
	adoptClient := NewAdoptiumClient(server.URL, server.Client())
	svc := NewAdoptiumRuntimeService(tempDir, adoptClient, server.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	binPath, err := svc.Download(ctx, 21)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("provisioned java binary does not exist at %s: %v", binPath, err)
	}

	status := svc.GetDownloadStatus()
	if status.Status != "ready" {
		t.Fatalf("expected status 'ready', got %q", status.Status)
	}
}

func TestAdoptiumRuntimeService_Download_Errors(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewAdoptiumRuntimeService(tempDir, nil, nil)

	t.Run("invalid major", func(t *testing.T) {
		_, err := svc.Download(context.Background(), 0)
		if err == nil {
			t.Fatal("expected error for major <= 0, got nil")
		}
	})

	t.Run("release fetch error 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		adoptClient := NewAdoptiumClient(server.URL, server.Client())
		svcError := NewAdoptiumRuntimeService(tempDir, adoptClient, server.Client())

		_, err := svcError.Download(context.Background(), 21)
		if err == nil {
			t.Fatal("expected error on 404 release, got nil")
		}
		if svcError.GetDownloadStatus().Status != "failed" {
			t.Fatalf("expected status 'failed', got %q", svcError.GetDownloadStatus().Status)
		}
	})

	t.Run("download package HTTP 500", func(t *testing.T) {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/download-fail" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			release := []adoptiumReleaseItem{
				{
					Binaries: []struct {
						ImageType string `json:"image_type"`
						OS        string `json:"os"`
						Arch      string `json:"architecture"`
						Package   struct {
							Name     string `json:"name"`
							Link     string `json:"link"`
							Checksum string `json:"checksum"`
							Size     int64  `json:"size"`
						} `json:"package"`
					}{
						{
							ImageType: "jdk",
							Package: struct {
								Name     string `json:"name"`
								Link     string `json:"link"`
								Checksum string `json:"checksum"`
								Size     int64  `json:"size"`
							}{
								Name: "fail.zip",
								Link: server.URL + "/download-fail",
								Size: 100,
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(release) // errcheck:ok test server response
		}))
		defer server.Close()

		adoptClient := NewAdoptiumClient(server.URL, server.Client())
		svcError := NewAdoptiumRuntimeService(tempDir, adoptClient, server.Client())

		_, err := svcError.Download(context.Background(), 21)
		if err == nil {
			t.Fatal("expected error on 500 download, got nil")
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/download-corrupt" {
				_, _ = w.Write([]byte("corrupt data")) // errcheck:ok test server response
				return
			}
			release := []adoptiumReleaseItem{
				{
					Binaries: []struct {
						ImageType string `json:"image_type"`
						OS        string `json:"os"`
						Arch      string `json:"architecture"`
						Package   struct {
							Name     string `json:"name"`
							Link     string `json:"link"`
							Checksum string `json:"checksum"`
							Size     int64  `json:"size"`
						} `json:"package"`
					}{
						{
							ImageType: "jdk",
							Package: struct {
								Name     string `json:"name"`
								Link     string `json:"link"`
								Checksum string `json:"checksum"`
								Size     int64  `json:"size"`
							}{
								Name:     "corrupt.zip",
								Link:     server.URL + "/download-corrupt",
								Size:     12,
								Checksum: "badhash123",
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(release) // errcheck:ok test server response
		}))
		defer server.Close()

		adoptClient := NewAdoptiumClient(server.URL, server.Client())
		svcError := NewAdoptiumRuntimeService(tempDir, adoptClient, server.Client())

		_, err := svcError.Download(context.Background(), 21)
		if err == nil {
			t.Fatal("expected error on checksum mismatch, got nil")
		}
	})
}

func TestJavaManager_DownloadRuntime(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	detector := &mockProvisionerDetector{}
	repo := &mockProvisionerInstanceRepo{
		instances: []*domain.Instance{
			{
				ID:          "inst-1",
				GameVersion: "1.20.4",
			},
		},
	}

	mgr := NewJavaManager(managedDir, detector, repo, nil)

	// 1. Status with nil provisioner
	mgr.SetProvisioner(nil)
	st := mgr.GetDownloadStatus()
	if st.Status != "idle" {
		t.Fatalf("expected idle status for nil provisioner, got %q", st.Status)
	}

	// 2. Download without provisioner
	if err := mgr.DownloadRuntime(context.Background(), 21); err == nil {
		t.Fatal("expected error when downloading without provisioner, got nil")
	}

	// 3. Set provisioner and download with major=0 (dynamic resolution)
	javaExe := "java"
	if runtime.GOOS == "windows" {
		javaExe = "java.exe"
	}
	zipBytes, validHash := createTestZipArchive(t, "jdk-17.0.10+7", javaExe)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipBytes) // errcheck:ok test server response
			return
		}
		release := []adoptiumReleaseItem{
			{
				Binaries: []struct {
					ImageType string `json:"image_type"`
					OS        string `json:"os"`
					Arch      string `json:"architecture"`
					Package   struct {
						Name     string `json:"name"`
						Link     string `json:"link"`
						Checksum string `json:"checksum"`
						Size     int64  `json:"size"`
					} `json:"package"`
				}{
					{
						ImageType: "jdk",
						Package: struct {
							Name     string `json:"name"`
							Link     string `json:"link"`
							Checksum string `json:"checksum"`
							Size     int64  `json:"size"`
						}{
							Name:     "test.zip",
							Link:     server.URL + "/download",
							Size:     int64(len(zipBytes)),
							Checksum: validHash,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release) // errcheck:ok test server response
	}))
	defer server.Close()

	client := NewAdoptiumClient(server.URL, server.Client())
	prov := NewAdoptiumRuntimeService(managedDir, client, server.Client())
	mgr.SetProvisioner(prov)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call DownloadRuntime with major=0 -> will resolve 1.20.4 -> major 17
	if err := mgr.DownloadRuntime(ctx, 0); err != nil {
		t.Fatalf("DownloadRuntime(0) failed: %v", err)
	}

	cur := mgr.GetDownloadStatus()
	if cur.Status != "ready" {
		t.Fatalf("expected status 'ready', got %q", cur.Status)
	}
}

func TestJavaManager_ParseJavaMajorFromOutput(t *testing.T) {
	tests := []struct {
		output   string
		expected int
	}{
		{output: `openjdk version "21.0.2" 2024-01-16`, expected: 21},
		{output: `java version "1.8.0_391"`, expected: 8},
		{output: `garbage unparseable output`, expected: 0},
	}

	for _, tc := range tests {
		got, _ := ParseJavaMajorFromOutput(tc.output)
		if got != tc.expected {
			t.Errorf("ParseJavaMajorFromOutput(%q) = %d; expected %d", tc.output, got, tc.expected)
		}
	}
}
