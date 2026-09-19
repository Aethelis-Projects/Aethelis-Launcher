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
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

const (
	DefaultBaseURL  = "https://api.curseforge.com"
	MinecraftGameID = 432
)

// ErrCurseForgeRateLimited indicates CurseForge returned HTTP 401 or 403 (quota or auth issue).
var ErrCurseForgeRateLimited = errors.New("curseforge rate limit or authentication error")

// BuiltinAPIKey holds an optional CurseForge API key. In official releases, CurseForge uses BYOK via CURSEFORGE_API_KEY (or -X main.CurseForgeKey in custom builds).
var BuiltinAPIKey = ""

type searchCacheEntry struct {
	rawJSON    []byte
	totalCount int64
	expiresAt  time.Time
}

type Client struct {
	baseURL     string
	apiKey      string
	httpClient  *http.Client
	clock       ports.Clock
	searchCache map[string]searchCacheEntry
	cacheKeys   []string
	mu          sync.RWMutex
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if apiKey == "" {
		apiKey = BuiltinAPIKey
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		httpClient:  httpClient,
		clock:       clock.NewRealClock(),
		searchCache: make(map[string]searchCacheEntry),
	}
}

func (c *Client) SetClock(clk ports.Clock) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock = clk
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

func (c *Client) SearchMods(
	ctx context.Context,
	query string,
	gameVersion string,
	loader string,
	pageSize, index int,
) ([]content.ModItem, int64, error) {
	apiKey := c.APIKey()
	if apiKey == "" {
		return nil, 0, errors.New("curseforge: API key is not configured (set CURSEFORGE_API_KEY environment variable)")
	}

	cacheKey := fmt.Sprintf("%s|%s|%s|%d|%d", query, gameVersion, loader, pageSize, index)
	now := c.now()

	c.mu.RLock()
	if entry, found := c.searchCache[cacheKey]; found {
		if now.Before(entry.expiresAt) {
			c.mu.RUnlock()
			return decodeSearchItems(entry.rawJSON)
		}
	}
	c.mu.RUnlock()

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

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("curseforge search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, 0, fmt.Errorf("CF_RATE_LIMITED: %w (HTTP %d)", ErrCurseForgeRateLimited, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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
	c.mu.Unlock()

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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("curseforge files request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("curseforge files HTTP %d", resp.StatusCode)
	}

	var filesRes cfFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&filesRes); err != nil {
		return nil, err
	}

	files := make([]content.ModFile, 0, len(filesRes.Data))
	for _, f := range filesRes.Data {
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

		files = append(files, content.ModFile{
			ID:           strconv.FormatInt(f.ID, 10),
			VersionID:    strconv.FormatInt(f.ID, 10),
			FileName:     f.FileName,
			URL:          f.DownloadURL,
			Size:         f.FileLength,
			SHA1:         sha1Val,
			Dependencies: deps,
			Primary:      true,
		})
	}

	return files, nil
}