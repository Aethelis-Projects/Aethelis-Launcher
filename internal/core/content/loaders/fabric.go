package loaders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/netutil"
)

const DefaultFabricMetaURL = "https://meta.fabricmc.net"

type FabricClient struct {
	baseURL    string
	version    string
	httpClient *http.Client
}

func NewFabricClient(baseURL string, client *http.Client) *FabricClient {
	if baseURL == "" {
		baseURL = DefaultFabricMetaURL
	}
	if client == nil {
		client = netutil.NewHTTPClient("0.6.1", 15*time.Second)
	}
	return &FabricClient{
		baseURL:    baseURL,
		version:    "0.6.1",
		httpClient: client,
	}
}

func (c *FabricClient) SetVersion(v string) {
	c.version = v
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
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", netutil.FormatUserAgent(c.version))
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
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

type FabricProfileLibrary struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type FabricProfile struct {
	ID                string                 `json:"id"`
	InheritsFrom      string                 `json:"inheritsFrom"`
	MainClass         string                 `json:"mainClass"`
	LauncherMainClass string                 `json:"launcherMainClass"`
	Libraries         []FabricProfileLibrary `json:"libraries"`
}

// GetProfile retrieves the version profile JSON for a specific game version and Fabric loader version.
func (c *FabricClient) GetProfile(ctx context.Context, gameVersion, loaderVersion string) (*FabricProfile, error) {
	url := fmt.Sprintf("%s/v2/versions/loader/%s/%s/profile/json", c.baseURL, gameVersion, loaderVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", netutil.FormatUserAgent(c.version))
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch fabric profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fabric meta profile returned HTTP %d", resp.StatusCode)
	}

	var profile FabricProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode fabric profile: %w", err)
	}

	return &profile, nil
}