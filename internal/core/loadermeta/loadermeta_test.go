package loadermeta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jsonServer(t *testing.T, payload any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload) // errcheck:ok test helper
	}))
}

func newTestResolver(srv *httptest.Server) *Resolver {
	r := NewResolver(srv.Client())
	r.FabricURL = srv.URL + "/fabric/"
	r.QuiltURL = srv.URL + "/quilt/"
	r.ForgeURL = srv.URL + "/forge"
	r.NeoURL = srv.URL + "/neo"
	return r
}

func TestResolveFabric(t *testing.T) {
	srv := jsonServer(t, []map[string]any{
		{"loader": map[string]any{"version": "0.16.9", "stable": false}},
		{"loader": map[string]any{"version": "0.16.4", "stable": true}},
		{"loader": map[string]any{"version": "0.15.11", "stable": true}},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "fabric", "1.21.4")
	if err != nil {
		t.Fatalf("resolve fabric: %v", err)
	}
	if opt.Default != "0.16.4" {
		t.Fatalf("fabric default = %q, want first stable 0.16.4", opt.Default)
	}
	if len(opt.Options) != 3 || opt.Options[0] != "0.16.9" {
		t.Fatalf("fabric options = %v", opt.Options)
	}
}

func TestResolveFabricNoStableFallsBackToNewest(t *testing.T) {
	srv := jsonServer(t, []map[string]any{
		{"loader": map[string]any{"version": "0.99.0", "stable": false}},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "fabric", "26.3")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opt.Default != "0.99.0" {
		t.Fatalf("default = %q, want newest 0.99.0", opt.Default)
	}
}

func TestResolveQuilt(t *testing.T) {
	srv := jsonServer(t, []map[string]any{
		{"loader": map[string]any{"version": "0.26.0-beta.2"}},
		{"loader": map[string]any{"version": "0.25.0"}},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "quilt", "1.21.1")
	if err != nil {
		t.Fatalf("resolve quilt: %v", err)
	}
	if opt.Default != "0.26.0-beta.2" {
		t.Fatalf("quilt default = %q, want newest first", opt.Default)
	}
}

func TestResolveForgePrefersRecommended(t *testing.T) {
	srv := jsonServer(t, map[string]any{
		"promos": map[string]string{
			"1.20.1-recommended": "47.3.0-forge",
			"1.20.1-latest":      "47.3.5-forge",
		},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "forge", "1.20.1")
	if err != nil {
		t.Fatalf("resolve forge: %v", err)
	}
	if opt.Default != "47.3.0" {
		t.Fatalf("forge default = %q, want recommended 47.3.0 (suffix trimmed)", opt.Default)
	}
	if len(opt.Options) != 1 || opt.Options[0] != "47.3.0" {
		t.Fatalf("forge options = %v", opt.Options)
	}
}

func TestResolveForgeLatestFallback(t *testing.T) {
	srv := jsonServer(t, map[string]any{
		"promos": map[string]string{"26.2-latest": "8.0.163-forge"},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "forge", "26.2")
	if err != nil {
		t.Fatalf("resolve forge: %v", err)
	}
	if opt.Default != "8.0.163" {
		t.Fatalf("forge default = %q, want latest 8.0.163", opt.Default)
	}
}

func TestResolveNeoForgeLineMatch(t *testing.T) {
	srv := jsonServer(t, []map[string]string{
		{"name": "21.4.100-beta"},
		{"name": "21.4.50"},
		{"name": "21.5.2"},
		{"name": "20.4.0"},
		{"name": "not-a-tag"},
	})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "neoforge", "1.21.4")
	if err != nil {
		t.Fatalf("resolve neoforge: %v", err)
	}
	if len(opt.Options) != 2 || opt.Options[0] != "21.4.100-beta" || opt.Default != "21.4.100-beta" {
		t.Fatalf("neoforge options = %v default=%q, want only 21.4.x newest-first", opt.Options, opt.Default)
	}
}

func TestResolveNeoForgeOldLineEmpty(t *testing.T) {
	srv := jsonServer(t, []map[string]string{{"name": "21.4.0"}})
	defer srv.Close()
	opt, err := newTestResolver(srv).Resolve(context.Background(), "neoforge", "1.20.4")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(opt.Options) != 0 || opt.Default != "" {
		t.Fatalf("expected empty neoforge result for 1.20.4, got %+v", opt)
	}
}

func TestResolveVanillaAndUnknown(t *testing.T) {
	srv := jsonServer(t, []map[string]any{})
	defer srv.Close()
	r := newTestResolver(srv)
	opt, err := r.Resolve(context.Background(), "vanilla", "1.21.4")
	if err != nil || len(opt.Options) != 0 || opt.Source != "vanilla" {
		t.Fatalf("vanilla should short-circuit, got %+v %v", opt, err)
	}
	if _, err := r.Resolve(context.Background(), "sponge", "1.21.4"); err == nil {
		t.Fatal("expected error for unsupported loader")
	}
}

func TestResolveTransportErrorIsHardError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := newTestResolver(srv).Resolve(context.Background(), "fabric", "1.21.4")
	if err == nil {
		t.Fatal("expected transport error for non-200")
	}
}
