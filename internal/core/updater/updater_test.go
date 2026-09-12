package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		current  string
		remote   string
		expected bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "0.1.1", true},
		{"1.0.0", "2.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"0.2.0", "0.1.0", false},
		{"1.2.0", "1.1.9", false},
		{"v0.1.0", "v0.2.0", true},
		{"v1.0.0-beta", "v1.0.0", false},
		{"v1.0.0", "v1.0.1-rc1", true},
	}

	for _, tc := range tests {
		got := isVersionNewer(tc.current, tc.remote)
		if got != tc.expected {
			t.Errorf("isVersionNewer(%q, %q) = %v; expected %v", tc.current, tc.remote, got, tc.expected)
		}
	}
}

func TestAutoUpdater_CheckForUpdates(t *testing.T) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	platKey := CurrentPlatformKey()

	manifest := UpdateManifest{
		Version:     "0.2.0",
		ReleaseDate: time.Now(),
		Changelog:   "- Added security audit\n- Faster downloads",
		Platforms: map[string]PlatformAsset{
			platKey: {
				URL:       "https://example.com/download",
				SHA256:    "abcdef123456",
				Signature: "dummySig",
				Size:      1024,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	// 1. Current version is older -> update available
	updater := NewAutoUpdater("0.1.0", server.URL, pubKey, server.Client())
	info, err := updater.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates failed: %v", err)
	}
	if !info.Available {
		t.Errorf("expected update available, got false")
	}
	if info.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", info.Version)
	}

	// 2. Current version is same or newer -> no update
	updaterSame := NewAutoUpdater("0.2.0", server.URL, pubKey, server.Client())
	infoSame, err := updaterSame.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates same failed: %v", err)
	}
	if infoSame.Available {
		t.Errorf("expected update NOT available, got true")
	}

	// 3. Platform missing in manifest
	manifestUnsupported := UpdateManifest{
		Version:   "0.3.0",
		Platforms: map[string]PlatformAsset{"otheros-arch": {URL: "..."}},
	}
	serverUnsupported := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifestUnsupported)
	}))
	defer serverUnsupported.Close()

	updaterUnsup := NewAutoUpdater("0.1.0", serverUnsupported.URL, pubKey, serverUnsupported.Client())
	_, err = updaterUnsup.CheckForUpdates(context.Background())
	if err == nil {
		t.Fatalf("expected ErrUnsupportedPlatform, got nil error")
	}
}

func TestAutoUpdater_DownloadAndApply(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	payload := []byte("nord-launcher-v0.2.0-binary-content")
	hasher := sha256.New()
	hasher.Write(payload)
	payloadSHA256 := hex.EncodeToString(hasher.Sum(nil))
	signature := ed25519.Sign(privKey, payload)
	sigB64 := base64.StdEncoding.EncodeToString(signature)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dummyExe := filepath.Join(tmpDir, "nord-launcher.exe")
	originalContent := []byte("nord-launcher-v0.1.0-original")
	if err := os.WriteFile(dummyExe, originalContent, 0755); err != nil {
		t.Fatalf("write dummy exe: %v", err)
	}

	updater := NewAutoUpdater("0.1.0", "", pubKey, server.Client())

	asset := PlatformAsset{
		URL:       server.URL,
		SHA256:    payloadSHA256,
		Signature: sigB64,
		Size:      int64(len(payload)),
	}

	// 1. Success case
	err = updater.DownloadAndApply(context.Background(), asset, dummyExe)
	if err != nil {
		t.Fatalf("DownloadAndApply failed: %v", err)
	}

	// Verify new executable content
	updatedContent, err := os.ReadFile(dummyExe)
	if err != nil {
		t.Fatalf("read updated exe: %v", err)
	}
	if string(updatedContent) != string(payload) {
		t.Errorf("expected %q, got %q", string(payload), string(updatedContent))
	}

	// Verify backup (.old) exists with original content
	backupContent, err := os.ReadFile(dummyExe + ".old")
	if err != nil {
		t.Fatalf("read backup exe: %v", err)
	}
	if string(backupContent) != string(originalContent) {
		t.Errorf("expected backup %q, got %q", string(originalContent), string(backupContent))
	}

	// 2. Failure: SHA256 mismatch
	corruptAsset := asset
	corruptAsset.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	err = updater.DownloadAndApply(context.Background(), corruptAsset, dummyExe)
	if err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}

	// 3. Failure: Signature mismatch
	badSigAsset := asset
	badSigAsset.Signature = base64.StdEncoding.EncodeToString([]byte("invalid-signature-data-32-bytes!"))
	err = updater.DownloadAndApply(context.Background(), badSigAsset, dummyExe)
	if err == nil {
		t.Fatalf("expected signature invalid error, got nil")
	}

	// 4. Failure: Bad base64 signature
	corruptSigAsset := asset
	corruptSigAsset.Signature = "!!!not-valid-base64!!!"
	err = updater.DownloadAndApply(context.Background(), corruptSigAsset, dummyExe)
	if err == nil {
		t.Fatalf("expected error on bad base64 signature, got nil")
	}

	// 5. Failure: HTTP 500 error on download
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer errServer.Close()

	errAsset := asset
	errAsset.URL = errServer.URL
	err = updater.DownloadAndApply(context.Background(), errAsset, dummyExe)
	if err == nil {
		t.Fatalf("expected error on HTTP 500 download, got nil")
	}
}

func TestAutoUpdater_EdgeCases(t *testing.T) {
	// Test nil http client defaults
	u := NewAutoUpdater("0.1.0", "http://example.com", nil, nil)
	if u.httpClient == nil {
		t.Fatalf("expected default httpClient, got nil")
	}

	// Test manifest server error 500
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad manifest", http.StatusInternalServerError)
	}))
	defer errServer.Close()

	uErr := NewAutoUpdater("0.1.0", errServer.URL, nil, errServer.Client())
	_, err := uErr.CheckForUpdates(context.Background())
	if err == nil {
		t.Fatalf("expected error on 500 manifest, got nil")
	}

	// Test ApplyUpdate error cases
	if err := uErr.ApplyUpdate(context.Background(), nil); err != ErrNoUpdateAvailable {
		t.Errorf("expected ErrNoUpdateAvailable for nil info, got %v", err)
	}
	if err := uErr.ApplyUpdate(context.Background(), &UpdateInfo{Available: false}); err != ErrNoUpdateAvailable {
		t.Errorf("expected ErrNoUpdateAvailable for unavailable update, got %v", err)
	}

	// Test stale backup cleanup
	if exe, err := os.Executable(); err == nil {
		oldPath := exe + ".old"
		_ = os.WriteFile(oldPath, []byte("stale backup"), 0644)
		CleanupStaleBackup()
		if _, statErr := os.Stat(oldPath); statErr == nil {
			t.Errorf("expected stale backup %s to be removed", oldPath)
			_ = os.Remove(oldPath)
		}
	} else {
		CleanupStaleBackup()
	}
}

func TestGetDefaultPublicKey(t *testing.T) {
	pk := GetDefaultPublicKey()
	if len(pk) != ed25519.PublicKeySize {
		t.Fatalf("expected valid default public key size %d, got %d", ed25519.PublicKeySize, len(pk))
	}
}

func TestRelaunch_Mock(t *testing.T) {
	orig := DefaultRelauncher
	defer func() { DefaultRelauncher = orig }()

	called := false
	DefaultRelauncher = func() error {
		called = true
		return nil
	}

	if err := Relaunch(); err != nil {
		t.Fatalf("unexpected relaunch error: %v", err)
	}
	if !called {
		t.Errorf("expected DefaultRelauncher to be called")
	}
}
