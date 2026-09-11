package launch_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

func TestInstanceService_HeadlessLifecycle(t *testing.T) {
	mockTime := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clk := clock.NewMockClock(mockTime)
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)

	// Test validation: empty name
	_, err := svc.CreateInstance("", "1.21.1", domain.LoaderFabric)
	if !errors.Is(err, domain.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}

	// Test valid creation
	inst, err := svc.CreateInstance("Nord-Pack", "1.21.1", domain.LoaderFabric)
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	if inst.ID == "" || inst.Name != "Nord-Pack" {
		t.Fatalf("unexpected instance data: %+v", inst)
	}
	if inst.State != domain.StateIdle {
		t.Fatalf("expected StateIdle, got %s", inst.State)
	}

	// Test listing
	list := svc.ListInstances()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}

	// Test launch
	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("invalid pid returned: %d", pid)
	}

	updated, err := svc.GetInstance(inst.ID)
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if updated.State != domain.StateRunning {
		t.Fatalf("expected StateRunning, got %s", updated.State)
	}
	if updated.LastPlayedAt == nil {
		t.Fatal("expected LastPlayedAt to be recorded")
	}
}

func TestInstanceService_SQLiteStorageIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "launcher_integration.db")

	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	repo := storage.NewInstanceRepository(db)
	mockTime := time.Date(2026, 9, 11, 14, 0, 0, 0, time.UTC)
	clk := clock.NewMockClock(mockTime)
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(repo, fileSys, procMgr, kr, clk)

	// Create instance
	inst, err := svc.CreateInstance("Persistent-Pack", "1.21.1", domain.LoaderNeoForge)
	if err != nil {
		t.Fatalf("create instance failed: %v", err)
	}

	// Read back directly from SQLite repository to verify persistence
	persisted, err := repo.GetByID(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("failed to read persisted instance from db: %v", err)
	}
	if persisted.Name != "Persistent-Pack" || persisted.Loader != domain.LoaderNeoForge {
		t.Fatalf("mismatched persisted data: %+v", persisted)
	}

	// Launch and verify state update in database
	_, err = svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}

	persistedAfterLaunch, err := repo.GetByID(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("failed to read updated instance from db: %v", err)
	}
	if persistedAfterLaunch.State != domain.StateRunning {
		t.Fatalf("expected db state to be running, got %s", persistedAfterLaunch.State)
	}
	if persistedAfterLaunch.LastPlayedAt == nil {
		t.Fatal("expected db last_played_at to be populated")
	}
}

func TestInstanceService_LaunchWithSupervisor(t *testing.T) {
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()
	clk := clock.NewMockClock(time.Now())

	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)

	inst, err := svc.CreateInstance("Supervisor-Pack", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		t.Fatalf("create instance failed: %v", err)
	}

	acc := &domain.Account{
		UUID:        "test-uuid",
		Username:    "SupervisorSteve",
		Type:        domain.AccountOffline,
		AccessToken: "0",
	}

	vMeta := &launch.VersionJSON{
		ID:        "1.21.1",
		MainClass: "net.minecraft.client.main.Main",
	}

	cfg := launch.LaunchConfig{
		Instance:    inst,
		Account:     acc,
		VersionMeta: vMeta,
		GameDir:     t.TempDir(),
	}

	// For headless unit testing, we use a command that exits cleanly and outputs simulated log line
	var execName string
	if filepath.Separator == '\\' {
		execName = "cmd.exe"
		// Inject a fake Java command via custom JVM args to avoid failing when java is not configured
		inst.JVMArgs = []string{"/c", "echo [main/INFO]: Minecraft started successfully"}
	} else {
		execName = "echo"
	}

	handle, supervisor, err := svc.LaunchWithSupervisor(context.Background(), cfg, execName)
	if err != nil {
		t.Fatalf("LaunchWithSupervisor failed: %v", err)
	}

	if handle.PID() <= 0 {
		t.Fatalf("invalid PID: %d", handle.PID())
	}

	exitCode, err := handle.Wait()
	if err != nil {
		t.Fatalf("handle.Wait error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Give log supervisor a brief moment to process the pipe
	time.Sleep(50 * time.Millisecond)
	report := supervisor.AnalyzeCrash(exitCode)
	if report != nil {
		t.Fatalf("expected nil crash report on clean exit, got: %+v", report)
	}

	updated, _ := svc.GetInstance(inst.ID)
	if updated.State != domain.StateRunning {
		t.Fatalf("expected StateRunning, got %s", updated.State)
	}
}