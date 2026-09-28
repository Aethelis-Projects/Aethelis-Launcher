package wails

import (
	"context"
	"errors"
	"fmt"

	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

// cfPackResolver adapts the CurseForge API client to the pack importer's
// file resolver contract. Lookup is exact on fileID: a stale/removed file
// must surface as an error (reported to the UI), never as a substitute build.
type cfPackResolver struct {
	client *curseforge.Client
}

func (r *cfPackResolver) ResolvePackFile(ctx context.Context, projectID, fileID int64) (string, string, int64, string, error) {
	if r == nil || r.client == nil {
		return "", "", 0, "", errors.New("curseforge client not initialized")
	}
	if r.client.APIKey() == "" {
		return "", "", 0, "", errors.New("curseforge API key unavailable")
	}
	files, err := r.client.GetModFiles(ctx, projectID, "", "")
	if err != nil {
		return "", "", 0, "", fmt.Errorf("resolve cf file %d/%d: %w", projectID, fileID, err)
	}
	for _, f := range files {
		if f.ID == fmt.Sprintf("%d", fileID) {
			if f.URL == "" {
				return "", "", 0, "", fmt.Errorf("curseforge returned no download URL for file %d", fileID)
			}
			return f.URL, f.SHA1, f.Size, f.FileName, nil
		}
	}
	return "", "", 0, "", fmt.Errorf("file %d not found under project %d (removed or renamed)", fileID, projectID)
}

var _ launch.CurseForgePackFileResolver = (*cfPackResolver)(nil)
