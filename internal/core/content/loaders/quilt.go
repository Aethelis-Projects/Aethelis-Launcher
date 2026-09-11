package loaders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

const DefaultQuiltMetaURL = "https://meta.quiltmc.org"

type QuiltClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewQuiltClient(baseURL string, client *http.Client) *QuiltClient {
	if baseURL == "" {
		baseURL = DefaultQuiltMetaURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &QuiltClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

type quiltLoaderEntry struct {
	Loader struct {
		Version string `json:"version"`
		Maven   string `json:"maven"`
	} `json:"loader"`
}

func (c *QuiltClient) GetLoadersForGameVersion(ctx context.Context, gameVersion string) ([]content.LoaderVersion, error) {
	url := fmt.Sprintf("%s/v3/versions/loader/%s", c.baseURL, gameVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch quilt loaders: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quilt meta returned HTTP %d", resp.StatusCode)
	}

	var raw []quiltLoaderEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	res := make([]content.LoaderVersion, 0, len(raw))
	for _, entry := range raw {
		res = append(res, content.LoaderVersion{
			Version: entry.Loader.Version,
			Stable:  true,
			Maven:   entry.Loader.Maven,
		})
	}

	return res, nil
}