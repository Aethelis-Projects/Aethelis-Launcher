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

func TestCurseForgeClient_SearchMods_SortAndCategory(t *testing.T) {
	var capturedSortField string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSortField = r.URL.Query().Get("sortField")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":            101,
					"slug":          "opti",
					"name":          "Opti",
					"summary":       "Optimization mod",
					"downloadCount": 500.0,
					"authors":       []map[string]string{{"name": "author1"}},
					"categories":    []map[string]string{{"name": "Optimization"}},
					"dateModified":  time.Now().Format(time.RFC3339),
				},
				{
					"id":            102,
					"slug":          "magic",
					"name":          "Magic Mod",
					"summary":       "Magic items",
					"downloadCount": 300.0,
					"authors":       []map[string]string{{"name": "author2"}},
					"categories":    []map[string]string{{"name": "Magic"}},
					"dateModified":  time.Now().Format(time.RFC3339),
				},
			},
			"pagination": map[string]any{
				"index":      0,
				"pageSize":   20,
				"totalCount": 2,
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "test-api-key", server.Client())

	// Test sort mapping and category filtering
	mods, total, err := client.SearchMods(context.Background(), "", "1.21.1", "fabric", 20, 0, "popularity", "magic")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if capturedSortField != "2" {
		t.Fatalf("expected sortField 2 for popularity, got %s", capturedSortField)
	}
	if len(mods) != 1 || mods[0].Slug != "magic" {
		t.Fatalf("expected only magic mod to be returned after category filter, got %+v", mods)
	}
	if total != 2 {
		t.Fatalf("expected total 2 from API, got %d", total)
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

	// Test 401: Fast fail, key invalid
	_, _, err := client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0)
	if err == nil {
		t.Fatalf("expected error on HTTP 401, got nil")
	}
	if !strings.HasPrefix(err.Error(), "CF_KEY_INVALID:") {
		t.Errorf("expected CF_KEY_INVALID: prefix, got %v", err)
	}
	if !errors.Is(err, curseforge.ErrCurseForgeKeyInvalid) {
		t.Errorf("expected errors.Is ErrCurseForgeKeyInvalid, got %v", err)
	}

	// Test 403: Rate limited with wait duration
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
	var rle *curseforge.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected RateLimitError, got %T", err)
	}
	if rle.StatusCode != http.StatusForbidden {
		t.Errorf("expected status code 403, got %d", rle.StatusCode)
	}
	if rle.RetryAfterSeconds <= 0 {
		t.Errorf("expected positive retry after seconds, got %d", rle.RetryAfterSeconds)
	}
}

func TestCurseForgeClient_Headers_UserAgentAndAccept(t *testing.T) {
	var capturedUA, capturedAccept, capturedAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedAccept = r.Header.Get("Accept")
		capturedAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{},
			"pagination": map[string]any{
				"totalCount": 0,
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "my-secret-key", server.Client())
	client.SetVersion("0.5.0")

	_, _, err := client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("SearchMods failed: %v", err)
	}

	expectedUA := "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"
	if capturedUA != expectedUA {
		t.Errorf("captured User-Agent = %q; want %q", capturedUA, expectedUA)
	}
	if capturedAccept != "application/json" {
		t.Errorf("captured Accept = %q; want %q", capturedAccept, "application/json")
	}
	if capturedAPIKey != "my-secret-key" {
		t.Errorf("captured x-api-key = %q; want %q", capturedAPIKey, "my-secret-key")
	}
}

func TestCurseForgeClient_RateLimit_InlineRetryUnder5s(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":   1,
					"slug": "mod1",
					"name": "Mod 1",
				},
			},
			"pagination": map[string]any{
				"totalCount": 1,
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "key", server.Client())
	mods, total, err := client.SearchMods(context.Background(), "query", "1.21.1", "fabric", 20, 0)
	if err != nil {
		t.Fatalf("expected successful retry, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected exactly 2 attempts, got %d", attempts)
	}
	if total != 1 || len(mods) != 1 {
		t.Errorf("expected 1 item, got total=%d len=%d", total, len(mods))
	}
}

func TestCurseForgeClient_RateLimit_FastFailOver5s(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "10")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "key", server.Client())
	_, _, err := client.SearchMods(context.Background(), "query", "1.21.1", "fabric", 20, 0)
	if err == nil {
		t.Fatalf("expected rate limit error, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt (no inline sleep on >5s), got %d", attempts)
	}
	var rle *curseforge.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected RateLimitError, got %T", err)
	}
	if rle.RetryAfterSeconds != 10 {
		t.Errorf("expected RetryAfterSeconds=10, got %d", rle.RetryAfterSeconds)
	}
}

func TestCurseForgeClient_SortRelevance_OmitsSortField(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{},
			"pagination": map[string]any{
				"totalCount": 0,
			},
		})
	}))
	defer server.Close()

	client := curseforge.NewClient(server.URL, "key", server.Client())

	// sort = ""
	_, _, err := client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0, "")
	if err != nil {
		t.Fatalf("SearchMods failed: %v", err)
	}
	if strings.Contains(capturedQuery, "sortField") {
		t.Errorf("expected sortField to be omitted when sort='', got: %s", capturedQuery)
	}

	// sort = "relevance"
	_, _, err = client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0, "relevance")
	if err != nil {
		t.Fatalf("SearchMods failed: %v", err)
	}
	if strings.Contains(capturedQuery, "sortField") {
		t.Errorf("expected sortField to be omitted when sort='relevance', got: %s", capturedQuery)
	}

	// sort = "invalid_sort" -> should return error (B5)
	_, _, err = client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0, "nonexistent_sort")
	if err == nil {
		t.Errorf("expected error for invalid sort, got nil")
	}
}