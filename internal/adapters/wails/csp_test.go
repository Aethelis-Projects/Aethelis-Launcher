package wails_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/adapters/wails"
)

func TestCSPMiddleware(t *testing.T) {
	called := false
	h := wails.CSPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("ok")) // errcheck:ok test response
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/index.html", nil))
	if !called {
		t.Fatal("next handler must run")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q; got %q", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Error("CSP must not allow inline/eval scripts")
	}
}

// v0.7.2 round-4 review: the predicted breakage was img-src, not style-src -
// mod icons are REMOTE user data (cdn.modrinth.com / mediafiles.forgecdn.net)
// and data:/blob: have real uses (SVG avatar presets, object URLs). Assert the
// escape hatches exist and STAY minimal: no global wildcards, no scheme-wide
// http:, and img-src must never fall back to default-src 'self'.
func TestCSPPolicyImageSources(t *testing.T) {
	p := wails.CSPPolicy()
	img := ""
	for _, part := range strings.Split(p, "; ") {
		if strings.HasPrefix(part, "img-src ") {
			img = part
		}
	}
	if img == "" {
		t.Fatal("CSP must carry an explicit img-src")
	}
	for _, want := range []string{"'self'", "data:", "blob:", "https://cdn.modrinth.com", "https://mediafiles.forgecdn.net", "*.forgecdn.net"} {
		if !strings.Contains(img, want) {
			t.Errorf("img-src missing %q; got %s", want, img)
		}
	}
	if strings.Contains(img, "*:") || strings.Contains(img, " http:") || strings.Contains(img, "http://*") {
		t.Errorf("img-src too permissive: %s", img)
	}
	if !strings.Contains(p, "style-src 'self' 'unsafe-inline'") {
		t.Error("style-src must allow inline (Solid style attributes)")
	}
}

// The <meta> fallback in the Vite template must stay byte-equivalent to the
// header, otherwise Linux webkit2gtk (where header propagation may be absent)
// enforces a DIFFERENT policy than Windows - exactly the drift the round-4
// review warned about.
func TestIndexHTMLMetaMatchesPolicy(t *testing.T) {
	data, err := readRepoFile("frontend/index.html")
	if err != nil {
		t.Skipf("frontend/index.html unavailable: %v", err)
	}
	want := wails.CSPPolicy()
	if !strings.Contains(string(data), `http-equiv="Content-Security-Policy"`) {
		t.Fatal("frontend/index.html must carry the CSP <meta> fallback")
	}
	metaTag := `http-equiv="Content-Security-Policy" content="`
	i := strings.Index(string(data), metaTag)
	if i < 0 {
		t.Fatal("malformed CSP meta tag")
	}
	rest := string(data)[i+len(metaTag):]
	got := rest[:strings.Index(rest, `"`)]
	if got != want {
		t.Errorf("meta/header CSP drift:\nmeta:   %s\nheader: %s", got, want)
	}
}

func readRepoFile(rel string) ([]byte, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	return os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", rel))
}
