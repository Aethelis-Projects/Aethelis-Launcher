package curseforge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
)

func TestCurseForgeClient_SearchMods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mods/search" {
			http.NotFound(w, r)
			return
		}

		apiKey := r.Header.Get("x-api-key")
		if apiKey != "test-api-key" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":            238222,
					"slug":          "jei",
					"name":          "Just Enough Items (JEI)",
					"summary":       "Item and Recipe viewing mod",
					"downloadCount": 120000000.0,
					"authors": []map[string]string{
						{"name": "mezz"},
					},
					"categories": []map[string]string{
						{"name": "Utility"},
					},
					"dateModified": time.Now().Format(time.RFC3339),
				},
			},
			"pagination": map[string]any{
				"index":      0,
				"pageSize":   20,
				"totalCount": 1,
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "test-api-key", server.Client())

	mods, total, err := client.SearchMods(context.Background(), "jei", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("curseforge search failed: %v", err)
	}

	if total != 1 || len(mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(mods))
	}

	mod := mods[0]
	if mod.ID != "238222" || mod.Slug != "jei" || mod.Author != "mezz" {
		t.Fatalf("unexpected mod metadata: %+v", mod)
	}
	if mod.Source != content.SourceCurseForge {
		t.Fatalf("expected SourceCurseForge, got %s", mod.Source)
	}
}

func TestCurseForgeClient_GetModFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mods/238222/files" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":          555000,
					"modId":       238222,
					"fileName":    "jei-1.21.1-fabric-19.1.0.jar",
					"downloadUrl": "https://edge.forgecdn.net/files/5550/0/jei.jar",
					"fileLength":  2500000,
					"hashes": []map[string]any{
						{"value": "11223344556677889900aabbccddeeff11223344", "algo": 1},
					},
					"dependencies": []map[string]any{
						{"modId": 306612, "relationType": 3}, // Required dependency (Fabric API)
					},
				},
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "test-api-key", server.Client())
	files, err := client.GetModFiles(context.Background(), 238222, "1.21.1", "fabric")
	if err != nil {
		t.Fatalf("get mod files failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.FileName != "jei-1.21.1-fabric-19.1.0.jar" || f.SHA1 != "11223344556677889900aabbccddeeff11223344" {
		t.Fatalf("unexpected file details: %+v", f)
	}
	if len(f.Dependencies) != 1 || f.Dependencies[0].ProjectID != "306612" || f.Dependencies[0].Type != content.DepRequired {
		t.Fatalf("unexpected dependency details: %+v", f.Dependencies)
	}
}

func TestCurseForge_LoaderToType(t *testing.T) {
	cases := map[string]int{
		"forge":    1,
		"fabric":   4,
		"quilt":    5,
		"neoforge": 6,
		"other":    0,
	}
	for loader, expected := range cases {
		got := curseforge.LoaderToType(loader)
		if got != expected {
			t.Errorf("LoaderToType(%q) = %d, want %d", loader, got, expected)
		}
	}
}

func TestCurseForge_ClientDefaults(t *testing.T) {
	c := curseforge.NewClient("", "", nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}