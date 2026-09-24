package java

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/netutil"
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

type JavaRuntimeUpdate struct {
	MajorVersion    int    `json:"major_version"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	DownloadURL     string `json:"download_url,omitempty"`
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
		client = netutil.NewHTTPClient("0.5.0", 30*time.Second)
	} else {
		client.Transport = netutil.NewTransport("0.5.0", client.Transport)
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
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", netutil.FormatUserAgent("0.5.0"))
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adoptium api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) // errcheck:ok read error body
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

// IsNewerVersion returns true if latest is strictly newer than current according to Java/Semver build numbering.
func IsNewerVersion(current, latest string) bool {
	currClean := strings.TrimSpace(current)
	latClean := strings.TrimSpace(latest)
	if currClean == "" || latClean == "" || currClean == latClean {
		return false
	}

	currVer, currBuild := splitVersionAndBuild(currClean)
	latVer, latBuild := splitVersionAndBuild(latClean)

	currParts := strings.Split(currVer, ".")
	latParts := strings.Split(latVer, ".")

	maxLen := len(currParts)
	if len(latParts) > maxLen {
		maxLen = len(latParts)
	}

	for i := 0; i < maxLen; i++ {
		var nCurr, nLat int
		if i < len(currParts) {
			nCurr = extractNumeric(currParts[i])
		}
		if i < len(latParts) {
			nLat = extractNumeric(latParts[i])
		}
		if nLat > nCurr {
			return true
		}
		if nLat < nCurr {
			return false
		}
	}

	bCurr := extractNumeric(currBuild)
	bLat := extractNumeric(latBuild)
	return bLat > bCurr
}

func splitVersionAndBuild(v string) (verPart, buildPart string) {
	v = strings.TrimPrefix(v, "jdk-")
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "+"); idx != -1 {
		return v[:idx], v[idx+1:]
	}
	if idx := strings.Index(v, "_"); idx != -1 {
		return v[:idx], v[idx+1:]
	}
	if idx := strings.Index(v, "-"); idx != -1 {
		return v[:idx], v[idx+1:]
	}
	return v, ""
}

func extractNumeric(s string) int {
	var sb strings.Builder
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			sb.WriteRune(ch)
		}
	}
	if sb.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(sb.String()) // errcheck:ok fallback 0 on invalid int
	return n
}
