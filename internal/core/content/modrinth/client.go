package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

const (
	DefaultBaseURL = "https://api.modrinth.com"
	DefaultUserAgent = "Nord-Launcher/0.1.0 (contact@nordlauncher.io)"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		userAgent:  DefaultUserAgent,
	}
}

type searchResponse struct {
	Hits      []searchHit `json:"hits"`
	Offset    int         `json:"offset"`
	Limit     int         `json:"limit"`
	TotalHits int         `json:"total_hits"`
}

type searchHit struct {
	ProjectID   string    `json:"project_id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	IconURL     string    `json:"icon_url"`
	Downloads   int64     `json:"downloads"`
	Follows     int64     `json:"follows"`
	Categories  []string  `json:"categories"`
	GameVersions []string `json:"versions"`
	DateModified time.Time `json:"date_modified"`
}

// SearchMods queries Modrinth for mods matching query, game version, loader, sort, and category.
func (c *Client) SearchMods(
	ctx context.Context,
	query string,
	gameVersion string,
	loader string,
	limit, offset int,
	opts ...string,
) ([]content.ModItem, int, error) {
	u, err := url.Parse(c.baseURL + "/v2/search")
	if err != nil {
		return nil, 0, err
	}

	var sort, category string
	if len(opts) > 0 {
		sort = opts[0]
	}
	if len(opts) > 1 {
		category = opts[1]
	}

	q := u.Query()
	if query != "" {
		q.Set("query", query)
	}

	var facets [][]string
	facets = append(facets, []string{"project_type:mod"})
	if loader != "" {
		facets = append(facets, []string{"categories:" + loader})
	}
	if gameVersion != "" {
		facets = append(facets, []string{"versions:" + gameVersion})
	}
	if category != "" {
		facets = append(facets, []string{"categories:" + category})
	}

	facetsJSON, err := json.Marshal(facets)
	if err == nil {
		q.Set("facets", string(facetsJSON))
	}

	// Map sort to Modrinth index ("relevance", "downloads", "follows", "newest", "updated")
	switch strings.ToLower(sort) {
	case "relevance":
		q.Set("index", "relevance")
	case "downloads":
		q.Set("index", "downloads")
	case "follows":
		q.Set("index", "follows")
	case "newest":
		q.Set("index", "newest")
	case "updated":
		q.Set("index", "updated")
	default:
		if sort != "" {
			q.Set("index", sort)
		} else {
			q.Set("index", "downloads")
		}
	}

	if limit <= 0 {
		limit = 20
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("modrinth search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("modrinth search returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var searchRes searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchRes); err != nil {
		return nil, 0, fmt.Errorf("decode modrinth search response: %w", err)
	}

	items := make([]content.ModItem, 0, len(searchRes.Hits))
	for _, hit := range searchRes.Hits {
		items = append(items, content.ModItem{
			ID:         hit.ProjectID,
			Slug:       hit.Slug,
			Source:     content.SourceModrinth,
			Name:       hit.Title,
			Author:     hit.Author,
			Summary:    hit.Description,
			IconURL:    hit.IconURL,
			Downloads:  hit.Downloads,
			Follows:    hit.Follows,
			Categories: hit.Categories,
			GameVers:   hit.GameVersions,
			UpdatedAt:  hit.DateModified,
		})
	}

	return items, searchRes.TotalHits, nil
}

type projectResponse struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Body        string    `json:"body"`
	IconURL     string    `json:"icon_url"`
	Downloads   int64     `json:"downloads"`
	Follows     int64     `json:"follows"`
	Categories  []string  `json:"categories"`
	Loaders     []string  `json:"loaders"`
	GameVersions []string `json:"game_versions"`
	Updated     time.Time `json:"updated"`
}

// GetProject fetches full project details for a slug or ID.
func (c *Client) GetProject(ctx context.Context, idOrSlug string) (*content.ModItem, error) {
	reqURL := fmt.Sprintf("%s/v2/project/%s", c.baseURL, url.PathEscape(idOrSlug))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch modrinth project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("modrinth project HTTP %d", resp.StatusCode)
	}

	var p projectResponse
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}

	return &content.ModItem{
		ID:          p.ID,
		Slug:        p.Slug,
		Source:      content.SourceModrinth,
		Name:        p.Title,
		Summary:     p.Description,
		Description: p.Body,
		IconURL:     p.IconURL,
		Downloads:   p.Downloads,
		Follows:     p.Follows,
		Categories:  p.Categories,
		Loaders:     p.Loaders,
		GameVers:    p.GameVersions,
		UpdatedAt:   p.Updated,
	}, nil
}

type versionResponse struct {
	ID            string         `json:"id"`
	ProjectID     string         `json:"project_id"`
	VersionNum    string         `json:"version_number"`
	Name          string         `json:"name"`
	VersionType   string         `json:"version_type"` // "release", "beta", "alpha"
	GameVersions  []string       `json:"game_versions"`
	Loaders       []string       `json:"loaders"`
	DatePublished time.Time     `json:"date_published"`
	Files         []fileResponse `json:"files"`
	Dependencies  []depResponse  `json:"dependencies"`
}

type fileResponse struct {
	Hashes struct {
		SHA1   string `json:"sha1"`
		SHA512 string `json:"sha512"`
	} `json:"hashes"`
	URL      string `json:"url"`
	FileName string `json:"filename"`
	Primary  bool   `json:"primary"`
	Size     int64  `json:"size"`
}

type depResponse struct {
	VersionID      *string `json:"version_id"`
	ProjectID      *string `json:"project_id"`
	DependencyType string  `json:"dependency_type"` // "required", "optional", "incompatible", "embedded"
	FileName       *string `json:"file_name"`
}

// GetProjectVersions fetches released versions for a project, filtered by game version and loader.
func (c *Client) GetProjectVersions(
	ctx context.Context,
	idOrSlug string,
	gameVersion, loader string,
) ([]content.ModVersion, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v2/project/%s/version", c.baseURL, url.PathEscape(idOrSlug)))
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if gameVersion != "" {
		gvJSON, _ := json.Marshal([]string{gameVersion})
		q.Set("game_versions", string(gvJSON))
	}
	if loader != "" {
		lJSON, _ := json.Marshal([]string{loader})
		q.Set("loaders", string(lJSON))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch modrinth versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("modrinth versions HTTP %d", resp.StatusCode)
	}

	var rawVersions []versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawVersions); err != nil {
		return nil, err
	}

	versions := make([]content.ModVersion, 0, len(rawVersions))
	for _, rv := range rawVersions {
		files := make([]content.ModFile, 0, len(rv.Files))
		for _, rf := range rv.Files {
			files = append(files, content.ModFile{
				ID:           rf.FileName,
				VersionID:    rv.ID,
				FileName:     rf.FileName,
				URL:          rf.URL,
				Size:         rf.Size,
				SHA1:         rf.Hashes.SHA1,
				SHA512:       rf.Hashes.SHA512,
				Primary:      rf.Primary,
				ReleaseType:  rv.VersionType,
				FileDate:     rv.DatePublished,
				GameVersions: rv.GameVersions,
				Loaders:      rv.Loaders,
			})
		}

		deps := make([]content.ModDependency, 0, len(rv.Dependencies))
		for _, rd := range rv.Dependencies {
			var pid, vid, fn string
			if rd.ProjectID != nil {
				pid = *rd.ProjectID
			}
			if rd.VersionID != nil {
				vid = *rd.VersionID
			}
			if rd.FileName != nil {
				fn = *rd.FileName
			}
			deps = append(deps, content.ModDependency{
				ProjectID: pid,
				VersionID: vid,
				Type:      content.DependencyType(rd.DependencyType),
				FileName:  fn,
			})
		}

		versions = append(versions, content.ModVersion{
			ID:           rv.ID,
			ProjectID:    rv.ProjectID,
			VersionNum:   rv.VersionNum,
			Name:         rv.Name,
			VersionType:  rv.VersionType,
			GameVersions: rv.GameVersions,
			Loaders:      rv.Loaders,
			Files:        files,
			Dependencies: deps,
			ReleaseDate:  rv.DatePublished,
		})
	}

	return versions, nil
}