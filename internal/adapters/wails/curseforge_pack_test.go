package wails

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

func TestCFPackResolver_ExactFileMatch(t *testing.T) {
	data := []byte("SODIUM-BYTES")
	h := sha1.Sum(data)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/mods/1/files") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id":          10,
						"modId":       1,
						"fileName":    "sodium-1.0.jar",
						"downloadUrl": "https://edge.forgecdn.net/files/10.jar",
						"fileLength":  len(data),
						"releaseType": 1,
						"hashes":      []map[string]interface{}{{"value": hex.EncodeToString(h[:]), "algo": 1}},
					},
					{
						"id":          11,
						"modId":       1,
						"fileName":    "sodium-0.9.jar",
						"downloadUrl": "https://edge.forgecdn.net/files/11.jar",
						"fileLength":  1,
						"releaseType": 1,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := curseforge.NewClient(ts.URL, "test-key", ts.Client())
	r := &cfPackResolver{client: client}
	url, sha1Hex, size, name, err := r.ResolvePackFile(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if url != "https://edge.forgecdn.net/files/10.jar" || size != int64(len(data)) || name != "sodium-1.0.jar" || sha1Hex != hex.EncodeToString(h[:]) {
		t.Fatalf("resolve fields wrong: %s / %s / %d / %s", url, sha1Hex, size, name)
	}

	if _, _, _, _, err := r.ResolvePackFile(context.Background(), 1, 999); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown file must error with not-found, got %v", err)
	}
}

func TestCFPackResolver_NoKeyAndNilClient(t *testing.T) {
	if _, _, _, _, err := (*cfPackResolver)(nil).ResolvePackFile(context.Background(), 1, 2); err == nil {
		t.Fatal("nil resolver must error, not silently succeed")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	c := curseforge.NewClient(ts.URL, "", ts.Client())
	c.SetAPIKey("")
	r := &cfPackResolver{client: c}
	if _, _, _, _, err := r.ResolvePackFile(context.Background(), 1, 2); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("missing key must surface as explicit error, got %v", err)
	}
}

func writeCFZipForAdapter(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range entries {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte(c))
	}
	_ = zw.Close()
	p := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWailsAdapter_ScanCurseForgePackZip_NilResolverHonest(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestType": "minecraftModpack",
		"name":         "No Net Pack",
		"minecraft": map[string]interface{}{
			"version":    "1.20.1",
			"modLoaders": []map[string]string{{"id": "fabric-0.15.0"}},
		},
		"files": []map[string]interface{}{{"projectID": 1, "fileID": 10}},
	}
	mj, _ := json.Marshal(manifest)
	zipPath := writeCFZipForAdapter(t, map[string]string{"manifest.json": string(mj)})

	adapter := NewWailsAdapter(nil)
	adapter.SetCurseForgePackImporter(launch.NewCurseForgePackImporter(nil, nil, t.TempDir(), nil))

	plan, err := adapter.ScanCurseForgePackZip(CFPackScanRequest{ZipPath: zipPath})
	if err != nil {
		t.Fatalf("adapter scan failed: %v", err)
	}
	if plan.Format != "manifest" || plan.InstanceName != "No Net Pack" || plan.GameVersion != "1.20.1" || plan.Loader != "fabric" {
		t.Fatalf("plan metadata wrong: %+v", plan)
	}
	if len(plan.Files) != 0 || len(plan.Unresolved) != 1 {
		t.Fatalf("without resolver everything must be unresolved: %+v", plan)
	}
	if !strings.Contains(plan.Unresolved[0].ResolveErr, "not initialized") {
		t.Fatalf("resolve reason must be explicit: %+v", plan.Unresolved[0])
	}
}

func TestWailsAdapter_DiscordRpc_ToggleAndStatusWithoutService(t *testing.T) {
	adapter := NewWailsAdapter(nil)
	// No settings repo wired: toggle must persist silently-fail-open (nil repo
	// is a valid embedded mode), status must degrade to all-off, never panic.
	if err := adapter.SetDiscordRpcEnabled(true); err != nil {
		t.Fatalf("toggle without repo: %v", err)
	}
	st, err := adapter.GetDiscordRpcStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Connected {
		t.Fatalf("without settings repo enabled/connected must read off: %+v", st)
	}
	if !st.AppIDSet {
		t.Fatalf("builtin app id must always report set: %+v", st)
	}
	st, err = adapter.GetDiscordRpcStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled {
		t.Fatal("enabled is repo-backed; must stay false without a repo")
	}
	// v0.7.2 G8: the app id ships as a binary constant, so status always
	// reports it set — even before any manager exists.
	if !st.AppIDSet {
		t.Fatal("builtin app id must report as set")
	}
}
