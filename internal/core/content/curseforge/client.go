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
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

const (
	DefaultBaseURL = "https://api.curseforge.com"
	MinecraftGameID = 432
)

// BuiltinAPIKey holds an optional CurseForge API key. In official releases, CurseForge uses BYOK via CURSEFORGE_API_KEY (or -X main.CurseForgeKey in custom builds).
var BuiltinAPIKey = ""

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
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
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}
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

func (c *Client) SearchMods(
	ctx context.Context,
	query string,
	gameVersion string,
	loader string,
	pageSize, index int,
) ([]content.ModItem, int64, error) {
	if c.apiKey == "" {
		return nil, 0, errors.New("curseforge: API key is not configured (set CURSEFORGE_API_KEY environment variable)")
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

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("curseforge search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("curseforge search HTTP %d: %s", resp.StatusCode, string(body))
	}

	var searchRes cfSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchRes); err != nil {
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
	if c.apiKey == "" {
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
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
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