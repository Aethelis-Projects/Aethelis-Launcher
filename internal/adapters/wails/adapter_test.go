package wails_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

func TestWailsAdapter_IPCBridge(t *testing.T) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
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

	// Test launch via IPC
	res, err := adapter.LaunchInstance(dto.ID)
	if err != nil {
		t.Fatalf("launch error: %v", err)
	}
	if !res.Success || res.PID <= 0 {
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