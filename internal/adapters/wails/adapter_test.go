package wails_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/manifest"
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
		"HasBuiltinCurseForgeKey",
		"UpdateInstance",
		"GetLogTail",
		"ListJavaRuntimes",
		"DownloadJavaRuntime",
		"GetJavaDownloadStatus",
		"RemoveJavaRuntime",
		"AddJavaRuntime",
		"ListModVersions",
		"GetModInstallStatus",
		"CheckModUpdates",
		"GetDiagnosticReport",
		"PickMrPackFile",
		"GetMrPackImportPlan",
		"ImportMrPack",
		"GetMrPackImportStatus",
		"ExportMrPack",
		"CheckJavaRuntimeUpdates",
		"UpgradeJavaRuntime",
		"UpdateMod",
		"OpenPath",
	}

	if len(expectedMethods) != 40 {
		t.Fatalf("expected exactly 40 Wails methods, got %d", len(expectedMethods))
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

func TestWailsAdapter_Settings_CurseForgeKey(t *testing.T) {
	db, err := storage.Open("file:settings_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	settingsRepo := storage.NewSettingsRepository(db)
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetSettings(settingsRepo)

	cf := curseforge.NewClient("http://127.0.0.1:0", "", nil)
	adapter.SetContent(nil, cf)

	// 1. Initial settings empty
	initSettings, err := adapter.GetSettings()
	if err != nil {
		t.Fatalf("get settings error: %v", err)
	}
	if len(initSettings.Settings) != 0 {
		t.Fatalf("expected empty settings initially, got %+v", initSettings.Settings)
	}

	// 2. Set CurseForge key
	err = adapter.SetSetting(wails.SetSettingRequest{
		Key:   "curseforge_api_key",
		Value: "cf-test-key-12345",
	})
	if err != nil {
		t.Fatalf("set setting error: %v", err)
	}

	if cf.APIKey() != "cf-test-key-12345" {
		t.Fatalf("expected cf client key 'cf-test-key-12345', got %q", cf.APIKey())
	}

	gotSettings, err := adapter.GetSettings()
	if err != nil {
		t.Fatalf("get settings error: %v", err)
	}
	if gotSettings.Settings["curseforge_api_key"] != "" {
		t.Fatalf("expected masked empty curseforge_api_key, got %q", gotSettings.Settings["curseforge_api_key"])
	}
	if gotSettings.Settings["has_curseforge_api_key"] != "true" {
		t.Fatalf("expected has_curseforge_api_key == 'true', got %q", gotSettings.Settings["has_curseforge_api_key"])
	}

	// 3. Clear key restores empty / fast-path
	err = adapter.SetSetting(wails.SetSettingRequest{
		Key:   "curseforge_api_key",
		Value: "",
	})
	if err != nil {
		t.Fatalf("clear setting error: %v", err)
	}

	if cf.APIKey() != "" {
		t.Fatalf("expected empty cf key after clear, got %q", cf.APIKey())
	}
}

func TestWailsAdapter_InstallMod_Modrinth_Success(t *testing.T) {
	tempDir := t.TempDir()
	modData := []byte("PK\x03\x04test-modrinth-mod-bytes")
	hSha1 := sha1.Sum(modData)
	sha1Hex := hex.EncodeToString(hSha1[:])

	var downloadRequested bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/test-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-1",
					"project_id":     "test-mod",
					"version_number": "1.0.0",
					"name":           "Test Mod 1.0.0",
					"game_versions":  []string{"1.21"},
					"loaders":        []string{"fabric"},
					"files": []map[string]interface{}{
						{
							"hashes": map[string]string{
								"sha1": sha1Hex,
							},
							"url":      fmt.Sprintf("http://%s/download/test-mod-1.0.0.jar", r.Host),
							"filename": "test-mod-1.0.0.jar",
							"primary":  true,
							"size":     len(modData),
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/test-mod-1.0.0.jar" {
			downloadRequested = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(modData)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "test-mod",
		Source:     "modrinth",
	})
	if err != nil {
		t.Fatalf("InstallMod returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %+v", res)
	}
	if res.FileName != "test-mod-1.0.0.jar" {
		t.Fatalf("expected filename 'test-mod-1.0.0.jar', got %q", res.FileName)
	}
	if !downloadRequested {
		t.Fatalf("expected download request to be served")
	}

	installedPath := filepath.Join(tempDir, "inst-test", "mods", "test-mod-1.0.0.jar")
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("failed to read installed mod file: %v", err)
	}
	if string(content) != string(modData) {
		t.Fatalf("file content mismatch: expected %s, got %s", string(modData), string(content))
	}
}

func TestWailsAdapter_InstallMod_CurseForge_Success(t *testing.T) {
	tempDir := t.TempDir()
	modData := []byte("PK\x03\x04test-curseforge-mod-bytes")
	hSha1 := sha1.Sum(modData)
	sha1Hex := hex.EncodeToString(hSha1[:])

	var downloadRequested bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/mods/99999/files") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id":          55555,
						"modId":       99999,
						"displayName": "CF Mod 1.0.0",
						"fileName":    "cf-mod-1.0.0.jar",
						"fileLength":  len(modData),
						"downloadUrl": fmt.Sprintf("http://%s/download/cf-mod-1.0.0.jar", r.Host),
						"hashes": []map[string]interface{}{
							{
								"value": sha1Hex,
								"algo":  1,
							},
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/cf-mod-1.0.0.jar" {
			downloadRequested = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(modData)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	cf := curseforge.NewClient(ts.URL, "dummy-cf-key", ts.Client())
	adapter.SetContent(nil, cf)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "99999",
		Source:     "curseforge",
	})
	if err != nil {
		t.Fatalf("InstallMod returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %+v", res)
	}
	if res.FileName != "cf-mod-1.0.0.jar" {
		t.Fatalf("expected filename 'cf-mod-1.0.0.jar', got %q", res.FileName)
	}
	if !downloadRequested {
		t.Fatalf("expected download request to be served")
	}

	installedPath := filepath.Join(tempDir, "inst-test", "mods", "cf-mod-1.0.0.jar")
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("failed to read installed mod file: %v", err)
	}
	if string(content) != string(modData) {
		t.Fatalf("file content mismatch: expected %s, got %s", string(modData), string(content))
	}
}

func TestWailsAdapter_InstallMod_ChecksumMismatch_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	modData := []byte("PK\x03\x04test-tampered-data")
	wrongSha1 := "0000000000000000000000000000000000000000"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/tampered/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-1",
					"project_id":     "tampered",
					"version_number": "1.0.0",
					"name":           "Tampered 1.0.0",
					"files": []map[string]interface{}{
						{
							"hashes": map[string]string{
								"sha1": wrongSha1,
							},
							"url":      fmt.Sprintf("http://%s/download/tampered.jar", r.Host),
							"filename": "tampered.jar",
							"primary":  true,
							"size":     len(modData),
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/tampered.jar" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(modData)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "tampered",
		Source:     "modrinth",
	})
	if err == nil {
		t.Fatalf("expected checksum error, got success: %+v", res)
	}
	if !strings.Contains(err.Error(), "sha1 mismatch") {
		t.Fatalf("expected 'sha1 mismatch' in error, got: %v", err)
	}

	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	entries, _ := os.ReadDir(modsDir)
	if len(entries) != 0 {
		t.Fatalf("expected mods directory to be empty after failed checksum, found: %d entries", len(entries))
	}
}

func TestWailsAdapter_InstallMod_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)
	existingPath := filepath.Join(modsDir, "existing-mod.jar")
	_ = os.WriteFile(existingPath, []byte("existing"), 0644)

	var downloadCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/existing-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-1",
					"project_id":     "existing-mod",
					"version_number": "1.0.0",
					"files": []map[string]interface{}{
						{
							"url":      fmt.Sprintf("http://%s/download/existing-mod.jar", r.Host),
							"filename": "existing-mod.jar",
							"primary":  true,
							"size":     8,
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/existing-mod.jar" {
			downloadCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("existing"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "existing-mod",
		Source:     "modrinth",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.Message != "Mod already installed" {
		t.Fatalf("expected idempotent success with 'Mod already installed', got: %+v", res)
	}
	if downloadCount != 0 {
		t.Fatalf("expected 0 download requests for existing mod, got: %d", downloadCount)
	}
}

func TestWailsAdapter_InstallMod_Validation(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)

	_, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "",
		ModID:      "test-mod",
	})
	if err == nil || !strings.Contains(err.Error(), "instance_id is required") {
		t.Fatalf("expected instance_id is required, got: %v", err)
	}

	_, err = adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-1",
		ModID:      "",
	})
	if err == nil || !strings.Contains(err.Error(), "mod_id is required") {
		t.Fatalf("expected mod_id is required, got: %v", err)
	}

	_, err = adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-1",
		ModID:      "test",
		Source:     "unsupported_source",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported mod source") {
		t.Fatalf("expected unsupported mod source, got: %v", err)
	}
}

func TestWailsAdapter_HasBuiltinCurseForgeKey(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)

	origEnv := os.Getenv("CURSEFORGE_API_KEY")
	origBuiltin := curseforge.BuiltinAPIKey
	defer func() {
		_ = os.Setenv("CURSEFORGE_API_KEY", origEnv) // errcheck:ok restore env
		curseforge.BuiltinAPIKey = origBuiltin
	}()

	_ = os.Unsetenv("CURSEFORGE_API_KEY") // errcheck:ok unset env
	curseforge.BuiltinAPIKey = ""

	hasKey, err := adapter.HasBuiltinCurseForgeKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasKey {
		t.Errorf("expected hasKey=false when neither builtin nor env is set")
	}

	curseforge.BuiltinAPIKey = "0123456789abcdef0123456789abcdef"
	hasKey, err = adapter.HasBuiltinCurseForgeKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasKey {
		t.Errorf("expected hasKey=true when builtin is set")
	}
}

func TestWailsAdapter_InstallMod_NonAllowlistedHost(t *testing.T) {
	tempDir := t.TempDir()
	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":             "ver-evil",
				"project_id":     "evil-mod",
				"version_number": "1.0.0",
				"name":           "Evil Mod 1.0.0",
				"files": []map[string]interface{}{
					{
						"url":      "http://evil.com/download/evil-mod.jar",
						"filename": "evil-mod.jar",
						"primary":  true,
					},
				},
			},
		})
	}))
	defer ts.Close()

	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)

	_, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "evil-mod",
		Source:     "modrinth",
	})
	if err == nil {
		t.Fatalf("expected error for non-allowlisted download host, got nil")
	}
	if !strings.Contains(err.Error(), "download host not allowed: evil.com") {
		t.Fatalf("expected 'download host not allowed: evil.com', got: %v", err)
	}
}

func TestWailsAdapter_InstallMod_RedirectToUnauthorizedHost(t *testing.T) {
	tempDir := t.TempDir()
	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/redirect-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-redirect",
					"project_id":     "redirect-mod",
					"version_number": "1.0.0",
					"name":           "Redirect Mod 1.0.0",
					"files": []map[string]interface{}{
						{
							"url":      fmt.Sprintf("http://%s/download/redirect-mod.jar", r.Host),
							"filename": "redirect-mod.jar",
							"primary":  true,
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/redirect-mod.jar" {
			// Redirect to non-allowlisted host
			http.Redirect(w, r, "http://unauthorized-evil.com/stolen.jar", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	adapter.SetHTTPClient(ts.Client())

	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)

	_, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID: "inst-test",
		ModID:      "redirect-mod",
		Source:     "modrinth",
	})
	if err == nil {
		t.Fatalf("expected error for redirect to unauthorized host, got nil")
	}
	if !strings.Contains(err.Error(), "redirect to non-allowlisted host rejected") {
		t.Fatalf("expected 'redirect to non-allowlisted host rejected', got: %v", err)
	}

	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	entries, _ := os.ReadDir(modsDir)
	if len(entries) != 0 {
		t.Fatalf("expected mods directory to be clean after redirect rejection, found %d entries", len(entries))
	}
}

func TestWailsAdapter_UpdateMod_AtomicReplacement(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)

	oldPath := filepath.Join(modsDir, "test-mod-1.0.0.jar")
	_ = os.WriteFile(oldPath, []byte("v1-bytes"), 0644)

	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "test-mod",
		ModName:     "Test Mod",
		FileName:    "test-mod-1.0.0.jar",
		Source:      "modrinth",
		VersionID:   "ver-1",
		InstalledAt: time.Now().Add(-1 * time.Hour),
	})
	_ = m.Save(modsDir)

	v2Data := []byte("PK\x03\x04test-mod-v2-bytes")
	hSha1 := sha1.Sum(v2Data)
	sha1Hex := hex.EncodeToString(hSha1[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/test-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-2",
					"project_id":     "test-mod",
					"version_number": "2.0.0",
					"name":           "Test Mod 2.0.0",
					"files": []map[string]interface{}{
						{
							"hashes": map[string]string{
								"sha1": sha1Hex,
							},
							"url":      fmt.Sprintf("http://%s/download/test-mod-2.0.0.jar", r.Host),
							"filename": "test-mod-2.0.0.jar",
							"primary":  true,
							"size":     len(v2Data),
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/test-mod-2.0.0.jar" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(v2Data)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.UpdateMod(wails.UpdateModRequest{
		InstanceID:      "inst-test",
		ModID:           "test-mod",
		OldFileName:     "test-mod-1.0.0.jar",
		Source:          "modrinth",
		TargetVersionID: "ver-2",
	})
	if err != nil {
		t.Fatalf("UpdateMod failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %+v", res)
	}
	if res.FileName != "test-mod-2.0.0.jar" {
		t.Errorf("expected filename 'test-mod-2.0.0.jar', got %q", res.FileName)
	}

	// Verify old jar is deleted
	if _, err := os.Stat(oldPath); err == nil {
		t.Errorf("expected old jar to be deleted, but it still exists")
	}

	// Verify new jar exists
	newPath := filepath.Join(modsDir, "test-mod-2.0.0.jar")
	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read updated mod file: %v", err)
	}
	if string(content) != string(v2Data) {
		t.Errorf("expected updated content %q, got %q", string(v2Data), string(content))
	}

	// Verify exactly 1 jar exists in directory
	entries, _ := os.ReadDir(modsDir)
	var jarCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jar") {
			jarCount++
		}
	}
	if jarCount != 1 {
		t.Errorf("expected exactly 1 .jar file on disk, found %d", jarCount)
	}

	// Verify manifest
	loadedM, _ := manifest.LoadManifest(modsDir)
	if loadedM.GetRecord("test-mod-1.0.0.jar") != nil {
		t.Errorf("expected old mod record removed from manifest")
	}
	rec2 := loadedM.GetRecord("test-mod-2.0.0.jar")
	if rec2 == nil || rec2.VersionID != "ver-2" {
		t.Errorf("expected new mod record in manifest with version ver-2, got %+v", rec2)
	}
}

func TestWailsAdapter_UpdateMod_RollbackOnCorruptedDownload(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)

	oldPath := filepath.Join(modsDir, "test-mod-1.0.0.jar")
	_ = os.WriteFile(oldPath, []byte("v1-original-bytes"), 0644)

	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "test-mod",
		ModName:     "Test Mod",
		FileName:    "test-mod-1.0.0.jar",
		Source:      "modrinth",
		VersionID:   "ver-1",
		InstalledAt: time.Now().Add(-1 * time.Hour),
	})
	_ = m.Save(modsDir)

	corruptData := []byte("corrupt-bytes")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/test-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-2",
					"project_id":     "test-mod",
					"version_number": "2.0.0",
					"files": []map[string]interface{}{
						{
							"hashes": map[string]string{
								"sha1": "0000000000000000000000000000000000000000",
							},
							"url":      fmt.Sprintf("http://%s/download/test-mod-2.0.0.jar", r.Host),
							"filename": "test-mod-2.0.0.jar",
							"primary":  true,
							"size":     len(corruptData),
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/test-mod-2.0.0.jar" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(corruptData)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	_, err := adapter.UpdateMod(wails.UpdateModRequest{
		InstanceID:      "inst-test",
		ModID:           "test-mod",
		OldFileName:     "test-mod-1.0.0.jar",
		Source:          "modrinth",
		TargetVersionID: "ver-2",
	})
	if err == nil {
		t.Fatalf("expected error on corrupt download, got nil")
	}
	if !strings.Contains(err.Error(), "sha1 mismatch") {
		t.Errorf("expected sha1 mismatch error, got: %v", err)
	}

	// Verify old file is intact
	content, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("expected old mod file to still exist: %v", err)
	}
	if string(content) != "v1-original-bytes" {
		t.Errorf("expected old mod file content untouched, got %q", string(content))
	}

	// Verify no new or temp file remains
	newPath := filepath.Join(modsDir, "test-mod-2.0.0.jar")
	if _, err := os.Stat(newPath); err == nil {
		t.Errorf("corrupt new file should not exist on disk")
	}

	// Verify manifest still has old record
	loadedM, _ := manifest.LoadManifest(modsDir)
	if loadedM.GetRecord("test-mod-1.0.0.jar") == nil {
		t.Errorf("expected old mod record to remain in manifest")
	}
}

func TestWailsAdapter_UpdateMod_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)

	destPath := filepath.Join(modsDir, "test-mod-2.0.0.jar")
	_ = os.WriteFile(destPath, []byte("v2-bytes"), 0644)

	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "test-mod",
		ModName:     "Test Mod",
		FileName:    "test-mod-2.0.0.jar",
		Source:      "modrinth",
		VersionID:   "ver-2",
		InstalledAt: time.Now(),
	})
	_ = m.Save(modsDir)

	var downloadCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/test-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-2",
					"project_id":     "test-mod",
					"version_number": "2.0.0",
					"files": []map[string]interface{}{
						{
							"url":      fmt.Sprintf("http://%s/download/test-mod-2.0.0.jar", r.Host),
							"filename": "test-mod-2.0.0.jar",
							"primary":  true,
							"size":     8,
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/test-mod-2.0.0.jar" {
			downloadCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("v2-bytes"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	res, err := adapter.UpdateMod(wails.UpdateModRequest{
		InstanceID:      "inst-test",
		ModID:           "test-mod",
		OldFileName:     "test-mod-2.0.0.jar",
		Source:          "modrinth",
		TargetVersionID: "ver-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.Message != "Mod already up to date" {
		t.Fatalf("expected idempotent success, got: %+v", res)
	}
	if downloadCount != 0 {
		t.Fatalf("expected 0 download requests on idempotent update, got %d", downloadCount)
	}
}

func TestWailsAdapter_ReconcileWithDisk_DuplicateSelfHeal(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)

	oldPath := filepath.Join(modsDir, "modmenu-11.0.4.jar")
	newPath := filepath.Join(modsDir, "modmenu-11.0.5.jar")
	_ = os.WriteFile(oldPath, []byte("old-modmenu"), 0644)
	_ = os.WriteFile(newPath, []byte("new-modmenu"), 0644)

	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "m915XbhN",
		ModSlug:     "modmenu",
		ModName:     "Mod Menu",
		FileName:    "modmenu-11.0.4.jar",
		Source:      "modrinth",
		VersionID:   "11.0.4",
		InstalledAt: time.Now().Add(-1 * time.Hour),
	})
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "m915XbhN",
		ModSlug:     "modmenu",
		ModName:     "Mod Menu",
		FileName:    "modmenu-11.0.5.jar",
		Source:      "modrinth",
		VersionID:   "11.0.5",
		InstalledAt: time.Now(),
	})
	_ = m.Save(modsDir)

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)

	mods, err := adapter.ListInstalledMods("inst-test")
	if err != nil {
		t.Fatalf("ListInstalledMods failed: %v", err)
	}

	// Should have self-healed by deleting older duplicate: 1 enabled, 0 disabled
	var enabledCount, disabledCount int
	for _, mod := range mods {
		if mod.Enabled {
			enabledCount++
			if mod.FileName != "modmenu-11.0.5.jar" {
				t.Errorf("expected modmenu-11.0.5.jar to be enabled, got %s", mod.FileName)
			}
		} else {
			disabledCount++
		}
	}
	if enabledCount != 1 || disabledCount != 0 {
		t.Errorf("expected 1 enabled and 0 disabled mod, got enabled=%d, disabled=%d", enabledCount, disabledCount)
	}

	// Verify file system state: older duplicate is deleted, not renamed to .disabled
	if _, err := os.Stat(oldPath); err == nil || !os.IsNotExist(err) {
		t.Errorf("expected active old jar to be deleted from disk")
	}
	if _, err := os.Stat(filepath.Join(modsDir, "modmenu-11.0.4.jar.disabled")); err == nil || !os.IsNotExist(err) {
		t.Errorf("expected old jar to not exist as .disabled on disk")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected modmenu-11.0.5.jar to exist on disk: %v", err)
	}
}

func TestWailsAdapter_CheckModUpdates_RegressionH2(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	_ = os.MkdirAll(modsDir, 0755)

	v2Data := []byte("PK\x03\x04test-mod-v2-bytes")
	hSha1 := sha1.Sum(v2Data)
	sha1Hex := hex.EncodeToString(hSha1[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/test-mod/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":             "ver-2",
					"project_id":     "test-mod",
					"version_number": "2.0.0",
					"name":           "Test Mod 2.0.0",
					"files": []map[string]interface{}{
						{
							"hashes": map[string]string{
								"sha1": sha1Hex,
							},
							"url":      fmt.Sprintf("http://%s/download/test-mod-2.0.0.jar", r.Host),
							"filename": "test-mod-2.0.0.jar",
							"primary":  true,
							"size":     len(v2Data),
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/download/test-mod-2.0.0.jar" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(v2Data)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	fileSys := fs.NewOSFileSystem()
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(fileSys, tempDir)
	u, _ := url.Parse(ts.URL)
	adapter.SetAllowedHosts([]string{u.Hostname()})
	mr := modrinth.NewClient(ts.URL, ts.Client())
	adapter.SetContent(mr, nil)
	adapter.SetHTTPClient(ts.Client())

	// Step 1: Pre-install v1
	oldPath := filepath.Join(modsDir, "test-mod-1.0.0.jar")
	_ = os.WriteFile(oldPath, []byte("v1-bytes"), 0644)
	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "test-mod",
		ModName:     "Test Mod",
		FileName:    "test-mod-1.0.0.jar",
		Source:      "modrinth",
		VersionID:   "ver-1",
		InstalledAt: time.Now().Add(-1 * time.Hour),
	})
	_ = m.Save(modsDir)

	// Step 2: Update to v2 via UpdateMod
	upRes, err := adapter.UpdateMod(wails.UpdateModRequest{
		InstanceID:      "inst-test",
		ModID:           "test-mod",
		OldFileName:     "test-mod-1.0.0.jar",
		Source:          "modrinth",
		TargetVersionID: "ver-2",
	})
	if err != nil || !upRes.Success {
		t.Fatalf("UpdateMod failed: %v", err)
	}

	// Step 3: Check for updates immediately after replace
	updates, err := adapter.CheckModUpdates("inst-test")
	if err != nil {
		t.Fatalf("CheckModUpdates failed: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("H2 regression failed: expected 0 updates after replace, got %d updates: %+v", len(updates), updates)
	}
}

func TestWailsAdapter_UpdateInstance(t *testing.T) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	procMgr := &mockProcMgr{}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, procMgr, kr, clk)
	adapter := wails.NewWailsAdapter(svc)

	// 1. Create with initial JavaPath
	createReq := wails.CreateInstanceRequest{
		Name:        "Vanilla-1.21",
		GameVersion: "1.21.1",
		Loader:      "vanilla",
		JavaPath:    "/initial/java/bin/java",
	}
	dto, err := adapter.CreateInstance(createReq)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if dto.JavaPath != "/initial/java/bin/java" {
		t.Errorf("expected JavaPath %q, got %q", "/initial/java/bin/java", dto.JavaPath)
	}

	// 2. Update Name, JavaPath, RAM, JVMArgs, and SkipJavaCheck
	skipTrue := true
	updateReq := wails.UpdateInstanceRequest{
		ID:            dto.ID,
		Name:          "Vanilla-1.21-CustomJava",
		JavaPath:      "/opt/temurin-21/bin/java",
		MinRAMMB:      1024,
		MaxRAMMB:      6144,
		JVMArgs:       []string{"-XX:+UseG1GC", "-Duser.language=ru"},
		SkipJavaCheck: &skipTrue,
	}
	updatedDTO, err := adapter.UpdateInstance(updateReq)
	if err != nil {
		t.Fatalf("UpdateInstance failed: %v", err)
	}
	if updatedDTO.Name != "Vanilla-1.21-CustomJava" {
		t.Errorf("expected updated name, got %q", updatedDTO.Name)
	}
	if updatedDTO.JavaPath != "/opt/temurin-21/bin/java" {
		t.Errorf("expected updated JavaPath, got %q", updatedDTO.JavaPath)
	}
	if updatedDTO.MinRAMMB != 1024 || updatedDTO.MaxRAMMB != 6144 {
		t.Errorf("expected RAM 1024/6144, got %d/%d", updatedDTO.MinRAMMB, updatedDTO.MaxRAMMB)
	}
	if len(updatedDTO.JVMArgs) != 2 || updatedDTO.JVMArgs[0] != "-XX:+UseG1GC" {
		t.Errorf("expected JVMArgs updated, got %+v", updatedDTO.JVMArgs)
	}
	if !updatedDTO.SkipJavaCheck {
		t.Errorf("expected SkipJavaCheck=true, got false")
	}
	if updatedDTO.GameVersion != "1.21.1" || updatedDTO.Loader != "vanilla" {
		t.Errorf("preserved fields altered: version=%q loader=%q", updatedDTO.GameVersion, updatedDTO.Loader)
	}

	// 3. Verify ListInstances reflects update
	list := adapter.ListInstances()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}
	if list[0].JavaPath != "/opt/temurin-21/bin/java" || list[0].MinRAMMB != 1024 || list[0].MaxRAMMB != 6144 {
		t.Errorf("ListInstances mismatch: %+v", list[0])
	}

	// 4. Verify BuildLaunchArguments receives updated RAM and JVMArgs
	instDomain, err := svc.GetInstance(dto.ID)
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}
	launchArgs, err := launch.BuildLaunchArguments(launch.LaunchConfig{
		Instance:    instDomain,
		Account:     &domain.Account{UUID: "uuid-1", Username: "Player"},
		VersionMeta: &launch.VersionJSON{ID: "1.21.1", MainClass: "net.minecraft.client.main.Main"},
		GameDir:     "/game",
		AssetsDir:   "/assets",
		NativesDir:  "/natives",
		ResolutionW: 854,
		ResolutionH: 480,
	})
	if err != nil {
		t.Fatalf("BuildLaunchArguments failed: %v", err)
	}
	joinedArgs := strings.Join(launchArgs, " ")
	if !strings.Contains(joinedArgs, "-Xms1024M") || !strings.Contains(joinedArgs, "-Xmx6144M") {
		t.Errorf("expected -Xms1024M -Xmx6144M in args, got: %s", joinedArgs)
	}
	if !strings.Contains(joinedArgs, "-XX:+UseG1GC") || !strings.Contains(joinedArgs, "-Duser.language=ru") {
		t.Errorf("expected custom JVM args in args, got: %s", joinedArgs)
	}

	// 5. Test ClearJavaPath
	clearedDTO, err := adapter.UpdateInstance(wails.UpdateInstanceRequest{
		ID:            dto.ID,
		ClearJavaPath: true,
	})
	if err != nil {
		t.Fatalf("UpdateInstance with ClearJavaPath failed: %v", err)
	}
	if clearedDTO.JavaPath != "" {
		t.Errorf("expected cleared JavaPath, got %q", clearedDTO.JavaPath)
	}

	// 6. Update with empty ID returns error
	_, err = adapter.UpdateInstance(wails.UpdateInstanceRequest{ID: ""})
	if err == nil {
		t.Error("expected error for empty ID, got nil")
	}
}

func TestWailsAdapter_GetLogTail(t *testing.T) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	mockProc := &mockProcMgr{}
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(nil, fileSys, mockProc, kr, clk)
	adapter := wails.NewWailsAdapter(svc)

	// 1. Idle instance with no supervisor returns empty slice, nil error
	tail, err := adapter.GetLogTail("nonexistent-inst", 50)
	if err != nil {
		t.Fatalf("expected nil error on idle GetLogTail, got: %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("expected empty tail, got: %+v", tail)
	}

	// 2. Adapter with nil service also returns empty slice, nil error
	nilAdapter := wails.NewWailsAdapter(nil)
	nilTail, err := nilAdapter.GetLogTail("any", 50)
	if err != nil || len(nilTail) != 0 {
		t.Fatalf("expected empty slice from nil adapter, got %v, err: %v", nilTail, err)
	}
}

func TestWailsAdapter_JavaManager_Methods(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)

	// 1. Unset java manager behaviour
	runtimes, err := adapter.ListJavaRuntimes()
	if err != nil || len(runtimes) != 0 {
		t.Fatalf("expected empty runtimes and nil err with unset javaMgr, got %v, err: %v", runtimes, err)
	}

	status := adapter.GetJavaDownloadStatus()
	if status.Status != "idle" {
		t.Fatalf("expected idle status, got %s", status.Status)
	}

	if err := adapter.DownloadJavaRuntime(21); err == nil {
		t.Error("expected error downloading with unset javaMgr, got nil")
	}

	if err := adapter.RemoveJavaRuntime("any"); err == nil {
		t.Error("expected error removing with unset javaMgr, got nil")
	}

	if _, err := adapter.AddJavaRuntime("any"); err == nil {
		t.Error("expected error adding with unset javaMgr, got nil")
	}

	// 2. Set JavaManager with test directory
	tempDir := t.TempDir()
	managedDir := filepath.Join(tempDir, "runtimes")
	_ = os.MkdirAll(managedDir, 0755)

	customDir := filepath.Join(tempDir, "custom-jdk")
	binDir := filepath.Join(customDir, "bin")
	_ = os.MkdirAll(binDir, 0755)

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}
	_ = os.WriteFile(filepath.Join(binDir, javaExe), []byte("fake-bin"), 0755)

	releaseContent := `JAVA_VERSION="21.0.2"
IMPLEMENTOR="Eclipse Adoptium"
`
	_ = os.WriteFile(filepath.Join(customDir, "release"), []byte(releaseContent), 0644)

	jm := java.NewJavaManager(managedDir, nil, nil, nil)
	adapter.SetJavaManager(jm)

	// Test AddJavaRuntime
	addedDTO, err := adapter.AddJavaRuntime(customDir)
	if err != nil {
		t.Fatalf("unexpected error in AddJavaRuntime: %v", err)
	}
	if addedDTO.MajorVersion != 21 {
		t.Errorf("expected major 21, got %d", addedDTO.MajorVersion)
	}
	if addedDTO.Kind != "detected" {
		t.Errorf("expected kind 'detected', got %s", addedDTO.Kind)
	}

	// Test ListJavaRuntimes
	list, err := adapter.ListJavaRuntimes()
	if err != nil {
		t.Fatalf("ListJavaRuntimes failed: %v", err)
	}
	if len(list) != 0 {
		// Since customDir was added externally but not in managedDir and no detector was attached,
		// list scans managedDir and detector.
	}

	// Test RemoveJavaRuntime on unmanaged path fails
	err = adapter.RemoveJavaRuntime(addedDTO.Path)
	if err == nil {
		t.Error("expected error removing unmanaged runtime, got nil")
	}
}

func TestWailsAdapter_SearchMods(t *testing.T) {
	// 1. Modrinth mock server
	mrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits": []map[string]any{
				{
					"project_id":  "mr-1",
					"slug":        "mr-mod",
					"title":       "Modrinth Mod",
					"description": "Modrinth description",
					"author":      "author1",
					"downloads":   12345,
					"categories":  []string{"fabric", "utility"},
				},
			},
			"total_hits": 42,
		})
	}))
	defer mrServer.Close()

	// 2. CurseForge mock server
	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":            999,
					"slug":          "cf-mod",
					"name":          "CurseForge Mod",
					"summary":       "CurseForge summary",
					"downloadCount": 54321.0,
					"authors":       []map[string]string{{"name": "cf-author"}},
					"categories":    []map[string]string{{"name": "Optimization"}},
				},
			},
			"pagination": map[string]any{
				"totalCount": 100,
			},
		})
	}))
	defer cfServer.Close()

	adapter := wails.NewWailsAdapter(nil)
	mrClient := modrinth.NewClient(mrServer.URL, mrServer.Client())
	cfClient := curseforge.NewClient(cfServer.URL, "dummy-cf-key", cfServer.Client())
	adapter.SetContent(mrClient, cfClient)

	// Test Modrinth search
	mrRes, err := adapter.SearchMods(wails.SearchModsRequest{
		Query:       "test",
		GameVersion: "1.21.1",
		Loader:      "fabric",
		Source:      "modrinth",
		Limit:       20,
		Offset:      0,
		Sort:        "downloads",
		Category:    "utility",
	})
	if err != nil {
		t.Fatalf("Modrinth SearchMods failed: %v", err)
	}
	if mrRes.TotalCount != 42 {
		t.Errorf("expected TotalCount 42, got %d", mrRes.TotalCount)
	}
	if len(mrRes.Items) != 1 || mrRes.Items[0].Slug != "mr-mod" {
		t.Errorf("unexpected Modrinth items: %+v", mrRes.Items)
	}

	// Test CurseForge search
	cfRes, err := adapter.SearchMods(wails.SearchModsRequest{
		Query:       "test",
		GameVersion: "1.21.1",
		Loader:      "fabric",
		Source:      "curseforge",
		Limit:       20,
		Offset:      0,
		Sort:        "popularity",
		Category:    "optimization",
	})
	if err != nil {
		t.Fatalf("CurseForge SearchMods failed: %v", err)
	}
	if cfRes.TotalCount != 100 {
		t.Errorf("expected TotalCount 100, got %d", cfRes.TotalCount)
	}
	if len(cfRes.Items) != 1 || cfRes.Items[0].Slug != "cf-mod" {
		t.Errorf("unexpected CurseForge items: %+v", cfRes.Items)
	}
}

func TestWailsAdapter_SearchMods_ErrorClassification(t *testing.T) {
	// 1. Test Rate Limit Error classification
	rlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`)) // errcheck:ok test mock
	}))
	defer rlServer.Close()

	cfClientRL := curseforge.NewClient(rlServer.URL, "test-key", rlServer.Client())
	adapterRL := wails.NewWailsAdapter(nil)
	adapterRL.SetContent(nil, cfClientRL)

	resRL, err := adapterRL.SearchMods(wails.SearchModsRequest{
		Query:  "test",
		Source: "curseforge",
	})
	if err != nil {
		t.Fatalf("expected nil error for classified rate_limited, got: %v", err)
	}
	if resRL.Reason != "rate_limited" {
		t.Errorf("expected Reason 'rate_limited', got %q", resRL.Reason)
	}
	if resRL.RetryAfterSeconds != 12 {
		t.Errorf("expected RetryAfterSeconds 12, got %d", resRL.RetryAfterSeconds)
	}

	// 2. Test Invalid Key (401) classification
	keyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid key"}`)) // errcheck:ok test mock
	}))
	defer keyServer.Close()

	cfClientKey := curseforge.NewClient(keyServer.URL, "bad-key", keyServer.Client())
	adapterKey := wails.NewWailsAdapter(nil)
	adapterKey.SetContent(nil, cfClientKey)

	resKey, err := adapterKey.SearchMods(wails.SearchModsRequest{
		Query:  "test",
		Source: "curseforge",
	})
	if err != nil {
		t.Fatalf("expected nil error for classified key_invalid, got: %v", err)
	}
	if resKey.Reason != "key_invalid" {
		t.Errorf("expected Reason 'key_invalid', got %q", resKey.Reason)
	}

	// 3. Test Unreachable Server classification
	cfClientUnreachable := curseforge.NewClient("http://127.0.0.1:59999", "test-key", &http.Client{Timeout: 50 * time.Millisecond})
	adapterUnreachable := wails.NewWailsAdapter(nil)
	adapterUnreachable.SetContent(nil, cfClientUnreachable)

	resUnreachable, err := adapterUnreachable.SearchMods(wails.SearchModsRequest{
		Query:  "test",
		Source: "curseforge",
	})
	if err != nil {
		t.Fatalf("expected nil error for unreachable, got: %v", err)
	}
	if resUnreachable.Reason != "unreachable" {
		t.Errorf("expected Reason 'unreachable', got %q", resUnreachable.Reason)
	}
}

func TestWailsAdapter_ListModVersions(t *testing.T) {
	mrHits := 0
	mrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mrHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":           "mr-ver-1",
				"project_id":   "sodium",
				"name":         "Sodium 1.0.0",
				"version_type": "release",
				"changelog":    "Sodium 1.0.0 release notes",
				"files": []map[string]any{
					{
						"id":       "file-1",
						"url":      "https://cdn.modrinth.com/sodium-1.0.0.jar",
						"filename": "sodium-1.0.0.jar",
						"primary":  true,
						"size":     1048576,
					},
				},
				"game_versions":  []string{"1.21.1"},
				"loaders":        []string{"fabric"},
				"date_published": time.Now().Format(time.RFC3339),
			},
		})
	}))
	defer mrServer.Close()

	cfHits := 0
	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":          555123,
					"modId":       123,
					"fileName":    "jei-1.21.1.jar",
					"downloadUrl": "https://edge.forgecdn.net/jei.jar",
					"fileLength":  2097152,
					"releaseType": 1,
					"fileDate":    time.Now().Format(time.RFC3339),
					"gameVersions": []string{"1.21.1", "Fabric"},
				},
			},
		})
	}))
	defer cfServer.Close()

	adapter := wails.NewWailsAdapter(nil)
	mrClient := modrinth.NewClient(mrServer.URL, mrServer.Client())
	cfClient := curseforge.NewClient(cfServer.URL, "cf-key", cfServer.Client())
	adapter.SetContent(mrClient, cfClient)

	// Test Modrinth ListModVersions + 10-minute cache
	mrReq := wails.ListModVersionsRequest{
		ModID:       "sodium",
		Source:      "modrinth",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	}
	mrFiles1, err := adapter.ListModVersions(mrReq)
	if err != nil {
		t.Fatalf("Modrinth ListModVersions failed: %v", err)
	}
	if len(mrFiles1) != 1 || mrFiles1[0].ReleaseType != "release" {
		t.Fatalf("unexpected Modrinth files: %+v", mrFiles1)
	}
	if mrFiles1[0].Changelog != "Sodium 1.0.0 release notes" {
		t.Fatalf("expected Modrinth changelog populated, got: %q", mrFiles1[0].Changelog)
	}
	if mrHits != 1 {
		t.Fatalf("expected 1 MR hit, got %d", mrHits)
	}

	// Second call should hit 10m TTL cache
	mrFiles2, err := adapter.ListModVersions(mrReq)
	if err != nil {
		t.Fatalf("Modrinth ListModVersions cache hit failed: %v", err)
	}
	if len(mrFiles2) != 1 || mrHits != 1 {
		t.Fatalf("expected cache hit (mrHits=1), got mrHits=%d", mrHits)
	}

	// Test CurseForge ListModVersions + 10-minute cache
	cfReq := wails.ListModVersionsRequest{
		ModID:       "123",
		Source:      "curseforge",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	}
	cfFiles1, err := adapter.ListModVersions(cfReq)
	if err != nil {
		t.Fatalf("CurseForge ListModVersions failed: %v", err)
	}
	if len(cfFiles1) != 1 || cfFiles1[0].ReleaseType != "release" {
		t.Fatalf("unexpected CurseForge files: %+v", cfFiles1)
	}
	if cfFiles1[0].Changelog != "" {
		t.Fatalf("expected CurseForge changelog empty, got: %q", cfFiles1[0].Changelog)
	}
	if cfHits != 1 {
		t.Fatalf("expected 1 CF hit, got %d", cfHits)
	}

	// Second call should hit 10m TTL cache
	cfFiles2, err := adapter.ListModVersions(cfReq)
	if err != nil {
		t.Fatalf("CurseForge ListModVersions cache hit failed: %v", err)
	}
	if len(cfFiles2) != 1 || cfHits != 1 {
		t.Fatalf("expected cache hit (cfHits=1), got cfHits=%d", cfHits)
	}
}

func TestWailsAdapter_InstallMod_DeterministicAndProgress(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "inst-test", "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("create mods dir: %v", err)
	}

	dummyJar := []byte("dummy-jar-content-for-testing")
	h := sha1.New()
	h.Write(dummyJar)
	dummySha1 := hex.EncodeToString(h.Sum(nil))

	mrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jar") {
			w.Header().Set("Content-Type", "application/java-archive")
			_, _ = w.Write(dummyJar)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		// Return two versions: beta and release. Deterministic selector must choose release!
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":           "beta-ver",
				"project_id":   "test-mod",
				"name":         "Test Mod Beta",
				"version_type": "beta",
				"files": []map[string]any{
					{
						"id":       "file-beta",
						"url":      "http://" + r.Host + "/test-beta.jar",
						"filename": "test-beta.jar",
						"primary":  true,
						"size":     len(dummyJar),
						"hashes": map[string]string{
							"sha1": dummySha1,
						},
					},
				},
				"game_versions":  []string{"1.21.1"},
				"loaders":        []string{"fabric"},
				"date_published": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id":           "rel-ver",
				"project_id":   "test-mod",
				"name":         "Test Mod Release",
				"version_type": "release",
				"files": []map[string]any{
					{
						"id":       "file-release",
						"url":      "http://" + r.Host + "/test-release.jar",
						"filename": "test-release.jar",
						"primary":  true,
						"size":     len(dummyJar),
						"hashes": map[string]string{
							"sha1": dummySha1,
						},
					},
				},
				"game_versions":  []string{"1.21.1"},
				"loaders":        []string{"fabric"},
				"date_published": time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			},
		})
	}))
	defer mrServer.Close()

	u, _ := url.Parse(mrServer.URL)
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetAllowedHosts([]string{u.Hostname(), u.Host})
	adapter.SetFileSystem(fs.NewOSFileSystem(), tempDir)

	mrClient := modrinth.NewClient(mrServer.URL, mrServer.Client())
	adapter.SetContent(mrClient, nil)

	// Check initial install status is idle
	initialStatus, err := adapter.GetModInstallStatus("inst-test")
	if err != nil {
		t.Fatalf("GetModInstallStatus failed: %v", err)
	}
	if initialStatus.Status != "idle" {
		t.Errorf("expected initial status 'idle', got '%s'", initialStatus.Status)
	}

	// Install without VersionID: deterministic selector picks release over beta
	resp, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID:  "inst-test",
		ModID:       "test-mod",
		Source:      "modrinth",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	})
	if err != nil {
		t.Fatalf("InstallMod failed: %v", err)
	}
	if resp.FileName != "test-release.jar" {
		t.Errorf("expected deterministic selection of release 'test-release.jar', got '%s'", resp.FileName)
	}

	// Status after install should be completed
	statusAfter, err := adapter.GetModInstallStatus("inst-test")
	if err != nil {
		t.Fatalf("GetModInstallStatus failed: %v", err)
	}
	if statusAfter.Status != "completed" || statusAfter.Percentage != 100 {
		t.Errorf("expected status 'completed' with 100%%, got %+v", statusAfter)
	}

	// Test installing with explicit VersionID (e.g. "beta-ver")
	_ = os.Remove(filepath.Join(modsDir, "test-release.jar"))
	respBeta, err := adapter.InstallMod(wails.InstallModRequest{
		InstanceID:  "inst-test",
		ModID:       "test-mod",
		Source:      "modrinth",
		GameVersion: "1.21.1",
		Loader:      "fabric",
		VersionID:   "beta-ver",
	})
	if err != nil {
		t.Fatalf("InstallMod with explicit VersionID failed: %v", err)
	}
	if respBeta.FileName != "test-beta.jar" {
		t.Errorf("expected explicit version 'test-beta.jar', got '%s'", respBeta.FileName)
	}
}

func TestWailsAdapter_ManifestReconcileAndDualFileDelete(t *testing.T) {
	tempDir := t.TempDir()
	instDir := filepath.Join(tempDir, "instances")
	instanceID := "inst-c4"
	modsDir := filepath.Join(instDir, instanceID, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}

	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(nil, instDir)
	adapter.SetDB(db.DB())

	// 1. Drop two untracked files on disk
	fileA := filepath.Join(modsDir, "mod-alpha.jar")
	fileB := filepath.Join(modsDir, "mod-beta.jar.disabled")
	_ = os.WriteFile(fileA, []byte("content-a"), 0644)
	_ = os.WriteFile(fileB, []byte("content-b"), 0644)

	// ListInstalledMods: should reconcile with disk, create nord-installs.json, and sync SQLite
	list1, err := adapter.ListInstalledMods(instanceID)
	if err != nil {
		t.Fatalf("ListInstalledMods failed: %v", err)
	}
	if len(list1) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(list1))
	}

	// Verify manifest was created
	mPath := filepath.Join(modsDir, "nord-installs.json")
	if _, err := os.Stat(mPath); os.IsNotExist(err) {
		t.Fatalf("expected nord-installs.json to be created")
	}

	// 2. Toggle mod-alpha to disabled
	err = adapter.ToggleMod(wails.ToggleModRequest{
		InstanceID: instanceID,
		FileName:   "mod-alpha.jar",
		Enable:     false,
	})
	if err != nil {
		t.Fatalf("ToggleMod failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "mod-alpha.jar.disabled")); err != nil {
		t.Fatalf("expected mod-alpha.jar.disabled to exist on disk")
	}

	// 3. Test Directive A2: Dual-file deletion
	// Create both candidates on disk
	dualJar := filepath.Join(modsDir, "dual-target.jar")
	dualDis := filepath.Join(modsDir, "dual-target.jar.disabled")
	_ = os.WriteFile(dualJar, []byte("dual-1"), 0644)
	_ = os.WriteFile(dualDis, []byte("dual-2"), 0644)

	// Trigger reconcile so it's tracked
	_, _ = adapter.ListInstalledMods(instanceID)

	// Call DeleteMod specifying only dual-target.jar
	err = adapter.DeleteMod(wails.DeleteModRequest{
		InstanceID: instanceID,
		FileName:   "dual-target.jar",
	})
	if err != nil {
		t.Fatalf("DeleteMod failed: %v", err)
	}

	// A2: BOTH .jar and .jar.disabled must be deleted from disk
	if _, err := os.Stat(dualJar); !os.IsNotExist(err) {
		t.Errorf("expected dual-target.jar to be deleted from disk")
	}
	if _, err := os.Stat(dualDis); !os.IsNotExist(err) {
		t.Errorf("expected dual-target.jar.disabled to also be deleted from disk (Directive A2)")
	}

	// 4. External Delete Reconcile: delete mod-beta from disk manually
	_ = os.Remove(fileB)
	listAfterExtDelete, err := adapter.ListInstalledMods(instanceID)
	if err != nil {
		t.Fatalf("ListInstalledMods after external delete failed: %v", err)
	}
	for _, m := range listAfterExtDelete {
		if m.Name == "mod-beta" {
			t.Errorf("expected mod-beta to be pruned after external disk removal")
		}
	}
}

func TestWailsAdapter_CheckModUpdates(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)

	// Empty instance ID should fail
	_, err := adapter.CheckModUpdates("")
	if err == nil {
		t.Fatalf("expected error on empty instance ID")
	}

	tmpDir := t.TempDir()
	instDir := filepath.Join(tmpDir, "instances")
	instID := "test-updates-inst"
	modsDir := filepath.Join(instDir, instID, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}

	adapter.SetFileSystem(fs.NewOSFileSystem(), instDir)

	// Mock server for Modrinth versions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/project/mod-alpha/version") {
			versions := []map[string]interface{}{
				{
					"id":             "ver-2.0.0",
					"project_id":     "mod-alpha",
					"name":           "Alpha 2.0.0",
					"version_number": "2.0.0",
					"game_versions":  []string{"1.21.1"},
					"loaders":        []string{"fabric"},
					"files": []map[string]interface{}{
						{
							"url":      "http://example.com/alpha-2.0.0.jar",
							"filename": "alpha-2.0.0.jar",
							"primary":  true,
							"size":     1024,
							"hashes":   map[string]string{"sha1": "abcd"},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(versions)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	mrClient := modrinth.NewClient(server.URL, server.Client())
	adapter.SetContent(mrClient, nil)

	// Case 1: Empty mods directory -> 0 updates
	updates, err := adapter.CheckModUpdates(instID)
	if err != nil {
		t.Fatalf("CheckModUpdates failed on empty mods: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}

	// Case 2: Mod installed with older version ver-1.0.0
	jarPath := filepath.Join(modsDir, "alpha-1.0.0.jar")
	if err := os.WriteFile(jarPath, []byte("alpha-content"), 0644); err != nil {
		t.Fatalf("write jar failed: %v", err)
	}
	manifestContent := []byte(`{
		"schema_version": 1,
		"mods": {
			"alpha-1.0.0": {
				"mod_id": "mod-alpha",
				"mod_name": "Alpha Mod",
				"file_name": "alpha-1.0.0.jar",
				"source": "modrinth",
				"version_id": "ver-1.0.0",
				"installed_at": "2026-09-20T12:00:00Z"
			}
		}
	}`)
	if err := os.WriteFile(filepath.Join(modsDir, "nord-installs.json"), manifestContent, 0644); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}

	updates, err = adapter.CheckModUpdates(instID)
	if err != nil {
		t.Fatalf("CheckModUpdates failed: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].ModID != "mod-alpha" || updates[0].FileName != "alpha-1.0.0.jar" || updates[0].LatestVersionID != "alpha-2.0.0.jar" {
		t.Fatalf("unexpected update DTO: %+v", updates[0])
	}
}

func TestWailsAdapter_GetDiagnosticReport(t *testing.T) {
	adapter := wails.NewWailsAdapter(nil)
	adapter.SetVersion("0.6.1")

	tmpDir := t.TempDir()
	instDir := filepath.Join(tmpDir, "instances")
	instID := "test-diag-inst"
	modsDir := filepath.Join(instDir, instID, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	adapter.SetFileSystem(fs.NewOSFileSystem(), instDir)

	dbPath := filepath.Join(tmpDir, "test_settings.db")
	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	settingsRepo := storage.NewSettingsRepository(db)
	_ = settingsRepo.Set(context.Background(), "curseforge_api_key", "secret-cf-api-key-12345")
	_ = settingsRepo.Set(context.Background(), "theme", "nord-dark")
	adapter.SetSettings(settingsRepo)

	report, err := adapter.GetDiagnosticReport(instID)
	if err != nil {
		t.Fatalf("GetDiagnosticReport failed: %v", err)
	}

	if !strings.Contains(report, "=== Nord Launcher Diagnostic Report ===") {
		t.Errorf("expected diagnostic header in report")
	}
	if !strings.Contains(report, "Launcher Version: 0.6.1") {
		t.Errorf("expected launcher version in report")
	}
	if !strings.Contains(report, "OS:") || !strings.Contains(report, "Architecture:") {
		t.Errorf("expected OS and architecture in report")
	}
	if !strings.Contains(report, "theme: nord-dark") {
		t.Errorf("expected theme in report")
	}
	if strings.Contains(report, "secret-cf-api-key-12345") {
		t.Errorf("SECURITY LEAK: raw CurseForge API key was present in diagnostic report")
	}
	if !strings.Contains(report, "curseforge_api_key: [SET]") {
		t.Errorf("expected masked curseforge_api_key: [SET] in report")
	}
}

func createMockMrPackBytes(t *testing.T, name, mcVer, loader, loaderVer string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	indexJSON := fmt.Sprintf(`{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": %q,
		"summary": "Test pack summary",
		"files": [
			{
				"path": "mods/test-mod.jar",
				"hashes": {"sha1": "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed"},
				"env": {"client": "required", "server": "required"},
				"downloads": ["https://example.com/test-mod.jar"],
				"fileSize": 1024
			}
		],
		"dependencies": {
			"minecraft": %q,
			%q: %q
		}
	}`, name, mcVer, loader, loaderVer)

	f, err := zw.Create("modrinth.index.json")
	if err != nil {
		t.Fatalf("create modrinth.index.json in zip: %v", err)
	}
	if _, err := f.Write([]byte(indexJSON)); err != nil {
		t.Fatalf("write index json: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestWailsAdapter_MrPackMethods(t *testing.T) {
	tempDir := t.TempDir()
	adapter := wails.NewWailsAdapter(nil)

	// 1. PickMrPackFile with custom picker hook
	expectedPath := filepath.Join(tempDir, "test.mrpack")
	adapter.SetFilePicker(func() (string, error) {
		return expectedPath, nil
	})
	picked, err := adapter.PickMrPackFile()
	if err != nil || picked != expectedPath {
		t.Fatalf("PickMrPackFile returned %q, err=%v (expected %q)", picked, err, expectedPath)
	}

	// 2. GetMrPackImportPlan validation
	_, err = adapter.GetMrPackImportPlan("")
	if err == nil {
		t.Fatal("expected error for empty mrpack path, got nil")
	}

	mrpackData := createMockMrPackBytes(t, "SkyFactory 5", "1.20.1", "fabric", "0.15.11")
	mrpackFile := filepath.Join(tempDir, "sample.mrpack")
	if err := os.WriteFile(mrpackFile, mrpackData, 0644); err != nil {
		t.Fatalf("failed to write mrpack file: %v", err)
	}

	plan, err := adapter.GetMrPackImportPlan(mrpackFile)
	if err != nil {
		t.Fatalf("GetMrPackImportPlan failed: %v", err)
	}
	if plan.Name != "SkyFactory 5" {
		t.Errorf("expected pack name 'SkyFactory 5', got %q", plan.Name)
	}
	if plan.GameVersion != "1.20.1" {
		t.Errorf("expected mc 1.20.1, got %q", plan.GameVersion)
	}
	if plan.Loader != "fabric" || plan.LoaderVer != "0.15.11" {
		t.Errorf("expected fabric 0.15.11, got %q %q", plan.Loader, plan.LoaderVer)
	}
	if plan.TotalFiles != 1 || plan.TotalSize != 1024 {
		t.Errorf("expected 1 file of size 1024, got %d files, %d bytes", plan.TotalFiles, plan.TotalSize)
	}

	// 3. ImportMrPack validation
	_, err = adapter.ImportMrPack(wails.ImportMrPackRequest{MrPackPath: ""})
	if err == nil {
		t.Fatal("expected error importing empty mrpack path, got nil")
	}

	// 4. GetMrPackImportStatus idle default
	status, err := adapter.GetMrPackImportStatus("nonexistent-inst")
	if err != nil || status.Status != "idle" {
		t.Fatalf("expected idle status for unknown instance, got %+v, err=%v", status, err)
	}

	// 5. ExportMrPack validation
	_, err = adapter.ExportMrPack(wails.ExportMrPackRequest{InstanceID: ""})
	if err == nil {
		t.Fatal("expected error exporting empty instance id, got nil")
	}
}

func TestWailsAdapter_JavaRuntimeUpdates(t *testing.T) {
	tempDir := t.TempDir()
	adapter := wails.NewWailsAdapter(nil)

	// When JavaManager is nil
	updates, err := adapter.CheckJavaRuntimeUpdates()
	if err != nil || len(updates) != 0 {
		t.Fatalf("expected empty updates when javaMgr is nil, got %+v, err=%v", updates, err)
	}

	_, err = adapter.UpgradeJavaRuntime(21)
	if err == nil {
		t.Fatal("expected error upgrading java when javaMgr is nil, got nil")
	}

	// With configured JavaManager
	managedDir := filepath.Join(tempDir, "runtimes")
	_ = os.MkdirAll(managedDir, 0755)
	jm := java.NewJavaManager(managedDir, nil, nil, nil)
	adapter.SetJavaManager(jm)

	status, err := adapter.UpgradeJavaRuntime(21)
	if err != nil {
		t.Fatalf("UpgradeJavaRuntime returned error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil download status")
	}
}

func TestWailsAdapter_OpenPath(t *testing.T) {
	tempDir := t.TempDir()
	adapter := wails.NewWailsAdapter(nil)

	var recordedPath string
	var recordedIsDir bool
	cleanup := wails.SetOpenPathExecForTesting(func(cleanPath string, isDir bool) error {
		recordedPath = cleanPath
		recordedIsDir = isDir
		return nil
	})
	defer cleanup()

	// 1. Existing directory
	testSubDir := filepath.Join(tempDir, "instances", "inst-1")
	if err := os.MkdirAll(testSubDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := adapter.OpenPath(testSubDir); err != nil {
		t.Fatalf("OpenPath directory failed: %v", err)
	}
	if recordedPath != filepath.Clean(testSubDir) || !recordedIsDir {
		t.Errorf("expected clean dir path %s, isDir=true; got %s, %v", testSubDir, recordedPath, recordedIsDir)
	}

	// 2. Existing file
	testFile := filepath.Join(testSubDir, "instance.json")
	if err := os.WriteFile(testFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := adapter.OpenPath(testFile); err != nil {
		t.Fatalf("OpenPath file failed: %v", err)
	}
	if recordedPath != filepath.Clean(testFile) || recordedIsDir {
		t.Errorf("expected clean file path %s, isDir=false; got %s, %v", testFile, recordedPath, recordedIsDir)
	}

	// 3. Empty path error
	if err := adapter.OpenPath(""); err == nil {
		t.Error("expected error on empty path, got nil")
	}
	if err := adapter.OpenPath("   "); err == nil {
		t.Error("expected error on whitespace path, got nil")
	}

	// 4. Non-existent path error
	nonExistent := filepath.Join(tempDir, "does-not-exist")
	if err := adapter.OpenPath(nonExistent); err == nil {
		t.Error("expected error on non-existent path, got nil")
	}
}

func TestWailsAdapter_Screenshots(t *testing.T) {
	tempDir := t.TempDir()
	instancesDir := filepath.Join(tempDir, "instances")
	instID := "test-shot-inst"
	shotsDir := filepath.Join(instancesDir, instID, "screenshots")
	if err := os.MkdirAll(shotsDir, 0755); err != nil {
		t.Fatalf("failed to create screenshots dir: %v", err)
	}

	adapter := wails.NewWailsAdapter(nil)
	adapter.SetFileSystem(nil, instancesDir)

	// 1. Initially empty directory
	list, err := adapter.ListScreenshots(instID)
	if err != nil {
		t.Fatalf("ListScreenshots failed on empty dir: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 screenshots, got %d", len(list))
	}

	// 2. Create sample files: shot1 (older), shot2 (newer), and ignore text file
	shot1 := filepath.Join(shotsDir, "2026-09-01_12.00.00.png")
	shot2 := filepath.Join(shotsDir, "2026-09-02_12.00.00.png")
	txtFile := filepath.Join(shotsDir, "notes.txt")

	_ = os.WriteFile(shot1, []byte("fake-png-1"), 0644)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(shot2, []byte("fake-png-2"), 0644)
	_ = os.WriteFile(txtFile, []byte("ignore me"), 0644)

	list, err = adapter.ListScreenshots(instID)
	if err != nil {
		t.Fatalf("ListScreenshots failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected exactly 2 PNG screenshots, got %d", len(list))
	}
	// Newest first: shot2 should be index 0
	if list[0].FileName != "2026-09-02_12.00.00.png" {
		t.Errorf("expected newest shot first, got %s", list[0].FileName)
	}

	// 3. GetScreenshotData
	dataRes, err := adapter.GetScreenshotData(wails.GetScreenshotDataRequest{
		InstanceID: instID,
		FileName:   "2026-09-02_12.00.00.png",
	})
	if err != nil {
		t.Fatalf("GetScreenshotData failed: %v", err)
	}
	if !strings.HasPrefix(dataRes.DataURL, "data:image/png;base64,") {
		t.Errorf("expected data URL to start with data:image/png;base64,, got %s", dataRes.DataURL)
	}

	// Security: traversal check
	_, err = adapter.GetScreenshotData(wails.GetScreenshotDataRequest{
		InstanceID: instID,
		FileName:   "../notes.txt",
	})
	if err == nil {
		t.Errorf("expected error on path traversal in GetScreenshotData, got nil")
	}

	// 4. DeleteScreenshot
	err = adapter.DeleteScreenshot(wails.DeleteScreenshotRequest{
		InstanceID: instID,
		FileName:   "2026-09-01_12.00.00.png",
	})
	if err != nil {
		t.Fatalf("DeleteScreenshot failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(shot1); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted from disk")
	}

	// Verify delete is idempotent
	err = adapter.DeleteScreenshot(wails.DeleteScreenshotRequest{
		InstanceID: instID,
		FileName:   "2026-09-01_12.00.00.png",
	})
	if err != nil {
		t.Errorf("expected idempotent DeleteScreenshot to return nil, got %v", err)
	}
}


