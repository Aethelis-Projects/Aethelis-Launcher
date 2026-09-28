package wails_test

// v0.7.2 review (owner point 2): the url+sha1 pair must not be trusted from
// the webview. These tests pin the resolution-first contract: with
// project_slug+version_id the adapter re-fetches the file record from the
// Modrinth API and rejects disagreeing client hints; without them only https
// on an allowed CDN host is dialed, redirects included (see e2e STEP 15 for
// the full download+import path over the TLS fixture).

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
)

func TestWailsAdapter_MrpackURLImport_GoSideResolution(t *testing.T) {
	payload := []byte(strings.Repeat("nord-mrpack-bytes-", 256))
	sum := sha1.Sum(payload)
	sha1hex := fmt.Sprintf("%x", sum)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/project/") && strings.HasSuffix(r.URL.Path, "/version") {
			resp := []map[string]any{{
				"id":            "verXYZ",
				"name":          "Fabric Skyblocks 2.6.0",
				"version_type":  "release",
				"game_versions": []string{"1.21.4"},
				"loaders":       []string{"fabric"},
				"files": []map[string]any{{
					// Plain http: the adapter must refuse to dial even a
					// *server-declared* record that left https.
					"hashes":   map[string]string{"sha1": sha1hex},
					"url":      "http://127.0.0.1:9/pack.mrpack",
					"filename": "pack.mrpack",
					"size":     len(payload),
					"primary":  true,
				}},
			}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp) // errcheck:ok test fixture
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()

	adapter := wails.NewWailsAdapter(nil)
	wails.NewHost(adapter).SetContent(modrinth.NewClient(api.URL, api.Client()), nil)

	// 1. Resolution mode ignores the client URL entirely: the error must come
	// from the *resolved* record (http origin), not from the pristine https
	// URL the (hypothetically compromised) webview supplied.
	_, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		URL:          "https://cdn.modrinth.com/data/x/versions/v/p.mrpack",
		InstanceName: "TrustTest",
		ProjectSlug:  "fabric-skyblocks",
		VersionID:    "verXYZ",
	})
	if err == nil || !strings.Contains(err.Error(), "allowed download host") {
		t.Fatalf("expected resolved-origin https/allowlist enforcement, got: %v", err)
	}

	// 2. A tampered sha1 hint is rejected before any dial.
	if _, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		InstanceName: "TrustTest",
		ProjectSlug:  "fabric-skyblocks",
		VersionID:    "verXYZ",
		SHA1:         fmt.Sprintf("%040x", 0xdeadbeef),
	}); err == nil || !strings.Contains(err.Error(), "does not match the Modrinth API record") {
		t.Fatalf("expected sha1 mismatch rejection, got: %v", err)
	}

	// 3. Fallback mode: untrusted origin rejected even with a valid hash.
	if _, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		URL:          "https://cdn.evil.test/pack.mrpack",
		InstanceName: "TrustTest",
		SHA1:         sha1hex,
	}); err == nil || !strings.Contains(err.Error(), "not an allowed download host") {
		t.Fatalf("expected host-allowlist rejection, got: %v", err)
	}

	// 4. Fallback mode without any hash is refused (no unverifiable downloads).
	if _, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		URL:          "https://cdn.modrinth.com/pack.mrpack",
		InstanceName: "TrustTest",
	}); err == nil || !strings.Contains(err.Error(), "sha1 or sha512") {
		t.Fatalf("expected hash-mandatory rejection, got: %v", err)
	}
}
