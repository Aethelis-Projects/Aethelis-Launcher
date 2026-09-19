package wails_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/nord-launcher/launcher/internal/core/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
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

	// 5. RestartApplication with mock relauncher
	relaunchCalled := false
	adapter.SetRelauncher(func() error {
		relaunchCalled = true
		return nil
	})
	if err := adapter.RestartApplication(); err != nil {
		t.Fatalf("unexpected relaunch error: %v", err)
	}
	if !relaunchCalled {
		t.Errorf("expected relauncher to be invoked")
	}

	// Error propagation
	adapter.SetRelauncher(func() error {
		return errors.New("mock relaunch failure")
	})
	if err := adapter.RestartApplication(); err == nil || err.Error() != "mock relaunch failure" {
		t.Fatalf("expected mock relaunch failure, got %v", err)
	}
}

func TestWailsAdapter_Updater(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)

	// 1. Uninitialized updater returns error
	_, err := adapter.CheckForUpdates()
	if err == nil || !strings.Contains(err.Error(), "auto-updater not initialized") {
		t.Fatalf("expected uninitialized updater error, got %v", err)
	}
	_, err = adapter.ApplyUpdate()
	if err == nil || !strings.Contains(err.Error(), "auto-updater not initialized") {
		t.Fatalf("expected uninitialized updater error on apply, got %v", err)
	}

	// 2. Setup mock server with UpdateManifest
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	payload := []byte("nord-launcher-binary-content-mock")
	sig := ed25519.Sign(privKey, payload)
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	now := time.Now().UTC().Truncate(time.Second)
	platKey := updater.CurrentPlatformKey()

	var manifestServer *httptest.Server
	manifest := updater.UpdateManifest{
		Version:     "0.2.0",
		ReleaseDate: now,
		Changelog:   "Nord Launcher v0.2.0 release notes.",
		Platforms: map[string]updater.PlatformAsset{
			platKey: {
				URL:       "",
				Signature: sigB64,
				Size:      int64(len(payload)),
			},
		},
	}

	manifestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer manifestServer.Close()

	asset := manifest.Platforms[platKey]
	asset.URL = manifestServer.URL + "/binary"
	manifest.Platforms[platKey] = asset

	u := updater.NewAutoUpdater("0.1.2", manifestServer.URL, pubKey, manifestServer.Client())
	adapter.SetUpdater(u)

	// 3. Check for updates -> update available
	info, err := adapter.CheckForUpdates()
	if err != nil {
		t.Fatalf("check for updates failed: %v", err)
	}
	if !info.HasUpdate {
		t.Fatalf("expected HasUpdate = true")
	}
	if info.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", info.Version)
	}
	if info.CurrentVersion != "0.1.2" {
		t.Errorf("expected current version 0.1.2, got %s", info.CurrentVersion)
	}
	if !info.ReleaseDate.Equal(now) {
		t.Errorf("expected release date %v, got %v", now, info.ReleaseDate)
	}
	if info.ReleaseNotes != "Nord Launcher v0.2.0 release notes." {
		t.Errorf("expected release notes match, got %s", info.ReleaseNotes)
	}

	// 4. Check for updates -> up to date
	manifestUpToDate := manifest
	manifestUpToDate.Version = "0.1.2"
	serverUpToDate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifestUpToDate)
	}))
	defer serverUpToDate.Close()

	uSame := updater.NewAutoUpdater("0.1.2", serverUpToDate.URL, pubKey, serverUpToDate.Client())
	adapter.SetUpdater(uSame)

	infoSame, err := adapter.CheckForUpdates()
	if err != nil {
		t.Fatalf("check for updates on same version failed: %v", err)
	}
	if infoSame.HasUpdate {
		t.Fatalf("expected HasUpdate = false")
	}
	if infoSame.CurrentVersion != "0.1.2" {
		t.Errorf("expected CurrentVersion 0.1.2, got %s", infoSame.CurrentVersion)
	}

	// 5. Apply update when no update available
	resNoUpdate, err := adapter.ApplyUpdate()
	if err != nil {
		t.Fatalf("unexpected error on apply without update: %v", err)
	}
	if resNoUpdate.Success || resNoUpdate.Message != "No update available" {
		t.Errorf("expected success=false, message='No update available', got %+v", resNoUpdate)
	}
}

func TestWailsAdapter_WailsV3BindingsRegistration(t *testing.T) {
	// Initialize global application if not already initialized
	_ = application.New(application.Options{})

	bindings := application.NewBindings(nil, nil)
	adapter := wails.NewWailsAdapter(nil)
	err := bindings.Add(application.NewService(adapter))
	if err != nil {
		t.Fatalf("bindings.Add failed: %v", err)
	}

	expectedMethods := []string{
		"CheckForUpdates",
		"ApplyUpdate",
		"RestartApplication",
		"GetCurrentVersion",
		"LoginMicrosoft",
		"LoginOffline",
		"ListInstances",
		"CreateInstance",
		"LaunchInstance",
		"ListAccounts",
		"SetActiveAccount",
		"SearchMods",
		"ListInstalledMods",
		"ToggleMod",
		"DeleteMod",
		"GetLastCrashReport",
		"InstallMod",
		"GetSettings",
		"SetSetting",
	}

	const prefix = "github.com/nord-launcher/launcher/internal/adapters/wails.WailsAdapter."

	for _, methodName := range expectedMethods {
		fqn := prefix + methodName
		method := bindings.Get(&application.CallOptions{
			MethodName: fqn,
		})
		if method == nil {
			t.Errorf("Method %s was not registered in Wails v3 bindings (expected FQN: %s)", methodName, fqn)
		}
	}
}

func TestWailsAdapter_WailsV3BindingCall_CheckForUpdates(t *testing.T) {
	manifest := updater.UpdateManifest{
		Version:     "0.1.4",
		ReleaseDate: time.Now(),
		Changelog:   "Test v0.1.4 changelog",
		Platforms: map[string]updater.PlatformAsset{
			updater.CurrentPlatformKey(): {
				URL:       "http://example.com/asset.exe",
				SHA256:    "abcd",
				Signature: "sig",
				Size:      12345,
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	u := updater.NewAutoUpdater("0.1.3", server.URL, updater.GetDefaultPublicKey(), server.Client())
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetUpdater(u)

	bindings := application.NewBindings(nil, nil)
	_ = bindings.Add(application.NewService(adapter))

	method := bindings.Get(&application.CallOptions{
		MethodName: "github.com/nord-launcher/launcher/internal/adapters/wails.WailsAdapter.CheckForUpdates",
	})
	if method == nil {
		t.Fatalf("method not found")
	}

	result, err := method.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	dto, ok := result.(*wails.UpdateInfoDTO)
	if !ok {
		t.Fatalf("expected *UpdateInfoDTO, got %T", result)
	}
	if !dto.HasUpdate || dto.Version != "0.1.4" || dto.CurrentVersion != "0.1.3" {
		t.Fatalf("unexpected DTO: %+v", dto)
	}
}

func TestWailsAdapter_GetCurrentVersion(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetVersion("v0.1.6")

	if ver := adapter.GetCurrentVersion(); ver != "0.1.6" {
		t.Fatalf("expected version 0.1.6, got %q", ver)
	}

	bindings := application.NewBindings(nil, nil)
	_ = bindings.Add(application.NewService(adapter))

	method := bindings.Get(&application.CallOptions{
		MethodName: "github.com/nord-launcher/launcher/internal/adapters/wails.WailsAdapter.GetCurrentVersion",
	})
	if method == nil {
		t.Fatalf("method GetCurrentVersion not found in bindings")
	}

	result, err := method.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	verStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if verStr != "0.1.6" {
		t.Fatalf("expected 0.1.6, got %q", verStr)
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

	// Calculate and report true p95 percentile over 100 batches of 50,000 calls (O1 / P3)
	benchOnce.Do(func() {
		const sampleCount = 100
		const batch = 50000
		samples := make([]int64, sampleCount)
		for i := 0; i < sampleCount; i++ {
			t0 := time.Now()
			for j := 0; j < batch; j++ {
				_ = adapter.ListInstances()
			}
			samples[i] = time.Since(t0).Nanoseconds() / batch
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		p95 := samples[int(float64(sampleCount)*0.95)]
		fmt.Printf("# p95: %d ns\n", p95)
	})
}