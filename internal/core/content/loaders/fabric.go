package loaders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

const DefaultFabricMetaURL = "https://meta.fabricmc.net"

type FabricClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewFabricClient(baseURL string, client *http.Client) *FabricClient {
	if baseURL == "" {
		baseURL = DefaultFabricMetaURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &FabricClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

type fabricLoaderEntry struct {
	Loader struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
		Maven   string `json:"maven"`
	} `json:"loader"`
}

// GetLoadersForGameVersion returns compatible Fabric loader versions for a Minecraft release.
func (c *FabricClient) GetLoadersForGameVersion(ctx context.Context, gameVersion string) ([]content.LoaderVersion, error) {
	url := fmt.Sprintf("%s/v2/versions/loader/%s", c.baseURL, gameVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch fabric loaders: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fabric meta returned HTTP %d", resp.StatusCode)
	}

	var raw []fabricLoaderEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	res := make([]content.LoaderVersion, 0, len(raw))
	for _, entry := range raw {
		res = append(res, content.LoaderVersion{
			Version: entry.Loader.Version,
			Stable:  entry.Loader.Stable,
			Maven:   entry.Loader.Maven,
		})
	}

	return res, nil
}