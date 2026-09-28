package wails_test

import (
	"net/http"
	"net/http/httptest"
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
