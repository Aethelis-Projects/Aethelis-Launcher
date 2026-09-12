package game_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	httpadapter "github.com/nord-launcher/launcher/internal/adapters/http"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/game"
)

func calcSHA1(data []byte) string {
	h := sha1.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func createTestZip(filename, content string) []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	f, _ := w.Create(filename)
	_, _ = f.Write([]byte(content))
	_ = w.Close()
	return buf.Bytes()
}

func setupMockServer(t *testing.T, requestCounter *int64) (*httptest.Server, map[string]string) {
	hashes := make(map[string]string)

	clientJarData := []byte("PK-MOCK-MINECRAFT-CLIENT-JAR-1.21.1")
	hashes["client_1_21_1"] = calcSHA1(clientJarData)

	clientJar112Data := []byte("PK-MOCK-MINECRAFT-CLIENT-JAR-1.12.2")
	hashes["client_1_12_2"] = calcSHA1(clientJar112Data)

	libWindowsData := []byte("MOCK-LIB-WINDOWS-DATA")
	hashes["lib_win"] = calcSHA1(libWindowsData)

	libOsxData := []byte("MOCK-LIB-OSX-DATA")
	hashes["lib_osx"] = calcSHA1(libOsxData)

	nativeZipData := createTestZip("lwjgl.dll", "NATIVE-DLL-BINARY-CONTENT")
	hashes["native_zip"] = calcSHA1(nativeZipData)

	assetObj1Data := []byte("MOCK-SOUND-OR-TEXTURE-OBJECT-1")
	hashes["asset_obj1"] = calcSHA1(assetObj1Data)

	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCounter != nil {
			atomic.AddInt64(requestCounter, 1)
		}
		path := r.URL.Path

		switch {
		case path == "/manifest.json":
			manifest := game.VersionManifestV2{
				Versions: []game.VersionManifestEntry{
					{
						ID:   "1.21.1",
						Type: "release",
						URL:  ts.URL + "/version/1.21.1.json",
						SHA1: hashes["ver_json_1_21_1"],
					},
					{
						ID:   "1.12.2",
						Type: "release",
						URL:  ts.URL + "/version/1.12.2.json",
						SHA1: hashes["ver_json_1_12_2"],
					},
				},
			}
			data, _ := json.Marshal(manifest)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)

		case path == "/version/1.21.1.json":
			ver := domain.VersionJSON{
				ID:        "1.21.1",
				MainClass: "net.minecraft.client.main.Main",
			}
			ver.Downloads.Client = &domain.DownloadArtifactInfo{
				URL:  ts.URL + "/client/1.21.1.jar",
				SHA1: hashes["client_1_21_1"],
				Size: int64(len(clientJarData)),
			}
			ver.AssetIndex = domain.AssetIndexInfo{
				ID:   "1.21",
				URL:  ts.URL + "/assets/indexes/1.21.json",
				SHA1: hashes["asset_index_1_21"],
			}
			// Library with OS rules (allow windows, disallow others)
			ver.Libraries = []domain.Library{
				{
					Name: "org.lwjgl:lwjgl-windows:3.3.3",
					Rules: []domain.Rule{
						{
							Action: "allow",
							OS:     &domain.OSRule{Name: "windows"},
						},
					},
					Downloads: domain.LibraryDownloads{
						Artifact: &domain.LibraryArtifact{
							Path: "org/lwjgl/lwjgl-windows/3.3.3/lwjgl-windows-3.3.3.jar",
							URL:  ts.URL + "/libraries/org/lwjgl/lwjgl-windows/3.3.3/lwjgl-windows-3.3.3.jar",
							SHA1: hashes["lib_win"],
							Size: int64(len(libWindowsData)),
						},
					},
				},
				{
					Name: "org.lwjgl:lwjgl-osx:3.3.3",
					Rules: []domain.Rule{
						{
							Action: "allow",
							OS:     &domain.OSRule{Name: "osx"},
						},
					},
					Downloads: domain.LibraryDownloads{
						Artifact: &domain.LibraryArtifact{
							Path: "org/lwjgl/lwjgl-osx/3.3.3/lwjgl-osx-3.3.3.jar",
							URL:  ts.URL + "/libraries/org/lwjgl/lwjgl-osx/3.3.3/lwjgl-osx-3.3.3.jar",
							SHA1: hashes["lib_osx"],
							Size: int64(len(libOsxData)),
						},
					},
				},
			}
			data, _ := json.Marshal(ver)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)

		case path == "/version/1.12.2.json":
			ver := domain.VersionJSON{
				ID:        "1.12.2",
				MainClass: "net.minecraft.client.main.Main",
			}
			ver.Downloads.Client = &domain.DownloadArtifactInfo{
				URL:  ts.URL + "/client/1.12.2.jar",
				SHA1: hashes["client_1_12_2"],
				Size: int64(len(clientJar112Data)),
			}
			// Library with native classifiers and Maven fallback
			ver.Libraries = []domain.Library{
				{
					Name: "net.minecraft:legacy-maven-lib:1.0",
					// No artifact URL -> triggers maven fallback URL
				},
				{
					Name: "org.lwjgl.lwjgl:lwjgl-platform:2.9.4-nightly-20150209",
					Natives: map[string]string{
						"windows": "natives-windows",
					},
					Downloads: domain.LibraryDownloads{
						Classifiers: map[string]domain.LibraryArtifact{
							"natives-windows": {
								Path: "org/lwjgl/lwjgl-platform/2.9.4/lwjgl-platform-2.9.4-natives-windows.jar",
								URL:  ts.URL + "/libraries/org/lwjgl/lwjgl-platform/2.9.4/lwjgl-platform-2.9.4-natives-windows.jar",
								SHA1: hashes["native_zip"],
								Size: int64(len(nativeZipData)),
							},
						},
					},
				},
			}
			data, _ := json.Marshal(ver)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)

		case path == "/client/1.21.1.jar":
			_, _ = w.Write(clientJarData)
		case path == "/client/1.12.2.jar":
			_, _ = w.Write(clientJar112Data)

		case path == "/assets/indexes/1.21.json":
			idx := game.AssetIndex{
				Objects: map[string]game.AssetObject{
					"icons/icon_16x16.png": {
						Hash: hashes["asset_obj1"],
						Size: int64(len(assetObj1Data)),
					},
				},
			}
			data, _ := json.Marshal(idx)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)

		case path == "/libraries/org/lwjgl/lwjgl-windows/3.3.3/lwjgl-windows-3.3.3.jar":
			_, _ = w.Write(libWindowsData)
		case path == "/libraries/org/lwjgl/lwjgl-osx/3.3.3/lwjgl-osx-3.3.3.jar":
			_, _ = w.Write(libOsxData)
		case path == "/libraries/net/minecraft/legacy-maven-lib/1.0/legacy-maven-lib-1.0.jar":
			_, _ = w.Write([]byte("LEGACY-MAVEN-DATA"))
		case path == "/libraries/org/lwjgl/lwjgl-platform/2.9.4/lwjgl-platform-2.9.4-natives-windows.jar":
			_, _ = w.Write(nativeZipData)

		case strings.HasPrefix(path, "/resources/"):
			_, _ = w.Write(assetObj1Data)

		default:
			http.NotFound(w, r)
		}
	}))

	return ts, hashes
}

func TestGameService_Provision_Modern_1_21_1(t *testing.T) {
	tempDir := t.TempDir()
	fsys := fs.NewOSFileSystem()
	httpClient := httpadapter.NewHTTPClient(10 * time.Second)

	ts, hashes := setupMockServer(t, nil)
	defer ts.Close()

	svc := game.NewGameService(
		httpClient,
		fsys,
		tempDir,
		game.WithManifestURL(ts.URL+"/manifest.json"),
		game.WithLibrariesBaseURL(ts.URL+"/libraries"),
		game.WithResourcesBaseURL(ts.URL+"/resources"),
		game.WithPlatform("windows", "amd64"),
	)

	inst := &domain.Instance{
		ID:          "inst-modern",
		Name:        "Test 1.21.1",
		GameVersion: "1.21.1",
		Loader:      domain.LoaderVanilla,
	}
	acc := &domain.Account{
		UUID:        "test-uuid-1",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
	}

	cfg, err := svc.Provision(context.Background(), inst, acc)
	if err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	if cfg.Instance.ID != "inst-modern" {
		t.Errorf("expected instance inst-modern, got %s", cfg.Instance.ID)
	}
	if !fsys.Exists(cfg.ClientJarPath) {
		t.Errorf("expected client jar to exist at %s", cfg.ClientJarPath)
	}
	if len(cfg.LibraryJarList) != 1 {
		t.Errorf("expected 1 library jar (windows only), got %d", len(cfg.LibraryJarList))
	}
	if !strings.Contains(cfg.LibraryJarList[0], "lwjgl-windows") {
		t.Errorf("expected lwjgl-windows in classpath, got %s", cfg.LibraryJarList[0])
	}

	// Verify asset object was downloaded
	hash := hashes["asset_obj1"]
	expectedAsset := filepath.Join(tempDir, "assets", "objects", hash[:2], hash)
	if !fsys.Exists(expectedAsset) {
		t.Errorf("expected asset object at %s", expectedAsset)
	}
}

func TestGameService_Provision_Legacy_1_12_2(t *testing.T) {
	tempDir := t.TempDir()
	fsys := fs.NewOSFileSystem()
	httpClient := httpadapter.NewHTTPClient(10 * time.Second)

	ts, _ := setupMockServer(t, nil)
	defer ts.Close()

	svc := game.NewGameService(
		httpClient,
		fsys,
		tempDir,
		game.WithManifestURL(ts.URL+"/manifest.json"),
		game.WithLibrariesBaseURL(ts.URL+"/libraries"),
		game.WithResourcesBaseURL(ts.URL+"/resources"),
		game.WithPlatform("windows", "amd64"),
	)

	inst := &domain.Instance{
		ID:          "inst-legacy",
		Name:        "Test 1.12.2",
		GameVersion: "1.12.2",
		Loader:      domain.LoaderVanilla,
	}
	acc := &domain.Account{
		UUID:        "test-uuid-2",
		Username:    "Alex",
		Type:        domain.AccountMicrosoft,
		AccessToken: "mock-token",
	}

	cfg, err := svc.Provision(context.Background(), inst, acc)
	if err != nil {
		t.Fatalf("legacy provision failed: %v", err)
	}

	// Verify native dll extracted into nativesDir
	extractedNative := filepath.Join(cfg.NativesDir, "lwjgl.dll")
	if !fsys.Exists(extractedNative) {
		t.Errorf("expected extracted native at %s", extractedNative)
	}
	data, _ := os.ReadFile(extractedNative)
	if string(data) != "NATIVE-DLL-BINARY-CONTENT" {
		t.Errorf("unexpected extracted native content: %s", string(data))
	}
}

func TestGameService_Provision_Idempotency(t *testing.T) {
	tempDir := t.TempDir()
	fsys := fs.NewOSFileSystem()
	httpClient := httpadapter.NewHTTPClient(10 * time.Second)

	var requestCount int64
	ts, _ := setupMockServer(t, &requestCount)
	defer ts.Close()

	svc := game.NewGameService(
		httpClient,
		fsys,
		tempDir,
		game.WithManifestURL(ts.URL+"/manifest.json"),
		game.WithLibrariesBaseURL(ts.URL+"/libraries"),
		game.WithResourcesBaseURL(ts.URL+"/resources"),
		game.WithPlatform("windows", "amd64"),
	)

	inst := &domain.Instance{
		ID:          "inst-idempotent",
		Name:        "Test Idempotent",
		GameVersion: "1.21.1",
	}
	acc := &domain.Account{
		UUID:     "uuid-1",
		Username: "Player",
		Type:     domain.AccountMicrosoft,
	}

	// 1st Provision -> downloads everything
	_, err := svc.Provision(context.Background(), inst, acc)
	if err != nil {
		t.Fatalf("first provision failed: %v", err)
	}
	firstReqs := atomic.LoadInt64(&requestCount)

	// 2nd Provision -> should only fetch manifest (to resolve version), all files cached and verified by sha1
	_, err = svc.Provision(context.Background(), inst, acc)
	if err != nil {
		t.Fatalf("second provision failed: %v", err)
	}
	secondReqs := atomic.LoadInt64(&requestCount) - firstReqs

	// Exactly 1 request in second run (manifest.json). Client jar, libraries, assets were skipped!
	if secondReqs > 1 {
		t.Errorf("expected at most 1 request on second run (cached), got %d", secondReqs)
	}
}

func TestGameService_Errors(t *testing.T) {
	tempDir := t.TempDir()
	fsys := fs.NewOSFileSystem()
	httpClient := httpadapter.NewHTTPClient(10 * time.Second)

	ts, _ := setupMockServer(t, nil)
	defer ts.Close()

	svc := game.NewGameService(
		httpClient,
		fsys,
		tempDir,
		game.WithManifestURL(ts.URL+"/manifest.json"),
	)

	// 1. Nil instance
	_, err := svc.Provision(context.Background(), nil, &domain.Account{})
	if !errors.Is(err, domain.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	// 2. Nil account
	_, err = svc.Provision(context.Background(), &domain.Instance{}, nil)
	if !errors.Is(err, domain.ErrNoActiveAccount) {
		t.Errorf("expected ErrNoActiveAccount, got %v", err)
	}

	// 3. Version not found
	_, err = svc.Provision(context.Background(), &domain.Instance{GameVersion: "99.99.99"}, &domain.Account{})
	if !errors.Is(err, domain.ErrVersionNotFound) {
		t.Errorf("expected ErrVersionNotFound, got %v", err)
	}

	// 4. Broken manifest URL
	brokenSvc := game.NewGameService(
		httpClient,
		fsys,
		tempDir,
		game.WithManifestURL("http://127.0.0.1:9/broken.json"),
	)
	_, err = brokenSvc.Provision(context.Background(), &domain.Instance{GameVersion: "1.21.1"}, &domain.Account{})
	if !errors.Is(err, domain.ErrDownloadFailed) {
		t.Errorf("expected ErrDownloadFailed, got %v", err)
	}
}

func TestMavenCoordinatesToPath(t *testing.T) {
	cases := []struct {
		coords   string
		expected string
	}{
		{
			coords:   "org.lwjgl:lwjgl:3.3.3",
			expected: "org/lwjgl/lwjgl/3.3.3/lwjgl-3.3.3.jar",
		},
		{
			coords:   "net.java.jinput:jinput-platform:2.0.5:natives-windows",
			expected: "net/java/jinput/jinput-platform/2.0.5/jinput-platform-2.0.5-natives-windows.jar",
		},
		{
			coords:   "com.mojang:patchy:1.1@zip",
			expected: "com/mojang/patchy/1.1/patchy-1.1.zip",
		},
	}

	for _, tc := range cases {
		actual := game.MavenCoordinatesToPath(tc.coords)
		if actual != tc.expected {
			t.Errorf("coords %q: expected %q, got %q", tc.coords, tc.expected, actual)
		}
	}
}
