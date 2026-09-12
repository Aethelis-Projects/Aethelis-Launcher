package java

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"
)

const (
	DefaultAdoptiumBaseURL = "https://api.adoptium.net/v3"
)

type AdoptiumAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Version     string `json:"version"`
}

type adoptiumReleaseItem struct {
	Binaries []struct {
		ImageType string `json:"image_type"`
		OS        string `json:"os"`
		Arch      string `json:"architecture"`
		Package   struct {
			Name     string `json:"name"`
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
			Size     int64  `json:"size"`
		} `json:"package"`
	} `json:"binaries"`
	VersionData struct {
		Semver string `json:"semver"`
	} `json:"version_data"`
}

type AdoptiumClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAdoptiumClient(baseURL string, client *http.Client) *AdoptiumClient {
	if baseURL == "" {
		baseURL = DefaultAdoptiumBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &AdoptiumClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

// MapGoPlatformToAdoptium maps Go GOOS and GOARCH to Adoptium API platform names.
func MapGoPlatformToAdoptium(goos, goarch string) (osName, archName string) {
	switch goos {
	case "windows":
		osName = "windows"
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "mac"
	default:
		osName = goos
	}

	switch goarch {
	case "amd64":
		archName = "x64"
	case "arm64":
		archName = "aarch64"
	default:
		archName = goarch
	}
	return osName, archName
}

// GetLatestRelease fetches the latest GA JDK release for the requested Java major version.
func (c *AdoptiumClient) GetLatestRelease(ctx context.Context, major int) (*AdoptiumAsset, error) {
	osName, archName := MapGoPlatformToAdoptium(runtime.GOOS, runtime.GOARCH)

	endpoint := fmt.Sprintf("%s/assets/feature_releases/%d/ga?os=%s&architecture=%s&image_type=jdk&vendor=eclipse",
		c.baseURL, major, osName, archName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create adoptium request: %w", err)
	}
	req.Header.Set("User-Agent", "Nord-Launcher/0.1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adoptium api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("adoptium api returned status %d: %s", resp.StatusCode, string(body))
	}

	var releases []adoptiumReleaseItem
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode adoptium response: %w", err)
	}

	if len(releases) == 0 || len(releases[0].Binaries) == 0 {
		return nil, fmt.Errorf("no adoptium binaries found for java %d on %s/%s", major, osName, archName)
	}

	rel := releases[0]
	bin := rel.Binaries[0]

	return &AdoptiumAsset{
		Name:        bin.Package.Name,
		DownloadURL: bin.Package.Link,
		SHA256:      bin.Package.Checksum,
		Size:        bin.Package.Size,
		Version:     rel.VersionData.Semver,
	}, nil
}
