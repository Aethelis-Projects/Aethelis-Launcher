package loaders_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/content/loaders"
)

func TestFabricLoader_GetVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/versions/loader/1.21.1" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"loader": map[string]any{
					"version": "0.16.5",
					"stable":  true,
					"maven":   "net.fabricmc:fabric-loader:0.16.5",
				},
			},
			{
				"loader": map[string]any{
					"version": "0.16.4",
					"stable":  false,
					"maven":   "net.fabricmc:fabric-loader:0.16.4",
				},
			},
		})
	}))
	defer server.Close()

	client := loaders.NewFabricClient(server.URL, server.Client())
	versions, err := client.GetLoadersForGameVersion(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("fetch fabric loaders failed: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != "0.16.5" || !versions[0].Stable {
		t.Fatalf("unexpected first version: %+v", versions[0])
	}
}

func TestNeoForgeLoader_GetVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/maven/versions/releases/net/neoforged/neoforge" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"versions": []string{
				"20.4.80",
				"20.4.81",
				"21.1.65",
				"21.1.66-beta",
			},
		})
	}))
	defer server.Close()

	client := loaders.NewNeoForgeClient(server.URL, server.Client())
	versions, err := client.GetLoadersForGameVersion(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("fetch neoforge loaders failed: %v", err)
	}

	// Should match 21.1.x and exclude 20.4.x
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions for 1.21.1, got %d", len(versions))
	}
	if versions[0].Version != "21.1.66-beta" || versions[0].Stable {
		t.Fatalf("unexpected beta version: %+v", versions[0])
	}
	if versions[1].Version != "21.1.65" || !versions[1].Stable {
		t.Fatalf("unexpected release version: %+v", versions[1])
	}
}