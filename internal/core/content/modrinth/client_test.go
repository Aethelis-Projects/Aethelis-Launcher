package modrinth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
)

func TestModrinthClient_SearchMods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/search" {
			http.NotFound(w, r)
			return
		}

		ua := r.Header.Get("User-Agent")
		if ua == "" {
			t.Error("expected User-Agent header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits": []map[string]any{
				{
					"project_id":    "AANobbMI",
					"slug":          "sodium",
					"title":         "Sodium",
					"description":   "Modern rendering engine for Minecraft",
					"author":        "jellysquid3",
					"icon_url":      "https://cdn.modrinth.com/sodium.png",
					"downloads":     15000000,
					"follows":       45000,
					"categories":    []string{"fabric", "optimization"},
					"versions":      []string{"1.21.1"},
					"date_modified": time.Now().Format(time.RFC3339),
				},
			},
			"total_hits": 1,
			"offset":     0,
			"limit":      20,
		})
	}))
	defer server.Close()

	client := modrinth.NewClient(server.URL, server.Client())

	mods, total, err := client.SearchMods(context.Background(), "sodium", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("search mods failed: %v", err)
	}

	if total != 1 || len(mods) != 1 {
		t.Fatalf("expected 1 mod hit, got %d (total %d)", len(mods), total)
	}

	mod := mods[0]
	if mod.ID != "AANobbMI" || mod.Slug != "sodium" || mod.Name != "Sodium" {
		t.Fatalf("unexpected mod hit: %+v", mod)
	}
	if mod.Source != content.SourceModrinth {
		t.Fatalf("expected SourceModrinth, got %s", mod.Source)
	}
}

func TestModrinthClient_SearchMods_SortAndCategory(t *testing.T) {
	var capturedIndex string
	var capturedFacets string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIndex = r.URL.Query().Get("index")
		capturedFacets = r.URL.Query().Get("facets")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits":       []map[string]any{},
			"total_hits": 0,
			"offset":     0,
			"limit":      20,
		})
	}))
	defer server.Close()

	client := modrinth.NewClient(server.URL, server.Client())

	_, _, err := client.SearchMods(context.Background(), "query", "1.21.1", "fabric", 20, 0, "newest", "technology")
	if err != nil {
		t.Fatalf("search mods failed: %v", err)
	}

	if capturedIndex != "newest" {
		t.Fatalf("expected index 'newest', got '%s'", capturedIndex)
	}
	if capturedFacets == "" || !strings.Contains(capturedFacets, "categories:technology") {
		t.Fatalf("expected facets to contain 'categories:technology', got '%s'", capturedFacets)
	}
}

func TestModrinthClient_GetProjectVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/project/sodium/version" {
			http.NotFound(w, r)
			return
		}

		pid := "P7dR8mSH"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":             "ver-123",
				"project_id":     "AANobbMI",
				"version_number": "0.5.8",
				"version_type":   "release",
				"name":           "Sodium 0.5.8",
				"changelog":      "## Sodium 0.5.8\n- Fixed performance issue",
				"game_versions":  []string{"1.21.1"},
				"loaders":        []string{"fabric"},
				"date_published": time.Now().Format(time.RFC3339),
				"files": []map[string]any{
					{
						"filename": "sodium-fabric-0.5.8.jar",
						"url":      "https://cdn.modrinth.com/sodium-fabric-0.5.8.jar",
						"primary":  true,
						"size":     1024000,
						"hashes": map[string]string{
							"sha1":   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
							"sha512": "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
						},
					},
				},
				"dependencies": []map[string]any{
					{
						"project_id":      &pid, // fabric-api
						"dependency_type": "required",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := modrinth.NewClient(server.URL, server.Client())
	versions, err := client.GetProjectVersions(context.Background(), "sodium", "1.21.1", "fabric")
	if err != nil {
		t.Fatalf("get project versions failed: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	v := versions[0]
	if v.VersionNum != "0.5.8" {
		t.Fatalf("unexpected version number: %s", v.VersionNum)
	}
	if v.VersionType != "release" {
		t.Errorf("expected VersionType release, got %s", v.VersionType)
	}
	if v.Changelog != "## Sodium 0.5.8\n- Fixed performance issue" {
		t.Errorf("expected changelog populated, got: %s", v.Changelog)
	}
	if len(v.Files) != 1 || v.Files[0].ReleaseType != "release" {
		t.Errorf("expected file ReleaseType release, got %+v", v.Files)
	}
	if len(v.Files) != 1 || v.Files[0].FileName != "sodium-fabric-0.5.8.jar" {
		t.Fatalf("unexpected files: %+v", v.Files)
	}
	if len(v.Files[0].Dependencies) != 1 || v.Files[0].Dependencies[0].ProjectID != "P7dR8mSH" {
		t.Fatalf("expected file-level dependencies mapped to files[0], got: %+v", v.Files[0].Dependencies)
	}
	if len(v.Dependencies) != 1 || v.Dependencies[0].ProjectID != "P7dR8mSH" {
		t.Fatalf("unexpected dependencies: %+v", v.Dependencies)
	}
}

func TestModrinthClient_GetProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/project/sodium" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "AANobbMI",
			"slug":          "sodium",
			"title":         "Sodium",
			"description":   "Modern rendering engine",
			"body":          "# Full markdown description here",
			"icon_url":      "https://cdn.modrinth.com/sodium.png",
			"downloads":     15000000,
			"follows":       45000,
			"categories":    []string{"fabric"},
			"loaders":       []string{"fabric"},
			"game_versions": []string{"1.21.1"},
			"updated":       time.Now().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	client := modrinth.NewClient(server.URL, server.Client())
	item, err := client.GetProject(context.Background(), "sodium")
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}

	if item.ID != "AANobbMI" || item.Name != "Sodium" || item.Description != "# Full markdown description here" {
		t.Fatalf("unexpected project: %+v", item)
	}

	// 404 test
	_, err = client.GetProject(context.Background(), "unknown-mod")
	if err == nil {
		t.Fatalf("expected error on unknown mod 404, got nil")
	}
}