package wails_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/ports"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

type mockProcHandle struct{}

func (m *mockProcHandle) PID() int           { return 12345 }
func (m *mockProcHandle) Wait() (int, error) { return 0, nil }
func (m *mockProcHandle) Kill() error        { return nil }

type mockProcMgr struct{}

func (m *mockProcMgr) StartProcess(
	ctx context.Context,
	executable string,
	args []string,
	dir string,
	env []string,
	stdout, stderr io.Writer,
) (ports.ProcessHandle, error) {
	return &mockProcHandle{}, nil
}

func TestWailsAdapter_IPCBridge(t *testing.T) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	procMgr := &mockProcMgr{}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)
	adapter := wails.NewWailsAdapter(svc)

	// Test create via IPC DTO
	req := wails.CreateInstanceRequest{
		Name:        "Nord-Test",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	}

	dto, err := adapter.CreateInstance(req)
	if err != nil {
		t.Fatalf("unexpected error creating instance: %v", err)
	}
	if dto.Name != "Nord-Test" || dto.State != "idle" {
		t.Fatalf("unexpected DTO: %+v", dto)
	}

	// Test list via IPC
	list := adapter.ListInstances()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}

	// Test launch without active account -> fails cleanly
	res, err := adapter.LaunchInstance(dto.ID)
	if err != nil {
		t.Fatalf("launch error: %v", err)
	}
	if res.Success || !strings.Contains(res.Error, "no active account") {
		t.Fatalf("expected failure without account, got: %+v", res)
	}

	// Set active account and launch again -> succeeds
	svc.SetActiveAccount(&domain.Account{
		UUID:        "test-uuid",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})

	res, err = adapter.LaunchInstance(dto.ID)
	if err != nil {
		t.Fatalf("launch error: %v", err)
	}
	if !res.Success || res.PID != 12345 {
		t.Fatalf("launch failed: %+v", res)
	}
}

func TestWailsAdapter_AccountsAndMods(t *testing.T) {
	tempDir := t.TempDir()
	db, err := storage.OpenDatabase(filepath.Join(tempDir, "adapter_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	accRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()
	authSvc := auth.NewAuthService("test-client", nil, accRepo, kr)

	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	clk := clock.NewMockClock(time.Now())
	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)

	adapter := wails.NewWailsAdapter(svc)
	adapter.SetAuth(authSvc, accRepo)
	adapter.SetFileSystem(fileSys, tempDir)

	// 1. Offline login
	accDTO, err := adapter.LoginOffline("Tester")
	if err != nil {
		t.Fatalf("login offline failed: %v", err)
	}
	if accDTO.Username != "Tester" || !accDTO.IsActive {
		t.Fatalf("unexpected acc DTO: %+v", accDTO)
	}

	// 2. List accounts
	accs, err := adapter.ListAccounts()
	if err != nil || len(accs) != 1 {
		t.Fatalf("expected 1 account, got %v (err: %v)", len(accs), err)
	}

	// 3. Mods management test
	instanceID := "test-inst"
	modsDir := filepath.Join(tempDir, instanceID, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("mkdir mods: %v", err)
	}

	modFile := filepath.Join(modsDir, "sodium.jar")
	if err := os.WriteFile(modFile, []byte("fake-jar"), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}

	// List installed mods
	installed, err := adapter.ListInstalledMods(instanceID)
	if err != nil || len(installed) != 1 {
		t.Fatalf("expected 1 installed mod, got %d (err: %v)", len(installed), err)
	}
	if !installed[0].Enabled || installed[0].Name != "sodium" {
		t.Fatalf("unexpected installed mod: %+v", installed[0])
	}

	// Toggle disable
	err = adapter.ToggleMod(wails.ToggleModRequest{
		InstanceID: instanceID,
		FileName:   "sodium.jar",
		Enable:     false,
	})
	if err != nil {
		t.Fatalf("toggle mod failed: %v", err)
	}

	installedAfterToggle, _ := adapter.ListInstalledMods(instanceID)
	if len(installedAfterToggle) != 1 || installedAfterToggle[0].Enabled {
		t.Fatalf("expected mod to be disabled: %+v", installedAfterToggle)
	}

	// Delete mod
	err = adapter.DeleteMod(wails.DeleteModRequest{
		InstanceID: instanceID,
		FileName:   "sodium.jar.disabled",
	})
	if err != nil {
		t.Fatalf("delete mod failed: %v", err)
	}

	installedAfterDelete, _ := adapter.ListInstalledMods(instanceID)
	if len(installedAfterDelete) != 0 {
		t.Fatalf("expected 0 mods after delete, got %d", len(installedAfterDelete))
	}

	// 4. Crash report recording
	adapter.RecordCrash(instanceID, &launch.CrashReport{
		Category: launch.CrashCategoryOOM,
		Summary:  "Out of Memory",
		Remedy:   "Increase RAM",
		ExitCode: 1,
	})
	report := adapter.GetLastCrashReport(instanceID)
	if report == nil || report.Category != "out_of_memory" {
		t.Fatalf("unexpected crash report: %+v", report)
	}
}

var benchOnce sync.Once

func BenchmarkWailsAdapter_IPCDispatch(b *testing.B) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()
	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)
	adapter := wails.NewWailsAdapter(svc)

	req := wails.CreateInstanceRequest{
		Name:        "Bench-Instance",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	}
	_, err := adapter.CreateInstance(req)
	if err != nil {
		b.Fatalf("create failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adapter.ListInstances()
	}
	b.StopTimer()

	// Calculate and report true p95 percentile over 10,000 iterations (O1)
	benchOnce.Do(func() {
		const sampleCount = 10000
		samples := make([]int64, sampleCount)
		for i := 0; i < sampleCount; i++ {
			t0 := time.Now()
			_ = adapter.ListInstances()
			samples[i] = time.Since(t0).Nanoseconds()
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		p95 := samples[int(float64(sampleCount)*0.95)]
		fmt.Printf("# p95: %d ns\n", p95)
	})
}