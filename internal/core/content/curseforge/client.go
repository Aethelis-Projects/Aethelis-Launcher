package curseforge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/netutil"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

const (
	DefaultBaseURL  = "https://api.curseforge.com"
	MinecraftGameID = 432
)

// ErrCurseForgeRateLimited indicates CurseForge returned HTTP 429 or 403 (quota or rate-limit issue).
var ErrCurseForgeRateLimited = errors.New("curseforge rate limit or authentication error")

// ErrCurseForgeKeyInvalid indicates the CurseForge API key was rejected by the server (HTTP 401).
var ErrCurseForgeKeyInvalid = errors.New("curseforge API key is invalid or rejected")

// RateLimitError provides structured details when CurseForge returns rate limits.
type RateLimitError struct {
	StatusCode        int
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("CF_RATE_LIMITED: %d seconds wait (HTTP %d)", e.RetryAfterSeconds, e.StatusCode)
}

func (e *RateLimitError) Unwrap() error {
	return ErrCurseForgeRateLimited
}

// BuiltinAPIKey holds an optional CurseForge API key (resolved from sidecar cf.key or env).
var (
	builtinMu     sync.RWMutex
	BuiltinAPIKey = ""
)

// SetBuiltinAPIKey sets the package-level built-in API key.
func SetBuiltinAPIKey(key string) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	BuiltinAPIKey = key
}

// GetBuiltinAPIKey returns the current package-level built-in API key.
func GetBuiltinAPIKey() string {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	return BuiltinAPIKey
}

// HasBuiltinKey reports whether a non-empty built-in API key is configured.
func HasBuiltinKey() bool {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	return BuiltinAPIKey != ""
}

type searchCacheEntry struct {
	rawJSON    []byte
	totalCount int64
	expiresAt  time.Time
}

// tokenBucket implements a pure-Go clock-aware rate limiter (1.0 rps, 5 burst).
type tokenBucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
	clock      ports.Clock
}

func newTokenBucket(rate, capacity float64, clk ports.Clock) *tokenBucket {
	now := time.Now()
	if clk != nil {
		now = clk.Now()
	}
	return &tokenBucket{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity,
		lastRefill: now,
		clock:      clk,
	}
}

func (tb *tokenBucket) now() time.Time {
	if tb.clock != nil {
		return tb.clock.Now()
	}
	return time.Now()
}

func (tb *tokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		now := tb.now()
		elapsed := now.Sub(tb.lastRefill).Seconds()
		if elapsed > 0 {
			tb.tokens += elapsed * tb.rate
			if tb.tokens > tb.capacity {
				tb.tokens = tb.capacity
			}
			tb.lastRefill = now
		}

		if tb.tokens >= 1.0 {
			tb.tokens -= 1.0
			tb.mu.Unlock()
			return nil
		}

		needed := 1.0 - tb.tokens
		waitSec := needed / tb.rate
		tb.mu.Unlock()

		waitDur := time.Duration(waitSec * float64(time.Second))
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
		}
	}
}

func parseRetryAfter(val string, now time.Time) (time.Duration, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(val); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	formats := []string{
		http.TimeFormat,
		time.RFC850,
		time.ANSIC,
	}
	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, val); err == nil {
			d := t.Sub(now)
			if d < 0 {
				d = 0
			}
			return d, true
		}
	}
	return 0, false
}

type Client struct {
	baseURL      string
	apiKey       string
	version      string
	httpClient   *http.Client
	clock        ports.Clock
	limiter      *tokenBucket
	contentCache ports.ContentCache
	searchCache  map[string]searchCacheEntry
	cacheKeys    []string
	mu           sync.RWMutex
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if apiKey == "" {
		apiKey = GetBuiltinAPIKey()
	}
	if httpClient == nil {
		httpClient = netutil.NewHTTPClient("0.5.0", 30*time.Second)
	}
	clk := clock.NewRealClock()
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		version:     "0.5.0",
		httpClient:  httpClient,
		clock:       clk,
		limiter:     newTokenBucket(1.0, 5.0, clk),
		searchCache: make(map[string]searchCacheEntry),
	}
}

func (c *Client) SetVersion(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version = v
}

func (c *Client) SetContentCache(cache ports.ContentCache) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contentCache = cache
}

func (c *Client) SetClock(clk ports.Clock) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock = clk
	if c.limiter != nil {
		c.limiter.mu.Lock()
		c.limiter.clock = clk
		c.limiter.mu.Unlock()
	}
}

func (c *Client) now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.clock != nil {
		return c.clock.Now()
	}
	return time.Now()
}

func (c *Client) SetAPIKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = key
}

func (c *Client) APIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiKey
}

// LoaderToTypeMap maps string loader names to CurseForge numeric enum.
func LoaderToType(loader string) int {
	switch loader {
	case "forge":
		return 1
	case "fabric":
		return 4
	case "quilt":
		return 5
	case "neoforge":
		return 6
	default:
		return 0
	}
}

type cfSearchResponse struct {
	Data       []cfMod `json:"data"`
	Pagination struct {
		Index       int   `json:"index"`
		PageSize    int   `json:"pageSize"`
		TotalCount  int64 `json:"totalCount"`
	} `json:"pagination"`
}

type cfMod struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Summary     string    `json:"summary"`
	DownloadCount float64 `json:"downloadCount"`
	Logo        *struct {
		URL string `json:"url"`
	} `json:"logo"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Categories []struct {
		Name string `json:"name"`
	} `json:"categories"`
	DateModified time.Time `json:"dateModified"`
}

func decodeSearchItems(rawJSON []byte) ([]content.ModItem, int64, error) {
	var searchRes cfSearchResponse
	if err := json.Unmarshal(rawJSON, &searchRes); err != nil {
		return nil, 0, fmt.Errorf("decode curseforge response: %w", err)
	}

	items := make([]content.ModItem, 0, len(searchRes.Data))
	for _, m := range searchRes.Data {
		author := ""
		if len(m.Authors) > 0 {
			author = m.Authors[0].Name
		}
		iconURL := ""
		if m.Logo != nil {
			iconURL = m.Logo.URL
		}
		cats := make([]string, 0, len(m.Categories))
		for _, cat := range m.Categories {
			cats = append(cats, cat.Name)
		}

		items = append(items, content.ModItem{
			ID:         strconv.FormatInt(m.ID, 10),
			Slug:       m.Slug,
			Source:     content.SourceCurseForge,
			Name:       m.Name,
			Author:     author,
			Summary:    m.Summary,
			IconURL:    iconURL,
			Downloads:  int64(m.DownloadCount),
			Categories: cats,
			UpdatedAt:  m.DateModified,
		})
	}

	return items, searchRes.Pagination.TotalCount, nil
}

func (c *Client) doRequest(ctx context.Context, u *url.URL) (*http.Response, error) {
	apiKey := c.APIKey()
	now := c.now()

	var lastResp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", netutil.FormatUserAgent(c.version))
		}
		if req.Header.Get("Accept") == "" {
			req.Header.Set("Accept", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		// 401 Unauthorized: Fast failure without retrying
		if resp.StatusCode == http.StatusUnauthorized {
			_ = resp.Body.Close() // errcheck:ok discard body on fast 401 error
			return nil, fmt.Errorf("CF_KEY_INVALID: %w (HTTP 401)", ErrCurseForgeKeyInvalid)
		}

		// 429 Too Many Requests or 403 Forbidden (Cloudflare rate limit)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
			retryDelay, hasRetryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), now)
			if !hasRetryAfter {
				retryDelay = time.Duration(10*(1<<attempt)) * time.Second
			}
			_ = resp.Body.Close() // errcheck:ok close body before retry or rate limit error

			// B4: Inline retry ONLY if Retry-After <= 5s and attempt < 2
			if retryDelay <= 5*time.Second && attempt < 2 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryDelay):
					continue
				}
			}

			waitSecs := int(retryDelay.Seconds())
			if waitSecs <= 0 {
				waitSecs = 1
			}
			return nil, &RateLimitError{
				StatusCode:        resp.StatusCode,
				RetryAfterSeconds: waitSecs,
			}
		}

		lastResp = resp
		break
	}

	if lastResp == nil {
		return nil, errors.New("curseforge: request failed with no response")
	}
	return lastResp, nil
}

func (c *Client) SearchMods(
	ctx context.Context,
	query string,
	gameVersion string,
	loader string,
	pageSize, index int,
	opts ...string,
) ([]content.ModItem, int64, error) {
	apiKey := c.APIKey()
	if apiKey == "" {
		return nil, 0, errors.New("curseforge: API key is not configured (set CURSEFORGE_API_KEY environment variable)")
	}

	var sort, category string
	if len(opts) > 0 {
		sort = opts[0]
	}
	if len(opts) > 1 {
		category = opts[1]
	}

	cacheKey := fmt.Sprintf("%s|%s|%s|%d|%d|%s|%s", query, gameVersion, loader, pageSize, index, sort, category)
	now := c.now()

	c.mu.RLock()
	if entry, found := c.searchCache[cacheKey]; found {
		if now.Before(entry.expiresAt) {
			c.mu.RUnlock()
			return decodeSearchItems(entry.rawJSON)
		}
	}
	diskCache := c.contentCache
	c.mu.RUnlock()

	// Check persistent L2 disk cache
	if diskCache != nil {
		if payload, expiresAt, ok, err := diskCache.Get(ctx, "search", cacheKey); err == nil && ok && now.Before(expiresAt) {
			items, total, err := decodeSearchItems([]byte(payload))
			if err == nil {
				c.mu.Lock()
				c.searchCache[cacheKey] = searchCacheEntry{
					rawJSON:    []byte(payload),
					totalCount: total,
					expiresAt:  now.Add(10 * time.Minute),
				}
				c.mu.Unlock()
				return items, total, nil
			}
		}
	}

	u, err := url.Parse(c.baseURL + "/v1/mods/search")
	if err != nil {
		return nil, 0, err
	}

	q := u.Query()
	q.Set("gameId", strconv.Itoa(MinecraftGameID))
	q.Set("classId", "6") // 6 = Mods
	if query != "" {
		q.Set("searchFilter", query)
	}
	if gameVersion != "" {
		q.Set("gameVersion", gameVersion)
	}
	if lType := LoaderToType(loader); lType > 0 {
		q.Set("modLoaderType", strconv.Itoa(lType))
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("index", strconv.Itoa(index))

	// Map sort to CurseForge sortField (1=Featured, 2=Popularity, 3=LastUpdated, 4=Name, 5=TotalDownloads)
	switch strings.ToLower(sort) {
	case "", "relevance":
		// Server default is relevance / featured; do not set sortField (B5)
	case "featured":
		q.Set("sortField", "1")
	case "popularity":
		q.Set("sortField", "2")
	case "updated":
		q.Set("sortField", "3")
	case "name":
		q.Set("sortField", "4")
	case "downloads":
		q.Set("sortField", "5")
	default:
		if n, err := strconv.Atoi(sort); err == nil && n >= 1 && n <= 5 {
			q.Set("sortField", sort)
		} else {
			return nil, 0, fmt.Errorf("curseforge: unsupported sort option %q", sort)
		}
	}

	u.RawQuery = q.Encode()

	resp, err := c.doRequest(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) // errcheck:ok read error response body
		return nil, 0, fmt.Errorf("curseforge search HTTP %d: %s", resp.StatusCode, string(body))
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read curseforge response: %w", err)
	}

	items, total, err := decodeSearchItems(rawBytes)
	if err != nil {
		return nil, 0, err
	}

	if category != "" {
		catLower := strings.ToLower(strings.TrimSpace(category))
		var filtered []content.ModItem
		for _, it := range items {
			matched := false
			for _, cat := range it.Categories {
				if strings.Contains(strings.ToLower(cat), catLower) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	c.mu.Lock()
	if len(c.searchCache) >= 64 && len(c.cacheKeys) > 0 {
		oldest := c.cacheKeys[0]
		c.cacheKeys = c.cacheKeys[1:]
		delete(c.searchCache, oldest)
	}
	c.searchCache[cacheKey] = searchCacheEntry{
		rawJSON:    rawBytes,
		totalCount: total,
		expiresAt:  now.Add(10 * time.Minute),
	}
	c.cacheKeys = append(c.cacheKeys, cacheKey)
	diskCache = c.contentCache
	c.mu.Unlock()

	// Persist in L2 disk cache (6 hour TTL for searches)
	if diskCache != nil {
		_ = diskCache.Set(ctx, "search", cacheKey, string(rawBytes), 6*time.Hour) // errcheck:ok best effort L2 cache write
	}

	return items, total, nil
}

type cfFilesResponse struct {
	Data []cfFile `json:"data"`
}

type cfFile struct {
	ID           int64         `json:"id"`
	ModID        int64         `json:"modId"`
	DisplayName  string        `json:"displayName"`
	FileName     string        `json:"fileName"`
	FileDate     time.Time     `json:"fileDate"`
	FileLength   int64         `json:"fileLength"`
	ReleaseType  int           `json:"releaseType"` // 1 = Release, 2 = Beta, 3 = Alpha
	DownloadURL  string        `json:"downloadUrl"`
	GameVersions []string      `json:"gameVersions"`
	Hashes       []cfHash      `json:"hashes"`
	Dependencies []cfFileDep   `json:"dependencies"`
}

type cfHash struct {
	Value string `json:"value"`
	Algo  int    `json:"algo"` // 1 = SHA-1, 2 = MD5
}

type cfFileDep struct {
	ModID        int64 `json:"modId"`
	RelationType int   `json:"relationType"` // 1=Embedded, 2=Optional, 3=Required, 4=Tool, 5=Incompatible, 6=Include
}

func parseModFiles(data []cfFile) []content.ModFile {
	files := make([]content.ModFile, 0, len(data))
	for _, f := range data {
		sha1Val := ""
		for _, h := range f.Hashes {
			if h.Algo == 1 { // SHA-1
				sha1Val = h.Value
				break
			}
		}

		deps := make([]content.ModDependency, 0, len(f.Dependencies))
		for _, d := range f.Dependencies {
			var depType content.DependencyType
			switch d.RelationType {
			case 3:
				depType = content.DepRequired
			case 2:
				depType = content.DepOptional
			case 5:
				depType = content.DepIncompatible
			case 1:
				depType = content.DepEmbedded
			default:
				depType = content.DepOptional
			}
			deps = append(deps, content.ModDependency{
				ProjectID: strconv.FormatInt(d.ModID, 10),
				Type:      depType,
			})
		}

		relType := content.ReleaseTypeRelease
		switch f.ReleaseType {
		case 2:
			relType = content.ReleaseTypeBeta
		case 3:
			relType = content.ReleaseTypeAlpha
		}

		files = append(files, content.ModFile{
			ID:           strconv.FormatInt(f.ID, 10),
			VersionID:    strconv.FormatInt(f.ID, 10),
			FileName:     f.FileName,
			URL:          f.DownloadURL,
			Size:         f.FileLength,
			SHA1:         sha1Val,
			Dependencies: deps,
			Primary:      true,
			ReleaseType:  relType,
			FileDate:     f.FileDate,
			GameVersions: f.GameVersions,
		})
	}
	return files
}

// GetModFiles retrieves release files for a specific CurseForge mod.
func (c *Client) GetModFiles(
	ctx context.Context,
	modID int64,
	gameVersion string,
	loader string,
) ([]content.ModFile, error) {
	apiKey := c.APIKey()
	if apiKey == "" {
		return nil, errors.New("curseforge: API key is not configured (set CURSEFORGE_API_KEY environment variable)")
	}

	filesCacheKey := fmt.Sprintf("%d|%s|%s", modID, gameVersion, loader)
	now := c.now()

	c.mu.RLock()
	diskCache := c.contentCache
	c.mu.RUnlock()

	// Check persistent L2 disk cache (24 hour TTL)
	if diskCache != nil {
		if payload, expiresAt, ok, err := diskCache.Get(ctx, "files", filesCacheKey); err == nil && ok && now.Before(expiresAt) {
			var filesRes cfFilesResponse
			if err := json.Unmarshal([]byte(payload), &filesRes); err == nil {
				return parseModFiles(filesRes.Data), nil
			}
		}
	}

	u, err := url.Parse(fmt.Sprintf("%s/v1/mods/%d/files", c.baseURL, modID))
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if gameVersion != "" {
		q.Set("gameVersion", gameVersion)
	}
	if lType := LoaderToType(loader); lType > 0 {
		q.Set("modLoaderType", strconv.Itoa(lType))
	}
	u.RawQuery = q.Encode()

	resp, err := c.doRequest(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) // errcheck:ok read error response body
		return nil, fmt.Errorf("curseforge files HTTP %d: %s", resp.StatusCode, string(body))
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read curseforge response: %w", err)
	}

	var filesRes cfFilesResponse
	if err := json.Unmarshal(rawBytes, &filesRes); err != nil {
		return nil, err
	}

	// Persist in L2 disk cache (24 hour TTL for mod files)
	if diskCache != nil {
		_ = diskCache.Set(ctx, "files", filesCacheKey, string(rawBytes), 24*time.Hour) // errcheck:ok best effort L2 cache write
	}

	return parseModFiles(filesRes.Data), nil
}