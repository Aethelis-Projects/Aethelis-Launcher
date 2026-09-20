package curseforge_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/clock"
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
					"releaseType": 1,
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
	if f.ReleaseType != content.ReleaseTypeRelease {
		t.Errorf("expected ReleaseType %s, got %s", content.ReleaseTypeRelease, f.ReleaseType)
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

func TestCurseForgeClient_NoAPIKey_FastPath(t *testing.T) {
	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "", server.Client())

	_, _, err := client.SearchMods(context.Background(), "jei", "1.21.1", "fabric", 20, 0)
	if err == nil {
		t.Fatalf("expected error without API key, got nil")
	}
	if !strings.Contains(err.Error(), "API key is not configured") {
		t.Fatalf("expected 'API key is not configured' error, got %v", err)
	}
	if reached {
		t.Fatalf("expected fast-path to return before issuing HTTP request")
	}

	_, err = client.GetModFiles(context.Background(), 238222, "1.21.1", "fabric")
	if err == nil {
		t.Fatalf("expected error without API key for GetModFiles, got nil")
	}
	if !strings.Contains(err.Error(), "API key is not configured") {
		t.Fatalf("expected 'API key is not configured' error, got %v", err)
	}
	if reached {
		t.Fatalf("expected fast-path to return before issuing HTTP request")
	}
}

func TestCurseForgeClient_SetAPIKey_ThreadSafe(t *testing.T) {
	client := curseforge.NewClient("http://127.0.0.1:0", "", nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			client.SetAPIKey(fmt.Sprintf("key-%d", idx))
		}(i)
		go func() {
			defer wg.Done()
			_ = client.APIKey()
		}()
	}
	wg.Wait()
}

func TestCurseForgeClient_SearchMods_CacheAndTTL(t *testing.T) {
	hitCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":            12345,
					"slug":          "cached-mod",
					"name":          "Cached Mod",
					"summary":       "Summary",
					"downloadCount": 1000.0,
					"authors":       []map[string]string{{"name": "author1"}},
					"categories":    []map[string]string{{"name": "Utility"}},
					"dateModified":  time.Now().Format(time.RFC3339),
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

	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	mockClock := clock.NewMockClock(now)

	client := curseforge.NewClient(server.URL, "test-key", server.Client())
	client.SetClock(mockClock)

	// First call: cache miss, hits server
	mods1, total1, err := client.SearchMods(context.Background(), "cached", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hitCount != 1 || total1 != 1 || len(mods1) != 1 {
		t.Fatalf("expected 1 hit, got %d hits", hitCount)
	}

	// Mutate returned slice to verify cache isolation (D3)
	mods1[0].Name = "Mutated In Place"

	// Advance clock by 5 minutes (within 10-minute TTL)
	mockClock.CurrentTime = mockClock.CurrentTime.Add(5 * time.Minute)

	// Second call: cache hit, no server hit
	mods2, total2, err := client.SearchMods(context.Background(), "cached", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hitCount != 1 {
		t.Fatalf("expected cache hit (still 1 server hit), got %d hits", hitCount)
	}
	if total2 != 1 || len(mods2) != 1 {
		t.Fatalf("expected 1 mod item from cache")
	}
	// Verify D3: mutation of mods1 did not mutate cache
	if mods2[0].Name != "Cached Mod" {
		t.Errorf("expected original Cached Mod name, got %s (shared mutation detected!)", mods2[0].Name)
	}

	// Advance clock past 10 minutes (TTL expired)
	mockClock.CurrentTime = mockClock.CurrentTime.Add(6 * time.Minute) // total 11 min

	// Third call: cache expired, hits server again
	mods3, _, err := client.SearchMods(context.Background(), "cached", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hitCount != 2 {
		t.Fatalf("expected cache expiration (2 server hits), got %d hits", hitCount)
	}
	if len(mods3) != 1 {
		t.Fatalf("expected 1 mod item after refresh")
	}
}

func TestCurseForgeClient_RateLimited_401_403(t *testing.T) {
	statusCode := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Limit exceeded or unauthorized", statusCode)
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "test-key", server.Client())

	// Test 401
	_, _, err := client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0)
	if err == nil {
		t.Fatalf("expected error on HTTP 401, got nil")
	}
	if !strings.HasPrefix(err.Error(), "CF_RATE_LIMITED:") {
		t.Errorf("expected CF_RATE_LIMITED: prefix, got %v", err)
	}
	if !errors.Is(err, curseforge.ErrCurseForgeRateLimited) {
		t.Errorf("expected errors.Is ErrCurseForgeRateLimited, got %v", err)
	}

	// Test 403
	statusCode = http.StatusForbidden
	_, _, err = client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0)
	if err == nil {
		t.Fatalf("expected error on HTTP 403, got nil")
	}
	if !strings.HasPrefix(err.Error(), "CF_RATE_LIMITED:") {
		t.Errorf("expected CF_RATE_LIMITED: prefix, got %v", err)
	}
	if !errors.Is(err, curseforge.ErrCurseForgeRateLimited) {
		t.Errorf("expected errors.Is ErrCurseForgeRateLimited, got %v", err)
	}
}