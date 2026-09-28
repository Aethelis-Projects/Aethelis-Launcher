package wails_test

// v0.7.2 review (owner): the transport record must never come from the
// webview. ImportMrPackFromURL only accepts project_slug + version_id and
// re-fetches url/sha1/sha512/size from the Modrinth API itself; client hints
// are optional cross-checks; anything else is rejected before the dial.

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

	// 1. The resolved record itself is validated (https + download-host
	// allowlist): an http origin from the API is refused.
	if _, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		InstanceName: "TrustTest",
		ProjectSlug:  "fabric-skyblocks",
		VersionID:    "verXYZ",
	}); err == nil || !strings.Contains(err.Error(), "allowed download host") {
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

	// 3. There is no url-fallback any more at all.
	if _, err := adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		InstanceName: "TrustTest",
	}); err == nil || !strings.Contains(err.Error(), "requires project_slug and version_id") {
		t.Fatalf("expected identifiers-required rejection, got: %v", err)
	}
}
