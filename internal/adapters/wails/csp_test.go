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
	for _, want := range []string{"default-src 'self'", "script-src 'self' 'unsafe-inline' wails:", "object-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q; got %q", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-eval") {
		t.Error("CSP must not allow eval scripts")
	}
	if !strings.Contains(csp, "'unsafe-inline'") {
		t.Error("CSP must allow inline script for Wails v3 IPC bridge injection")
	}
	if !strings.Contains(csp, "wails:") {
		t.Error("CSP must allow wails: scheme for Wails v3 runtime loading")
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
	if !strings.Contains(p, "frame-ancestors 'none'") {
		t.Error("header policy must pin frame-ancestors")
	}
}

// v0.7.2 round-5 (owner p3): frame-ancestors/sandbox/report-* are IGNORED in a
// <meta> and the browser logs a warning for each - which would fail the
// "console must be clean" acceptance item. The meta twin therefore carries the
// policy minus exactly those directives.
func TestCSPPolicyForMetaDropsHeaderOnlyDirectives(t *testing.T) {
	full := wails.CSPPolicy()
	meta := wails.CSPPolicyForMeta()
	for _, d := range []string{"frame-ancestors", "sandbox", "report-uri", "report-to", "trusted-types"} {
		if strings.Contains(meta, d) {
			t.Errorf("meta policy must not carry %q (ignored via <meta> + console warning)", d)
		}
	}
	for _, d := range []string{"default-src 'self'", "script-src 'self' 'unsafe-inline' wails:", "img-src", "connect-src 'self'", "object-src 'none'"} {
		if !strings.Contains(meta, d) {
			t.Errorf("meta policy lost %q", d)
		}
	}
	if strings.Contains(full, "frame-ancestors") == strings.Contains(meta, "frame-ancestors") {
		t.Error("meta variant must differ from the header only by the dropped directives")
	}
	if strings.Count(meta, "; ")+1 >= strings.Count(full, "; ")+1 {
		t.Errorf("meta must be strictly shorter than the header policy (dropped %d directives)", 0)
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
	want := wails.CSPPolicyForMeta()
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

// TestWailsRuntimeScriptPermittedByCSP ensures the regression in v0.7.2
// (Wails IPC bridge completely dead due to missing 'unsafe-inline' and 'wails:')
// cannot happen again. The webview must be allowed to execute the inline module
// bootstrap script and load /wails/runtime.js via the wails: scheme, while
// keeping unsafe-eval strictly banned.
func TestWailsRuntimeScriptPermittedByCSP(t *testing.T) {
	policy := wails.CSPPolicy()
	metaPolicy := wails.CSPPolicyForMeta()

	for _, p := range []string{policy, metaPolicy} {
		if !strings.Contains(p, "script-src 'self' 'unsafe-inline' wails:") {
			t.Errorf("CSP must permit 'unsafe-inline' and 'wails:' on script-src for Wails IPC bridge bootstrap; got %q", p)
		}
		if strings.Contains(p, "unsafe-eval") {
			t.Errorf("CSP must not permit 'unsafe-eval'; got %q", p)
		}
	}

	htmlData, err := readRepoFile("frontend/index.html")
	if err != nil {
		t.Skipf("frontend/index.html unavailable: %v", err)
	}
	htmlStr := string(htmlData)
	if !strings.Contains(htmlStr, `window.wails = wails`) {
		t.Errorf("frontend/index.html must bootstrap window.wails for IPC bridge")
	}
	if !strings.Contains(htmlStr, `/wails/runtime.js`) {
		t.Errorf("frontend/index.html must load /wails/runtime.js")
	}
}

func readRepoFile(rel string) ([]byte, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	return os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", rel))
}
