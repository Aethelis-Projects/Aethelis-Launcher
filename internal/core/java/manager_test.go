package java_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

type mockJavaDetector struct {
	installations []ports.JavaInstallation
}

func (m *mockJavaDetector) DetectInstallations(ctx context.Context) ([]ports.JavaInstallation, error) {
	return m.installations, nil
}

func (m *mockJavaDetector) FindSuitableJava(ctx context.Context, requiredMajor int) (*ports.JavaInstallation, error) {
	for _, inst := range m.installations {
		if inst.MajorVersion == requiredMajor {
			return &inst, nil
		}
	}
	return nil, nil
}

func (m *mockJavaDetector) ValidateJava(ctx context.Context, path string) (*ports.JavaInstallation, error) {
	for _, inst := range m.installations {
		if inst.Path == path {
			return &inst, nil
		}
	}
	return nil, fmt.Errorf("java not found")
}

type mockInstanceRepo struct {
	instances []*domain.Instance
}

func (m *mockInstanceRepo) Save(ctx context.Context, inst *domain.Instance) error {
	for i, existing := range m.instances {
		if existing.ID == inst.ID {
			m.instances[i] = inst
			return nil
		}
	}
	m.instances = append(m.instances, inst)
	return nil
}

func (m *mockInstanceRepo) GetByID(ctx context.Context, id string) (*domain.Instance, error) {
	for _, inst := range m.instances {
		if inst.ID == id {
			return inst, nil
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func (m *mockInstanceRepo) ListAll(ctx context.Context) ([]*domain.Instance, error) {
	return m.instances, nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockInstanceRepo) UpdateState(ctx context.Context, id string, state domain.InstanceState) error {
	for _, inst := range m.instances {
		if inst.ID == id {
			inst.State = state
			return nil
		}
	}
	return nil
}

func (m *mockInstanceRepo) EnsureDefaultInstance(ctx context.Context) (*domain.Instance, error) {
	return nil, nil
}

func TestJavaManager_ListRuntimes(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// 1. Create a simulated managed runtime folder with release file and dummy java binary
	managedRuntimeHome := filepath.Join(managedDir, "adoptium-21-21.0.2")
	managedBinDir := filepath.Join(managedRuntimeHome, "bin")
	if err := os.MkdirAll(managedBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	managedBinary := filepath.Join(managedBinDir, javaExe)
	if err := os.WriteFile(managedBinary, []byte("fake-bin"), 0755); err != nil {
		t.Fatalf("failed to write dummy binary: %v", err)
	}

	releaseContent := `JAVA_VERSION="21.0.2"
OS_NAME="Windows"
OS_ARCH="x86_64"
IMPLEMENTOR="Eclipse Adoptium"
`
	if err := os.WriteFile(filepath.Join(managedRuntimeHome, "release"), []byte(releaseContent), 0644); err != nil {
		t.Fatalf("failed to write release file: %v", err)
	}

	// 2. Setup mock detector with 1 system installation
	systemJavaBin := filepath.Join(tempDir, "system-jdk", "bin", javaExe)
	detector := &mockJavaDetector{
		installations: []ports.JavaInstallation{
			{
				Path:         systemJavaBin,
				HomeDir:      filepath.Join(tempDir, "system-jdk"),
				MajorVersion: 17,
				FullVersion:  "17.0.10",
				Vendor:       "Oracle",
			},
		},
	}

	// 3. Setup mock repo with instances using both runtimes
	repo := &mockInstanceRepo{
		instances: []*domain.Instance{
			{
				ID:          "inst-1",
				Name:        "Survival",
				GameVersion: "1.20.1",
				JavaPath:    systemJavaBin,
			},
			{
				ID:          "inst-2",
				Name:        "Modded 1.21",
				GameVersion: "1.21.1",
				JavaPath:    managedBinary,
			},
		},
	}

	mgr := java.NewJavaManager(managedDir, detector, repo, nil)
	runtimes, err := mgr.ListRuntimes(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimes failed: %v", err)
	}

	if len(runtimes) != 2 {
		t.Fatalf("expected 2 runtimes, got %d", len(runtimes))
	}

	// Managed runtime should be sorted first
	managed := runtimes[0]
	if managed.Kind != "managed" {
		t.Errorf("expected managed runtime to have Kind='managed', got %q", managed.Kind)
	}
	if managed.MajorVersion != 21 {
		t.Errorf("expected major 21, got %d", managed.MajorVersion)
	}
	if len(managed.UsedBy) != 1 || managed.UsedBy[0] != "Modded 1.21" {
		t.Errorf("expected UsedBy=['Modded 1.21'], got %+v", managed.UsedBy)
	}

	// System runtime should have Kind='detected'
	system := runtimes[1]
	if system.Kind != "detected" {
		t.Errorf("expected system runtime to have Kind='detected', got %q", system.Kind)
	}
	if system.MajorVersion != 17 {
		t.Errorf("expected major 17, got %d", system.MajorVersion)
	}
	if len(system.UsedBy) != 1 || system.UsedBy[0] != "Survival" {
		t.Errorf("expected UsedBy=['Survival'], got %+v", system.UsedBy)
	}
}

func TestJavaManager_RemoveRuntime_Guards(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// Create a managed runtime folder
	managedHome := filepath.Join(managedDir, "adoptium-21-test")
	managedBinDir := filepath.Join(managedHome, "bin")
	if err := os.MkdirAll(managedBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	managedBinary := filepath.Join(managedBinDir, javaExe)
	if err := os.WriteFile(managedBinary, []byte("fake"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	// 1. Cannot remove system runtime (outside managed dir)
	systemBinary := filepath.Join(tempDir, "outside", "bin", javaExe)
	repo := &mockInstanceRepo{}
	mgr := java.NewJavaManager(managedDir, nil, repo, nil)

	err := mgr.RemoveRuntime(systemBinary)
	if err == nil {
		t.Error("expected error removing runtime outside managedDir, got nil")
	}

	// 2. Cannot remove if an instance is running with this runtime
	repo.instances = []*domain.Instance{
		{
			ID:       "inst-running",
			Name:     "Running Instance",
			JavaPath: managedBinary,
			State:    domain.StateRunning,
		},
	}
	err = mgr.RemoveRuntime(managedBinary)
	if err == nil {
		t.Error("expected error removing runtime used by running instance, got nil")
	}

	// 3. Cannot remove if used by an instance (even stopped)
	repo.instances[0].State = domain.StateIdle
	err = mgr.RemoveRuntime(managedBinary)
	if err == nil {
		t.Error("expected error removing runtime used by instance, got nil")
	}

	// 4. Successful removal when not used by any instance
	repo.instances = []*domain.Instance{}
	err = mgr.RemoveRuntime(managedBinary)
	if err != nil {
		t.Fatalf("unexpected error removing unused managed runtime: %v", err)
	}

	if _, statErr := os.Stat(managedHome); !os.IsNotExist(statErr) {
		t.Errorf("expected runtime directory %s to be deleted, but stat returned %v", managedHome, statErr)
	}
}

func TestJavaManager_AddRuntime(t *testing.T) {
	tempDir := t.TempDir()
	customDir := filepath.Join(tempDir, "custom-jdk")
	binDir := filepath.Join(customDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}
	customBinary := filepath.Join(binDir, javaExe)
	if err := os.WriteFile(customBinary, []byte("fake"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	releaseContent := `JAVA_VERSION="17.0.9"
OS_NAME="Windows"
IMPLEMENTOR="BellSoft"
`
	if err := os.WriteFile(filepath.Join(customDir, "release"), []byte(releaseContent), 0644); err != nil {
		t.Fatalf("failed to write release file: %v", err)
	}

	mgr := java.NewJavaManager(filepath.Join(tempDir, "managed"), nil, nil, nil)

	// 1. Add valid directory
	inst, err := mgr.AddRuntime(context.Background(), customDir)
	if err != nil {
		t.Fatalf("AddRuntime failed for valid directory: %v", err)
	}
	if inst.MajorVersion != 17 {
		t.Errorf("expected major 17, got %d", inst.MajorVersion)
	}
	if inst.Kind != "detected" {
		t.Errorf("expected kind 'detected', got %s", inst.Kind)
	}

	// 2. Add empty path returns error
	_, err = mgr.AddRuntime(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}

	// 3. Add nonexistent path returns error
	_, err = mgr.AddRuntime(context.Background(), filepath.Join(tempDir, "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func createMockAdoptiumZip(t *testing.T, subDir, javaExe, fullVersion string) ([]byte, string) {
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
	releaseContent := fmt.Sprintf("JAVA_VERSION=%q\nIMPLEMENTOR=\"Eclipse Adoptium\"\n", fullVersion)
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

func TestJavaManager_CheckRuntimeUpdates(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// 1. Create a simulated managed runtime folder with release file (version 21.0.2)
	managedRuntimeHome := filepath.Join(managedDir, "adoptium-21-21.0.2")
	managedBinDir := filepath.Join(managedRuntimeHome, "bin")
	if err := os.MkdirAll(managedBinDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedBinDir, javaExe), []byte("fake-bin"), 0755); err != nil {
		t.Fatalf("failed to write dummy binary: %v", err)
	}
	releaseContent := `JAVA_VERSION="21.0.2"
IMPLEMENTOR="Eclipse Adoptium"
`
	if err := os.WriteFile(filepath.Join(managedRuntimeHome, "release"), []byte(releaseContent), 0644); err != nil {
		t.Fatalf("failed to write release file: %v", err)
	}

	// 2. Mock Adoptium API returning latest version 21.0.3+9
	mockAPIResponse := `[
		{
			"binaries": [
				{
					"image_type": "jdk",
					"os": "windows",
					"architecture": "x64",
					"package": {
						"name": "OpenJDK21U-jdk_x64_windows_hotspot_21.0.3_9.zip",
						"link": "https://example.com/jdk21.zip",
						"checksum": "fake-checksum",
						"size": 12345
					}
				}
			],
			"version_data": {
				"semver": "21.0.3+9"
			}
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockAPIResponse)) // errcheck:ok mock response
	}))
	defer srv.Close()

	mgr := java.NewJavaManager(managedDir, nil, nil, srv.Client())
	client := java.NewAdoptiumClient(srv.URL, srv.Client())
	mgr.SetClient(client)

	updates, err := mgr.CheckRuntimeUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckRuntimeUpdates failed: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	u := updates[0]
	if u.MajorVersion != 21 {
		t.Errorf("expected major 21, got %d", u.MajorVersion)
	}
	if u.CurrentVersion != "21.0.2" {
		t.Errorf("expected current version '21.0.2', got %s", u.CurrentVersion)
	}
	if u.LatestVersion != "21.0.3+9" {
		t.Errorf("expected latest version '21.0.3+9', got %s", u.LatestVersion)
	}
	if !u.UpdateAvailable {
		t.Errorf("expected update_available=true")
	}
	if u.DownloadURL != "https://example.com/jdk21.zip" {
		t.Errorf("unexpected download URL: %s", u.DownloadURL)
	}
}

func TestJavaManager_UpgradeRuntime_RunningGuard(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	managedHome := filepath.Join(managedDir, "adoptium-21-21.0.2")
	managedBinDir := filepath.Join(managedHome, "bin")
	_ = os.MkdirAll(managedBinDir, 0755)
	managedBinary := filepath.Join(managedBinDir, javaExe)
	_ = os.WriteFile(managedBinary, []byte("fake-bin"), 0755)

	repo := &mockInstanceRepo{
		instances: []*domain.Instance{
			{
				ID:       "inst-running",
				Name:     "Running Instance",
				JavaPath: managedBinary,
				State:    domain.StateRunning,
			},
		},
	}

	mgr := java.NewJavaManager(managedDir, nil, repo, nil)

	_, err := mgr.UpgradeRuntime(context.Background(), 21)
	if err == nil {
		t.Fatal("expected error upgrading runtime with running instance, got nil")
	}
	if !strings.Contains(err.Error(), "Running Instance") || !strings.Contains(err.Error(), "running") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Test StateLaunching guard
	repo.instances[0].State = domain.StateLaunching
	_, err = mgr.UpgradeRuntime(context.Background(), 21)
	if err == nil {
		t.Fatal("expected error upgrading runtime with launching instance, got nil")
	}
}

func TestJavaManager_UpgradeRuntime_RelinksInstances(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// Old runtime adoptium-21-21.0.2
	oldHome := filepath.Join(managedDir, "adoptium-21-21.0.2")
	oldBinDir := filepath.Join(oldHome, "bin")
	_ = os.MkdirAll(oldBinDir, 0755)
	oldBinary := filepath.Join(oldBinDir, javaExe)
	_ = os.WriteFile(oldBinary, []byte("old-bin"), 0755)
	_ = os.WriteFile(filepath.Join(oldHome, "release"), []byte("JAVA_VERSION=\"21.0.2\"\n"), 0644)

	repo := &mockInstanceRepo{
		instances: []*domain.Instance{
			{
				ID:       "inst-1",
				Name:     "Test Instance",
				JavaPath: oldBinary,
				State:    domain.StateIdle,
			},
		},
	}

	// Prepare mock zip archive for new runtime 21.0.3+9
	zipBytes, zipSHA := createMockAdoptiumZip(t, "jdk-21.0.3+9", javaExe, "21.0.3")

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/assets/feature_releases/") {
			w.Header().Set("Content-Type", "application/json")
			resp := []map[string]any{
				{
					"binaries": []map[string]any{
						{
							"image_type":   "jdk",
							"os":           "windows",
							"architecture": "x64",
							"package": map[string]any{
								"name":     "OpenJDK21U-jdk.zip",
								"link":     srvURL + "/download/jdk21.zip",
								"checksum": zipSHA,
								"size":     len(zipBytes),
							},
						},
					},
					"version_data": map[string]any{
						"semver": "21.0.3+9",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp) // errcheck:ok mock response
			return
		}
		if strings.Contains(r.URL.Path, "/download/") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes) // errcheck:ok mock zip download
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	srvURL = srv.URL

	mgr := java.NewJavaManager(managedDir, nil, repo, srv.Client())
	client := java.NewAdoptiumClient(srv.URL, srv.Client())
	mgr.SetClient(client)
	prov := java.NewAdoptiumRuntimeService(managedDir, client, srv.Client())
	mgr.SetProvisioner(prov)

	newBin, err := mgr.UpgradeRuntime(context.Background(), 21)
	if err != nil {
		t.Fatalf("UpgradeRuntime failed: %v", err)
	}

	if _, statErr := os.Stat(newBin); statErr != nil {
		t.Fatalf("expected upgraded binary to exist at %s, got: %v", newBin, statErr)
	}

	// Verify repo instance has updated JavaPath
	if repo.instances[0].JavaPath != filepath.Clean(newBin) {
		t.Errorf("expected instance JavaPath updated to %s, got %s", newBin, repo.instances[0].JavaPath)
	}

	// Verify old runtime directory was cleaned up
	if _, statErr := os.Stat(oldHome); !os.IsNotExist(statErr) {
		t.Errorf("expected old runtime directory %s to be deleted, got err: %v", oldHome, statErr)
	}
}

func TestJavaManager_CleanUnusedRuntimes_Guards(t *testing.T) {
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managedDir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}

	// 1. Unused managed runtime (Java 21)
	unusedHome := filepath.Join(managedDir, "adoptium-21-21.0.2")
	_ = os.MkdirAll(filepath.Join(unusedHome, "bin"), 0755)
	unusedBin := filepath.Join(unusedHome, "bin", javaExe)
	_ = os.WriteFile(unusedBin, []byte("fake"), 0755)
	_ = os.WriteFile(filepath.Join(unusedHome, "release"), []byte("JAVA_VERSION=\"21.0.2\"\n"), 0644)

	// 2. Used managed runtime (Java 17)
	usedHome := filepath.Join(managedDir, "adoptium-17-17.0.10")
	_ = os.MkdirAll(filepath.Join(usedHome, "bin"), 0755)
	usedBin := filepath.Join(usedHome, "bin", javaExe)
	_ = os.WriteFile(usedBin, []byte("fake"), 0755)
	_ = os.WriteFile(filepath.Join(usedHome, "release"), []byte("JAVA_VERSION=\"17.0.10\"\n"), 0644)

	// 3. Detected runtime outside managed dir
	detectedHome := filepath.Join(tempDir, "system-jdk")
	_ = os.MkdirAll(filepath.Join(detectedHome, "bin"), 0755)
	detectedBin := filepath.Join(detectedHome, "bin", javaExe)
	_ = os.WriteFile(detectedBin, []byte("fake"), 0755)

	detector := &mockJavaDetector{
		installations: []ports.JavaInstallation{
			{
				Path:         detectedBin,
				HomeDir:      detectedHome,
				MajorVersion: 8,
				Kind:         "detected",
			},
		},
	}

	repo := &mockInstanceRepo{
		instances: []*domain.Instance{
			{
				ID:       "inst-used",
				Name:     "Used Inst",
				JavaPath: usedBin,
				State:    domain.StateIdle,
			},
		},
	}

	mgr := java.NewJavaManager(managedDir, detector, repo, nil)

	removed, err := mgr.CleanUnusedRuntimes(context.Background())
	if err != nil {
		t.Fatalf("CleanUnusedRuntimes failed: %v", err)
	}

	if len(removed) != 1 || filepath.Clean(removed[0]) != filepath.Clean(unusedBin) {
		t.Fatalf("expected removed [%s], got %+v", unusedBin, removed)
	}

	// Verify unused runtime is gone
	if _, statErr := os.Stat(unusedHome); !os.IsNotExist(statErr) {
		t.Errorf("expected unused home %s to be deleted", unusedHome)
	}

	// Verify used runtime is still present
	if _, statErr := os.Stat(usedHome); statErr != nil {
		t.Errorf("expected used home %s to still exist", usedHome)
	}

	// Verify detected runtime is still present
	if _, statErr := os.Stat(detectedHome); statErr != nil {
		t.Errorf("expected detected home %s to still exist", detectedHome)
	}
}
