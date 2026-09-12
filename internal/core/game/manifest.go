package game

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

const (
	PistonMetaManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	MojangLibrariesURL   = "https://libraries.minecraft.net"
	MojangResourcesURL   = "https://resources.download.minecraft.net"
)

// VersionManifestV2 represents Mojang's root version manifest catalog.
type VersionManifestV2 struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []VersionManifestEntry `json:"versions"`
}

// VersionManifestEntry represents a single version item in the manifest.
type VersionManifestEntry struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	URL             string    `json:"url"`
	Time            time.Time `json:"time"`
	ReleaseTime     time.Time `json:"releaseTime"`
	SHA1            string    `json:"sha1"`
	ComplianceLevel int       `json:"complianceLevel"`
}

// AssetIndex represents the contents of an asset index json file.
type AssetIndex struct {
	Objects map[string]AssetObject `json:"objects"`
}

// AssetObject represents a single hash-addressed resource file.
type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// FetchVersionManifest downloads and parses the version manifest v2.
func FetchVersionManifest(ctx context.Context, client ports.HTTPClient, manifestURL string) (*VersionManifestV2, error) {
	if manifestURL == "" {
		manifestURL = PistonMetaManifestURL
	}
	body, err := client.Get(ctx, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch version manifest: %w", err)
	}

	var manifest VersionManifestV2
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal version manifest: %w", err)
	}
	return &manifest, nil
}

// FindVersionEntry searches a manifest for a given version ID.
func FindVersionEntry(manifest *VersionManifestV2, versionID string) (*VersionManifestEntry, error) {
	if manifest == nil {
		return nil, domain.ErrInvalidConfig
	}
	for i := range manifest.Versions {
		if manifest.Versions[i].ID == versionID {
			return &manifest.Versions[i], nil
		}
	}
	return nil, domain.ErrVersionNotFound
}
