package launch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/ports"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

type mockProcessHandle struct {
	pid      int
	exitCode int
	waitCh   chan struct{}
	err      error
}

func (m *mockProcessHandle) PID() int { return m.pid }
func (m *mockProcessHandle) Wait() (int, error) {
	if m.waitCh != nil {
		<-m.waitCh
	}
	return m.exitCode, m.err
}
func (m *mockProcessHandle) Kill() error {
	if m.waitCh != nil {
		select {
		case <-m.waitCh:
		default:
			close(m.waitCh)
		}
	}
	return nil
}

type mockProcessManager struct {
	handle   ports.ProcessHandle
	err      error
	lastArgs []string
}

func (m *mockProcessManager) StartProcess(
	ctx context.Context,
	executable string,
	args []string,
	dir string,
	env []string,
	stdout, stderr io.Writer,
) (ports.ProcessHandle, error) {
	m.lastArgs = args
	if m.err != nil {
		return nil, m.err
	}
	return m.handle, nil
}

type mockSessionRefresher struct {
	called bool
	acc    *domain.Account
}

func (r *mockSessionRefresher) RefreshSession(ctx context.Context, uuid string) (*domain.Account, error) {
	r.called = true
	return r.acc, nil
}

func TestInstanceService_HeadlessLifecycle(t *testing.T) {
	mockTime := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clk := clock.NewMockClock(mockTime)
	fileSys := fs.NewOSFileSystem()
	waitCh := make(chan struct{})
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 2468, exitCode: 0, waitCh: waitCh}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-steve",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-access-token",
		ExpiresAt:   mockTime.Add(2 * time.Hour),
	})

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
	if pid != 2468 {
		t.Fatalf("expected pid 2468, got %d", pid)
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

	// Terminate mock process and verify transition to Idle
	close(waitCh)
	time.Sleep(20 * time.Millisecond)
	finalInst, _ := svc.GetInstance(inst.ID)
	if finalInst.State != domain.StateIdle {
		t.Fatalf("expected StateIdle after process completion, got %s", finalInst.State)
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
	waitCh := make(chan struct{})
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 7777, exitCode: 0, waitCh: waitCh}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(repo, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-alex",
		Username:    "Alex",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
		ExpiresAt:   mockTime.Add(1 * time.Hour),
	})

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
	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if pid != 7777 {
		t.Fatalf("expected pid 7777, got %d", pid)
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

	close(waitCh)
	time.Sleep(20 * time.Millisecond)
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

func TestMonitorProcess_ExitZero(t *testing.T) {
	inst := &domain.Instance{
		ID:    "inst-zero",
		State: domain.StateRunning,
	}
	handle := &mockProcessHandle{pid: 100, exitCode: 0}
	var mu sync.RWMutex
	onCrashCalled := false

	launch.MonitorProcess(handle, inst, nil, func(id string, report *launch.CrashReport) {
		onCrashCalled = true
	}, nil, &mu)

	if inst.State != domain.StateIdle {
		t.Errorf("expected state Idle, got %s", inst.State)
	}
	if onCrashCalled {
		t.Error("expected onCrash NOT to be called on exit code 0")
	}
}

func TestMonitorProcess_ExitOne_Crash(t *testing.T) {
	inst := &domain.Instance{
		ID:    "inst-crash",
		State: domain.StateRunning,
	}
	handle := &mockProcessHandle{pid: 101, exitCode: 1}
	var mu sync.RWMutex
	var capturedReport *launch.CrashReport
	var capturedID string

	launch.MonitorProcess(handle, inst, nil, func(id string, report *launch.CrashReport) {
		capturedID = id
		capturedReport = report
	}, nil, &mu)

	if inst.State != domain.StateCrashed {
		t.Errorf("expected state Crashed, got %s", inst.State)
	}
	if capturedID != "inst-crash" {
		t.Errorf("expected crash instance ID 'inst-crash', got %s", capturedID)
	}
	if capturedReport == nil || capturedReport.ExitCode != 1 {
		t.Errorf("expected crash report with exit code 1, got %+v", capturedReport)
	}
}

func TestInstanceService_OfflineSupported(t *testing.T) {
	fileSys := fs.NewOSFileSystem()
	waitCh := make(chan struct{})
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 555, waitCh: waitCh}}
	kr := keyring.NewMemoryKeyring()
	clk := clock.NewMockClock(time.Now())

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "offline-uuid-123",
		Username:    "OfflinePlayer",
		Type:        domain.AccountOffline,
		AccessToken: "0",
	})

	inst, err := svc.CreateInstance("Offline-Pack", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed for offline account: %v", err)
	}
	if pid != 555 {
		t.Errorf("expected PID 555, got %d", pid)
	}
	if inst.State != domain.StateRunning {
		t.Errorf("expected instance state Running, got %s", inst.State)
	}

	hasTokenZero := false
	for i, arg := range mockProc.lastArgs {
		if arg == "--accessToken" && i+1 < len(mockProc.lastArgs) && mockProc.lastArgs[i+1] == "0" {
			hasTokenZero = true
			break
		}
	}
	if !hasTokenZero {
		t.Errorf("expected --accessToken 0 in arguments, got: %v", mockProc.lastArgs)
	}

	close(waitCh)
}

func TestInstanceService_NoActiveAccount(t *testing.T) {
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 555}}
	kr := keyring.NewMemoryKeyring()
	clk := clock.NewMockClock(time.Now())

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	// No active account set

	inst, err := svc.CreateInstance("NoAcc-Pack", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.Launch(context.Background(), inst.ID)
	if !errors.Is(err, domain.ErrNoActiveAccount) {
		t.Errorf("expected ErrNoActiveAccount, got %v", err)
	}
}

func TestInstanceService_TokenRefreshBeforeLaunch(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 8888}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)

	expiredAcc := &domain.Account{
		UUID:        "uuid-expired",
		Username:    "ExpiredPlayer",
		Type:        domain.AccountMicrosoft,
		AccessToken: "old-token",
		ExpiresAt:   now.Add(2 * time.Minute), // within 5 min window!
	}
	svc.SetActiveAccount(expiredAcc)

	refreshedAcc := &domain.Account{
		UUID:        "uuid-expired",
		Username:    "ExpiredPlayer",
		Type:        domain.AccountMicrosoft,
		AccessToken: "fresh-token",
		ExpiresAt:   now.Add(24 * time.Hour),
	}
	refresher := &mockSessionRefresher{acc: refreshedAcc}
	svc.SetSessionRefresher(refresher)

	inst, err := svc.CreateInstance("Refresh-Pack", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if pid != 8888 {
		t.Errorf("expected pid 8888, got %d", pid)
	}
	if !refresher.called {
		t.Error("expected RefreshSession to be called when within 5 minutes of expiration")
	}
}

func TestInstanceService_ErrorsAndEdgeCases(t *testing.T) {
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()
	clk := clock.NewMockClock(time.Now())

	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)

	// 1. GetInstance Not Found
	_, err := svc.GetInstance("non-existent-inst")
	if !errors.Is(err, domain.ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound, got %v", err)
	}

	// 2. Launch Not Found
	_, err = svc.Launch(context.Background(), "non-existent-inst")
	if !errors.Is(err, domain.ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound, got %v", err)
	}
}

type mockGameProvisioner struct {
	cfg *domain.LaunchConfig
	err error
}

func (p *mockGameProvisioner) Provision(ctx context.Context, inst *domain.Instance, acc *domain.Account) (*domain.LaunchConfig, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.cfg, nil
}

type mockJavaDetector struct {
	installs []ports.JavaInstallation
	err      error
}

func (d *mockJavaDetector) DetectInstallations(ctx context.Context) ([]ports.JavaInstallation, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.installs, nil
}

func TestInstanceService_Launch_WithProvisioner(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	waitCh := make(chan struct{})
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 9999, exitCode: 0, waitCh: waitCh}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	acc := &domain.Account{
		UUID:        "uuid-steve",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
		ExpiresAt:   now.Add(2 * time.Hour),
	}
	svc.SetActiveAccount(acc)

	inst, _ := svc.CreateInstance("Prov-Pack", "1.21.1", domain.LoaderVanilla)

	provCfg := &domain.LaunchConfig{
		Instance:    inst,
		Account:     acc,
		VersionMeta: &domain.VersionJSON{ID: "1.21.1", MainClass: "net.minecraft.client.main.Main"},
		GameDir:     t.TempDir(),
	}

	// 1. Success branch
	svc.SetProvisioner(&mockGameProvisioner{cfg: provCfg})
	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch with provisioner failed: %v", err)
	}
	if pid != 9999 {
		t.Errorf("expected pid 9999, got %d", pid)
	}
	close(waitCh)

	// 2. Error branch
	svc.SetProvisioner(&mockGameProvisioner{err: errors.New("download failed")})
	_, err = svc.Launch(context.Background(), inst.ID)
	if err == nil {
		t.Fatal("expected provision error, got nil")
	}
}

func TestInstanceService_Launch_WithJavaDetector(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	waitCh := make(chan struct{})
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 1111, exitCode: 0, waitCh: waitCh}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-steve",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
		ExpiresAt:   now.Add(2 * time.Hour),
	})

	inst, _ := svc.CreateInstance("JavaDetect-Pack", "1.21.1", domain.LoaderVanilla)

	detector := &mockJavaDetector{
		installs: []ports.JavaInstallation{
			{Path: "/custom/java21/bin/java", MajorVersion: 21},
			{Path: "/custom/java17/bin/java", MajorVersion: 17},
		},
	}
	svc.SetJavaDetector(detector)
	svc.SetOnCrash(func(id string, report *launch.CrashReport) {})

	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if pid != 1111 {
		t.Errorf("expected pid 1111, got %d", pid)
	}
	close(waitCh)
}

func TestInstanceService_Launch_FailClosed_IncompatibleJava(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 2222, exitCode: 0}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-alex",
		Username:    "Alex",
		Type:        domain.AccountOffline,
		AccessToken: "mock-offline",
	})

	inst, _ := svc.CreateInstance("Incompatible-Java-Pack", "1.21.1", domain.LoaderVanilla)

	// Only Java 8 and Java 17 available, but 1.21.1 requires Java 21!
	detector := &mockJavaDetector{
		installs: []ports.JavaInstallation{
			{Path: "/custom/java8/bin/java", MajorVersion: 8},
			{Path: "/custom/java17/bin/java", MajorVersion: 17},
		},
	}
	svc.SetJavaDetector(detector)

	_, err := svc.Launch(context.Background(), inst.ID)
	if err == nil {
		t.Fatal("expected fail-closed error for incompatible Java version, got nil")
	}
	expectedSubstring := "Java 21 required for Minecraft 1.21.1"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Fatalf("expected error containing %q, got: %v", expectedSubstring, err)
	}
}

func TestInstanceService_Launch_FailClosed_NoJava(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 3333, exitCode: 0}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-alex",
		Username:    "Alex",
		Type:        domain.AccountOffline,
		AccessToken: "mock-offline",
	})

	inst, _ := svc.CreateInstance("No-Java-Pack", "1.21.1", domain.LoaderVanilla)

	detector := &mockJavaDetector{
		installs: []ports.JavaInstallation{},
	}
	svc.SetJavaDetector(detector)

	_, err := svc.Launch(context.Background(), inst.ID)
	if err == nil {
		t.Fatal("expected fail-closed error for empty Java detector, got nil")
	}
	expectedSubstring := "no compatible Java 21 installation found for Minecraft 1.21.1"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Fatalf("expected error containing %q, got: %v", expectedSubstring, err)
	}
}

func TestInstanceService_UpdateInstance_FullUpsertPreservesFields(t *testing.T) {
	tempDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tempDir, "nord.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	instRepo := storage.NewInstanceRepository(db)
	now := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(instRepo, fileSys, mockProc, kr, clk)

	// Create initial instance
	created, err := svc.CreateInstance("Original-Pack", "1.20.1", domain.LoaderFabric)
	if err != nil {
		t.Fatalf("create instance failed: %v", err)
	}

	// Advance clock
	nowLater := now.Add(1 * time.Hour)
	clk.CurrentTime = nowLater

	// Update instance with custom JavaPath, RAM, JVM args, and SkipJavaCheck
	skipVal := true
	updated, err := svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
		ID:            created.ID,
		Name:          "Renamed-Pack",
		JavaPath:      "C:\\Java21\\bin\\java.exe",
		MinRAMMB:      1024,
		MaxRAMMB:      6144,
		JVMArgs:       []string{"-XX:+UseG1GC", "-Duser.language=ru"},
		SkipJavaCheck: &skipVal,
	})
	if err != nil {
		t.Fatalf("update instance failed: %v", err)
	}

	if updated.Name != "Renamed-Pack" {
		t.Errorf("expected updated name %q, got %q", "Renamed-Pack", updated.Name)
	}
	if updated.JavaPath != "C:\\Java21\\bin\\java.exe" {
		t.Errorf("expected updated JavaPath, got %q", updated.JavaPath)
	}
	if updated.MinRAMMB != 1024 || updated.MaxRAMMB != 6144 {
		t.Errorf("expected updated RAM min=1024 max=6144, got min=%d max=%d", updated.MinRAMMB, updated.MaxRAMMB)
	}
	if len(updated.JVMArgs) != 2 || updated.JVMArgs[0] != "-XX:+UseG1GC" {
		t.Errorf("expected updated JVMArgs, got %+v", updated.JVMArgs)
	}
	if !updated.SkipJavaCheck {
		t.Errorf("expected updated SkipJavaCheck=true, got false")
	}
	if !updated.UpdatedAt.Equal(nowLater) {
		t.Errorf("expected UpdatedAt %v, got %v", nowLater, updated.UpdatedAt)
	}

	// Load from SQLite repository to verify persistence & preserved fields
	persisted, err := instRepo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if persisted.Name != "Renamed-Pack" || persisted.JavaPath != "C:\\Java21\\bin\\java.exe" {
		t.Errorf("persisted fields mismatch: name=%q java=%q", persisted.Name, persisted.JavaPath)
	}
	if persisted.GameVersion != "1.20.1" || persisted.Loader != domain.LoaderFabric {
		t.Errorf("original fields corrupted: version=%q loader=%q", persisted.GameVersion, persisted.Loader)
	}
	if persisted.MinRAMMB != 1024 || persisted.MaxRAMMB != 6144 {
		t.Errorf("persisted RAM settings corrupted: min=%d max=%d", persisted.MinRAMMB, persisted.MaxRAMMB)
	}
	if len(persisted.JVMArgs) != 2 || persisted.JVMArgs[1] != "-Duser.language=ru" {
		t.Errorf("persisted JVMArgs corrupted: %+v", persisted.JVMArgs)
	}
	if !persisted.SkipJavaCheck {
		t.Errorf("persisted SkipJavaCheck corrupted: got false")
	}
}

func createFakeJava(t *testing.T, major int) string {
	tmpDir := t.TempDir()
	var fakeJavaPath string
	if runtime.GOOS == "windows" {
		fakeJavaPath = filepath.Join(tmpDir, "java.cmd")
		content := fmt.Sprintf("@echo openjdk version \"%d.0.2\" 2022-01-18>&2\r\n", major)
		if err := os.WriteFile(fakeJavaPath, []byte(content), 0755); err != nil {
			t.Fatalf("failed to write fake java: %v", err)
		}
	} else {
		fakeJavaPath = filepath.Join(tmpDir, "java")
		content := fmt.Sprintf("#!/bin/sh\necho 'openjdk version \"%d.0.2\" 2022-01-18' >&2\n", major)
		if err := os.WriteFile(fakeJavaPath, []byte(content), 0755); err != nil {
			t.Fatalf("failed to write fake java: %v", err)
		}
	}
	return fakeJavaPath
}

func TestInstanceService_Launch_SkipJavaCheck(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 7777, exitCode: 0}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	svc.SetActiveAccount(&domain.Account{
		UUID:        "uuid-steve",
		Username:    "Steve",
		Type:        domain.AccountOffline,
		AccessToken: "mock-offline",
	})

	// 1.21.1 requires Java 21; we configure fake Java 17
	fakeJava17 := createFakeJava(t, 17)
	inst, err := svc.CreateInstance("Fabric-1.21", "1.21.1", domain.LoaderFabric)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// 1. With SkipJavaCheck = false (default), Launch fails closed referencing Java Manager
	_, err = svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
		ID:       inst.ID,
		JavaPath: fakeJava17,
	})
	if err != nil {
		t.Fatalf("UpdateInstance failed: %v", err)
	}

	_, err = svc.Launch(context.Background(), inst.ID)
	if err == nil {
		t.Fatal("expected fail-closed error for Java 17 on Minecraft 1.21.1, got nil")
	}
	if !strings.Contains(err.Error(), "Java 21 required for Minecraft 1.21.1 (found Java 17 at ") {
		t.Errorf("error did not contain expected version mismatch prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "install Temurin 21 via Java Manager") {
		t.Errorf("error did not contain reference to Java Manager: %v", err)
	}

	// 2. With SkipJavaCheck = true, Launch bypasses major mismatch
	skipCheck := true
	_, err = svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
		ID:            inst.ID,
		SkipJavaCheck: &skipCheck,
	})
	if err != nil {
		t.Fatalf("UpdateInstance with SkipJavaCheck failed: %v", err)
	}

	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("expected Launch to succeed with SkipJavaCheck=true, got error: %v", err)
	}
	if pid != 7777 {
		t.Errorf("expected PID 7777, got %d", pid)
	}

	// 3. Even with SkipJavaCheck = true, a non-existent or unexecutable JavaPath fails closed
	_, err = svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
		ID:       inst.ID,
		JavaPath: filepath.Join(t.TempDir(), "nonexistent_java.exe"),
	})
	if err != nil {
		t.Fatalf("UpdateInstance with bad path failed: %v", err)
	}

	_, err = svc.Launch(context.Background(), inst.ID)
	if err == nil {
		t.Fatal("expected error executing non-existent Java executable, got nil")
	}
	if !strings.Contains(err.Error(), "failed to execute configured Java") {
		t.Errorf("expected 'failed to execute configured Java', got: %v", err)
	}
}

func TestInstanceService_Setters(t *testing.T) {
	svc := launch.NewInstanceService(nil, nil, nil, nil, nil)
	svc.SetProvisioner(nil)
	svc.SetJavaDetector(nil)
	svc.SetAccountRepository(nil)
	svc.SetActiveAccount(nil)
	svc.SetSessionRefresher(nil)
	svc.SetOnCrash(nil)
}

func TestLogSupervisor_GetTail(t *testing.T) {
	sup := launch.NewLogSupervisor(10)
	// Empty buffer returns empty slice
	if tail := sup.GetTail(5); len(tail) != 0 {
		t.Errorf("expected empty tail, got: %+v", tail)
	}

	// Add 5 lines
	for i := 1; i <= 5; i++ {
		sup.ProcessLine(fmt.Sprintf("line %d", i))
	}

	// Request last 3 lines
	tail := sup.GetTail(3)
	if len(tail) != 3 || tail[0] != "line 3" || tail[2] != "line 5" {
		t.Errorf("expected lines 3, 4, 5, got %+v", tail)
	}

	// Request more lines than buffer has (n = 10, len = 5)
	allTail := sup.GetTail(10)
	if len(allTail) != 5 || allTail[0] != "line 1" {
		t.Errorf("expected all 5 lines, got %+v", allTail)
	}

	// Request n <= 0 returns all
	zeroTail := sup.GetTail(0)
	if len(zeroTail) != 5 {
		t.Errorf("expected 5 lines for n=0, got %d", len(zeroTail))
	}
}

func TestInstanceService_GetLogTail(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 9999, exitCode: 0}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)

	// 1. Idle instance without supervisor returns empty slice, nil error
	tail, err := svc.GetLogTail("no-such-id", 50)
	if err != nil {
		t.Fatalf("expected nil error on idle GetLogTail, got %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("expected empty slice, got %+v", tail)
	}
}

func TestInstanceService_SetInstanceFavorite_And_Group(t *testing.T) {
	now := time.Now()
	clk := clock.NewMockClock(now)
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcessManager{handle: &mockProcessHandle{pid: 9999, exitCode: 0}}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	inst, err := svc.CreateInstance("Favorites Test", "1.21.1", domain.LoaderFabric)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	updated, err := svc.SetInstanceFavorite(context.Background(), inst.ID, true)
	if err != nil {
		t.Fatalf("SetInstanceFavorite failed: %v", err)
	}
	if !updated.IsFavorite {
		t.Errorf("expected IsFavorite to be true")
	}

	updated, err = svc.SetInstanceGroup(context.Background(), inst.ID, "Speedrun")
	if err != nil {
		t.Fatalf("SetInstanceGroup failed: %v", err)
	}
	if updated.Group != "Speedrun" {
		t.Errorf("expected Group to be Speedrun, got %s", updated.Group)
	}

	sup := svc.GetSupervisor(inst.ID)
	if sup != nil {
		t.Errorf("expected nil supervisor for idle instance")
	}
}