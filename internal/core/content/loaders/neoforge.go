package loaders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

const DefaultNeoForgeMavenURL = "https://maven.neoforged.net"

type NeoForgeClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewNeoForgeClient(baseURL string, client *http.Client) *NeoForgeClient {
	if baseURL == "" {
		baseURL = DefaultNeoForgeMavenURL
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NeoForgeClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

type neoForgeMavenMeta struct {
	Versions []string `json:"versions"`
}

// GetLoadersForGameVersion returns NeoForge releases matching the game version prefix (e.g. 21.1 for Minecraft 1.21.1).
func (c *NeoForgeClient) GetLoadersForGameVersion(ctx context.Context, gameVersion string) ([]content.LoaderVersion, error) {
	url := fmt.Sprintf("%s/api/maven/versions/releases/net/neoforged/neoforge", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch neoforge versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("neoforge maven returned HTTP %d", resp.StatusCode)
	}

	var meta neoForgeMavenMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}

	// Minecraft 1.20.4 -> NeoForge 20.4.x, 1.21.1 -> NeoForge 21.1.x
	prefix := ""
	parts := strings.Split(gameVersion, ".")
	if len(parts) >= 2 && parts[0] == "1" {
		major := parts[1]
		minor := "0"
		if len(parts) >= 3 {
			minor = parts[2]
		}
		prefix = fmt.Sprintf("%s.%s", major, minor)
	}

	res := make([]content.LoaderVersion, 0)
	for i := len(meta.Versions) - 1; i >= 0; i-- {
		v := meta.Versions[i]
		if prefix == "" || strings.HasPrefix(v, prefix) {
			res = append(res, content.LoaderVersion{
				Version: v,
				Stable:  !strings.Contains(v, "beta") && !strings.Contains(v, "alpha"),
				Maven:   fmt.Sprintf("net.neoforged:neoforge:%s", v),
			})
		}
	}

	return res, nil
}