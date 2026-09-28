package game_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/game"
)

func integritySHA1(data []byte) string {
	h := sha1.New()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// integrityHTTP is a minimal ports.HTTPClient for integrity tests: plain GET
// to dest with the same sha1 enforcement semantics as the real adapter.
type integrityHTTP struct {
	served *int64
}

func (c *integrityHTTP) Get(ctx context.Context, url string, _ map[string]string) ([]byte, error) {
	atomic.AddInt64(c.served, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *integrityHTTP) DownloadFile(ctx context.Context, url string, destPath string, expectedSHA1 string, _ func(int64, int64)) error {
	atomic.AddInt64(c.served, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if expectedSHA1 != "" && integritySHA1(data) != expectedSHA1 {
		return fmt.Errorf("sha1 mismatch downloading %s", url)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	tmp := destPath + ".part"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, destPath)
}

type integrityFixture struct {
	server    *httptest.Server
	dataDir   string
	served    int64
	clientJar []byte
}

func newIntegrityFixture(t *testing.T) *integrityFixture {
	t.Helper()
	fixture := &integrityFixture{clientJar: []byte("PK-MOCK-CLIENT-JAR-INTEGRITY")}
	mux := http.NewServeMux()

	// Build documents in dependency order: asset index first, then version
	// json referencing it, then manifest referencing the version json. The
	// httptest server URL is not yet known, so build against a placeholder
	// that is replaced before serving.
	assetData := []byte("MOCK-ASSET-BYTES")
	assetSHA := integritySHA1(assetData)
	assetIndexBytes, _ := json.Marshal(map[string]interface{}{
		"objects": map[string]interface{}{
			"icons/icon_16x16.png": map[string]interface{}{"hash": assetSHA, "size": len(assetData)},
		},
	})
	clientSHA := integritySHA1(fixture.clientJar)

	mux.HandleFunc("/assets-12.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(assetIndexBytes)
	})
	mux.HandleFunc("/client.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture.clientJar)
	})
	mux.HandleFunc("/"+assetSHA[:2]+"/"+assetSHA, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(assetData)
	})
	mux.HandleFunc("/libraries/com/mojang/logging/1.1.1/logging-1.1.1.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("LOGGING-LIB"))
	})

	fixture.server = httptest.NewServer(mux)
	base := fixture.server.URL

	versionBytes, _ := json.Marshal(map[string]interface{}{
		"id":        "1.21.1",
		"type":      "release",
		"mainClass": "net.minecraft.client.main.Main",
		"downloads": map[string]interface{}{
			"client": map[string]interface{}{"sha1": clientSHA, "size": len(fixture.clientJar), "url": base + "/client.jar"},
		},
		"assetIndex": map[string]interface{}{"id": "12", "sha1": integritySHA1(assetIndexBytes), "url": base + "/assets-12.json"},
		"libraries": []map[string]interface{}{{
			"name": "com.mojang:logging:1.1.1",
			"downloads": map[string]interface{}{"artifact": map[string]interface{}{
				"path": "com/mojang/logging/1.1.1/logging-1.1.1.jar", "sha1": integritySHA1([]byte("LOGGING-LIB")), "size": len("LOGGING-LIB"),
			}},
		}},
	})
	mux.HandleFunc("/version.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(versionBytes)
	})
	manifestBytes, _ := json.Marshal(map[string]interface{}{
		"latest":   map[string]string{"release": "1.21.1"},
		"versions": []map[string]interface{}{{"id": "1.21.1", "type": "release", "url": base + "/version.json", "sha1": integritySHA1(versionBytes)}},
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifestBytes)
	})

	fixture.dataDir = t.TempDir()
	return fixture
}

func (f *integrityFixture) service(t *testing.T) *game.GameService {
	t.Helper()
	return game.NewGameService(
		&integrityHTTP{served: &f.served},
		fs.NewOSFileSystem(),
		f.dataDir,
		game.WithManifestURL(f.server.URL+"/manifest.json"),
		game.WithLibrariesBaseURL(f.server.URL+"/libraries"),
		game.WithResourcesBaseURL(f.server.URL),
	)
}

// fillCache provisions the whole local cache directly from the fixture.
func (f *integrityFixture) fillCache(t *testing.T) {
	t.Helper()
	write := func(rel string, data []byte) {
		full := filepath.Join(f.dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	versionResp, err := http.Get(f.server.URL + "/version.json")
	if err != nil {
		t.Fatal(err)
	}
	versionBytes, _ := io.ReadAll(versionResp.Body)
	versionResp.Body.Close()
	write("versions/1.21.1/1.21.1.json", versionBytes)
	write("versions/1.21.1/1.21.1.jar", f.clientJar)
	write("libraries/com/mojang/logging/1.1.1/logging-1.1.1.jar", []byte("LOGGING-LIB"))
	indexResp, err := http.Get(f.server.URL + "/assets-12.json")
	if err != nil {
		t.Fatal(err)
	}
	indexBytes, _ := io.ReadAll(indexResp.Body)
	indexResp.Body.Close()
	write("assets/indexes/12.json", indexBytes)
	var parsed struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(indexBytes, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, obj := range parsed.Objects {
		assetResp, err := http.Get(fmt.Sprintf("%s/%s/%s", f.server.URL, obj.Hash[:2], obj.Hash))
		if err != nil {
			t.Fatal(err)
		}
		assetBytes, _ := io.ReadAll(assetResp.Body)
		assetResp.Body.Close()
		write(filepath.Join("assets/objects", obj.Hash[:2], obj.Hash), assetBytes)
	}
}

func findFinding(res *game.IntegrityResult, reason string) *game.IntegrityFinding {
	for i := range res.Findings {
		if res.Findings[i].Reason == reason {
			return &res.Findings[i]
		}
	}
	return nil
}

func TestIntegrity_CleanCacheReportsZeroProblemsNoTraffic(t *testing.T) {
	fixture := newIntegrityFixture(t)
	defer fixture.server.Close()
	fixture.fillCache(t)

	svc := fixture.service(t)
	inst := &domain.Instance{ID: "inst-int-1", Name: "Int", GameVersion: "1.21.1"}

	before := atomic.LoadInt64(&fixture.served)
	res, err := svc.CheckInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	after := atomic.LoadInt64(&fixture.served)
	if after-before != 1 {
		t.Errorf("expected exactly 1 request (manifest), got %d", after-before)
	}
	if res.ProblemsCount != 0 {
		findings := make([]string, 0)
		for _, f := range res.Findings {
			findings = append(findings, f.Path+": "+f.Reason)
		}
		t.Errorf("expected clean cache, problems=%d [%s]", res.ProblemsCount, strings.Join(findings, " | "))
	}
	if res.CheckedCount < 5 {
		t.Errorf("expected >=5 checked files (json, jar, lib, index, object); got %d", res.CheckedCount)
	}
}

func TestIntegrity_RepairRedownloadsExactlyTheBrokenFile(t *testing.T) {
	fixture := newIntegrityFixture(t)
	defer fixture.server.Close()
	fixture.fillCache(t)

	corrupted := filepath.Join(fixture.dataDir, "versions", "1.21.1", "1.21.1.jar")
	if err := os.WriteFile(corrupted, []byte("TRUNCATED-GARBAGE"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := fixture.service(t)
	inst := &domain.Instance{ID: "inst-int-2", Name: "Int", GameVersion: "1.21.1"}

	res, err := svc.CheckInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if res.ProblemsCount != 1 {
		t.Fatalf("expected exactly 1 problem (corrupted client jar), got %d", res.ProblemsCount)
	}
	if findFinding(res, "ChecksumMismatch") == nil {
		t.Fatalf("expected ChecksumMismatch finding, got %+v", res.Findings)
	}

	before := atomic.LoadInt64(&fixture.served)
	res, err = svc.FixInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	after := atomic.LoadInt64(&fixture.served)
	if res.RepairedCount != 1 {
		t.Fatalf("expected 1 repaired, got %d (problems %d)", res.RepairedCount, res.ProblemsCount)
	}
	if after-before != 2 {
		t.Errorf("expected manifest + exactly 1 repair download, got %d requests", after-before)
	}
	data, err := os.ReadFile(corrupted)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(fixture.clientJar) {
		t.Error("client jar was not restored byte-exact")
	}

	before = atomic.LoadInt64(&fixture.served)
	res, err = svc.CheckInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("post-repair check failed: %v", err)
	}
	if res.ProblemsCount != 0 {
		t.Errorf("post-repair problems = %d, want 0", res.ProblemsCount)
	}
	if after := atomic.LoadInt64(&fixture.served); after-before != 1 {
		t.Errorf("post-repair check should only fetch manifest, got %d requests", after-before)
	}
}

func TestIntegrity_MissingFileReportedAndRepaired(t *testing.T) {
	fixture := newIntegrityFixture(t)
	defer fixture.server.Close()
	fixture.fillCache(t)

	assetSHA := integritySHA1([]byte("MOCK-ASSET-BYTES"))
	target := filepath.Join(fixture.dataDir, "assets", "objects", assetSHA[:2], assetSHA)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	svc := fixture.service(t)
	inst := &domain.Instance{ID: "inst-int-3", Name: "Int", GameVersion: "1.21.1"}
	res, err := svc.CheckInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if res.ProblemsCount != 1 {
		t.Fatalf("expected 1 finding, got %d %+v", res.ProblemsCount, res.Findings)
	}
	if findFinding(res, "Missing") == nil {
		t.Fatalf("expected Missing finding, got %+v", res.Findings)
	}
	res, err = svc.FixInstanceFiles(context.Background(), inst)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if res.RepairedCount != 1 {
		t.Fatalf("expected 1 repaired, got %d (problems=%d findings=%+v)", res.RepairedCount, res.ProblemsCount, res.Findings)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("asset object not restored")
	}
}
