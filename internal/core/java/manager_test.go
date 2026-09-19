package java_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
