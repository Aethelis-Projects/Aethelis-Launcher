package launch_test

import (
	"archive/zip"
	"bytes"
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
	"testing"

	"github.com/nord-launcher/launcher/internal/core/launch"
)

func cfPackSHA1(data []byte) string {
	h := sha1.New()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

type cfPackFakeHTTP struct{ failFor string }

func (c *cfPackFakeHTTP) Get(ctx context.Context, url string, _ map[string]string) ([]byte, error) {
	return nil, fmt.Errorf("unused")
}

func (c *cfPackFakeHTTP) DownloadFile(ctx context.Context, url string, dest string, expectedSHA1 string, _ func(int64, int64)) error {
	if strings.Contains(url, c.failFor) && c.failFor != "" {
		return fmt.Errorf("simulated network failure for %s", c.failFor)
	}
	resp, err := http.Get(url)
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
	if expectedSHA1 != "" && cfPackSHA1(data) != expectedSHA1 {
		return fmt.Errorf("sha1 mismatch for %s", url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

type cfPackFakeResolver struct {
	jarData  []byte
	altData  []byte
	dlServer string
}

func (r *cfPackFakeResolver) ResolvePackFile(_ context.Context, projectID, fileID int64) (string, string, int64, string, error) {
	switch {
	case projectID == 1 && fileID == 10:
		return r.dlServer + "/file/10.jar", cfPackSHA1(r.jarData), int64(len(r.jarData)), "sodium-1.0.jar", nil
	case projectID == 2 && fileID == 20:
		return r.dlServer + "/file/20.jar", cfPackSHA1(r.altData), int64(len(r.altData)), "lithium-1.0.jar", nil
	default:
		return "", "", 0, "", fmt.Errorf("file %d not found under project %d", fileID, projectID)
	}
}

func writeCFPackZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pack.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

func cfManifestJSON(requiredMissing bool) string {
	files := []map[string]interface{}{
		{"projectID": 1, "fileID": 10},
		{"projectID": 2, "fileID": 20},
		{"projectID": 3, "fileID": 30},
	}
	if requiredMissing {
		files[0]["required"] = false
	}
	m := map[string]interface{}{
		"manifestType":    "minecraftModpack",
		"manifestVersion": 1,
		"name":            "Epic Pack",
		"version":         1,
		"minecraft": map[string]interface{}{
			"version":    "1.20.1",
			"modLoaders": []map[string]string{{"id": "forge-14.23.5.2847"}},
		},
		"files": files,
	}
	data, _ := json.Marshal(m)
	return string(data)
}

func setupCFPackImporter(t *testing.T, resolver launch.CurseForgePackFileResolver, failFor string) (*launch.CurseForgePackImporter, string) {
	t.Helper()
	repo := newMockRepo()
	instancesDir := t.TempDir()
	svc := launch.NewInstanceService(repo, nil, nil, nil, mockClock{})
	imp := launch.NewCurseForgePackImporter(svc, &cfPackFakeHTTP{failFor: failFor}, instancesDir, resolver)
	return imp, instancesDir
}

func TestCFPack_Scan_Manifest(t *testing.T) {
	zipPath := writeCFPackZip(t, map[string]string{
		"manifest.json":                    cfManifestJSON(false),
		"overrides/options.txt":            "fov:90.0\n",
		"overrides/config/x.toml":          "a=1\n",
		"overrides/launcher_accounts.json": "{\"tokens\":[]}\n",
	})
	dl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file/10.jar" {
			_, _ = w.Write([]byte("SODIUM-BYTES"))
			return
		}
		if r.URL.Path == "/file/20.jar" {
			_, _ = w.Write([]byte("LITHIUM-BYTES"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer dl.Close()

	resolver := &cfPackFakeResolver{jarData: []byte("SODIUM-BYTES"), altData: []byte("LITHIUM-BYTES"), dlServer: dl.URL}
	imp, _ := setupCFPackImporter(t, resolver, "")

	plan, err := imp.ScanCurseForgeZip(context.Background(), zipPath)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if plan.Format != "manifest" || plan.GameVersion != "1.20.1" || plan.Loader != "forge" || plan.LoaderVersion != "14.23.5.2847" {
		t.Fatalf("plan metadata wrong: %+v", plan)
	}
	if len(plan.Files) != 2 {
		t.Fatalf("expected 2 resolvable files, got %d: %+v", len(plan.Files), plan.Files)
	}
	if len(plan.Unresolved) != 1 {
		t.Fatalf("expected 1 unresolved (3/30), got %+v", plan.Unresolved)
	}
	found := false
	for _, n := range plan.BlockedNames {
		if strings.Contains(n, "launcher_accounts") {
			found = true
		}
	}
	if !found {
		t.Fatalf("credential override must be blocked, got %+v", plan.BlockedNames)
	}
	if len(plan.OverrideNames) != 2 {
		t.Fatalf("expected 2 safe overrides, got %+v", plan.OverrideNames)
	}
}

func TestCFPack_Import_EndToEnd(t *testing.T) {
	zipPath := writeCFPackZip(t, map[string]string{
		"manifest.json":                    cfManifestJSON(false),
		"overrides/options.txt":            "fov:90.0\n",
		"overrides/config/x.toml":          "a=1\n",
		"overrides/launcher_accounts.json": "{\"tokens\":[]}\n",
	})
	dl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file/10.jar" {
			_, _ = w.Write([]byte("SODIUM-BYTES"))
			return
		}
		if r.URL.Path == "/file/20.jar" {
			_, _ = w.Write([]byte("LITHIUM-BYTES"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer dl.Close()

	resolver := &cfPackFakeResolver{jarData: []byte("SODIUM-BYTES"), altData: []byte("LITHIUM-BYTES"), dlServer: dl.URL}
	imp, instancesDir := setupCFPackImporter(t, resolver, "")

	plan, err := imp.ScanCurseForgeZip(context.Background(), zipPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := imp.ImportCurseForgeZip(context.Background(), plan)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if res.Downloaded != 2 || res.OverrideFiles != 2 {
		t.Fatalf("res wrong: %+v", res)
	}
	if len(res.Unresolved) != 1 || !strings.Contains(res.Unresolved[0], "3/30") {
		t.Fatalf("unresolved report wrong: %+v", res.Unresolved)
	}
	if len(res.SkippedCred) != 1 {
		t.Fatalf("expected credential skip report, got %+v", res.SkippedCred)
	}
	instDir := filepath.Join(instancesDir, res.InstanceID)
	for _, want := range []string{"mods/sodium-1.0.jar", "mods/lithium-1.0.jar", "overrides_options.txt_marker"} {
		if want == "overrides_options.txt_marker" {
			data, err := os.ReadFile(filepath.Join(instDir, "options.txt"))
			if err != nil || string(data) != "fov:90.0\n" {
				t.Fatalf("override options.txt missing/corrupt: %v", err)
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(instDir, want)); err != nil {
			t.Errorf("expected %s to exist: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(instDir, "launcher_accounts.json")); !os.IsNotExist(err) {
		t.Error("credential file leaked into instance dir")
	}
	if _, err := os.Stat(filepath.Join(instDir, "config", "x.toml")); err != nil {
		t.Errorf("nested override missing: %v", err)
	}
}

func TestCFPack_RequiredDownloadFailureAbortsLoudly(t *testing.T) {
	zipPath := writeCFPackZip(t, map[string]string{
		"manifest.json": cfManifestJSON(false), // file 10 is required
	})
	dl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dl.Close()
	resolver := &cfPackFakeResolver{jarData: []byte("X"), altData: []byte("Y"), dlServer: dl.URL}
	imp, _ := setupCFPackImporter(t, resolver, "")

	plan, err := imp.ScanCurseForgeZip(context.Background(), zipPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = imp.ImportCurseForgeZip(context.Background(), plan)
	if err == nil {
		t.Fatal("required download failure must abort the import with error")
	}
	if !strings.Contains(err.Error(), "required mod") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestCFPack_ModlistHTMLIsHonest(t *testing.T) {
	html := `<html><body><ul><li><a href="https://www.curseforge.com/minecraft/mc-mods/sodium">Sodium</a></li></ul></body></html>`
	zipPath := writeCFPackZip(t, map[string]string{"modlist.html": html})
	imp, _ := setupCFPackImporter(t, nil, "")
	plan, err := imp.ScanCurseForgeZip(context.Background(), zipPath)
	if err != nil {
		t.Fatalf("modlist scan must not fail: %v", err)
	}
	if plan.Format != "modlist-html" || len(plan.Unresolved) != 1 {
		t.Fatalf("modlist plan wrong: %+v", plan)
	}
	if _, err := imp.ImportCurseForgeZip(context.Background(), plan); err == nil {
		t.Fatal("import of modlist-only plan must be refused, not half-done")
	}
}

func TestCFPack_ZipSlipRejected(t *testing.T) {
	zipPath := writeCFPackZip(t, map[string]string{
		"manifest.json":        cfManifestJSON(false),
		"overrides/../../evil": "payload",
	})
	imp, _ := setupCFPackImporter(t, nil, "")
	if _, err := imp.ScanCurseForgeZip(context.Background(), zipPath); err == nil {
		t.Fatal("traversal override must be rejected at scan time")
	}
}

func TestCFPack_NotAPack(t *testing.T) {
	zipPath := writeCFPackZip(t, map[string]string{"README.txt": "just a zip"})
	imp, _ := setupCFPackImporter(t, nil, "")
	if _, err := imp.ScanCurseForgeZip(context.Background(), zipPath); err == nil {
		t.Fatal("plain zip must be rejected as non-pack")
	}
}
