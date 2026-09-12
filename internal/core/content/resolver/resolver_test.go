package resolver_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/resolver"
)

type mockFetcher struct {
	modVersions map[string][]content.ModVersion
}

func (m *mockFetcher) GetVersions(ctx context.Context, projectID string, gameVersion, loader string) ([]content.ModVersion, error) {
	v, ok := m.modVersions[projectID]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func TestResolver_RecursiveDependencies(t *testing.T) {
	fetcher := &mockFetcher{
		modVersions: map[string][]content.ModVersion{
			"sodium": {
				{
					ID:         "sodium-v1",
					ProjectID:  "sodium",
					VersionNum: "0.5.8",
					Files: []content.ModFile{
						{
							ID:       "sodium-file-1",
							FileName: "sodium-0.5.8.jar",
							URL:      "https://example.com/sodium.jar",
							SHA1:     "1234567890123456789012345678901234567890",
							Size:     500000,
							Primary:  true,
						},
					},
					Dependencies: []content.ModDependency{
						{
							ProjectID: "fabric-api",
							Type:      content.DepRequired,
						},
					},
				},
			},
			"fabric-api": {
				{
					ID:         "fabric-api-v1",
					ProjectID:  "fabric-api",
					VersionNum: "0.96.0",
					Files: []content.ModFile{
						{
							ID:       "fabric-api-file-1",
							FileName: "fabric-api-0.96.0.jar",
							URL:      "https://example.com/fabric-api.jar",
							SHA1:     "abcdefabcdefabcdefabcdefabcdefabcdefabcd",
							Size:     2000000,
							Primary:  true,
						},
					},
				},
			},
		},
	}

	r := resolver.NewDependencyResolver(fetcher)
	res, err := r.Resolve(context.Background(), []string{"sodium"}, "1.21.1", "fabric", nil)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(res.ResolvedMods) != 2 {
		t.Fatalf("expected 2 resolved mods, got %d", len(res.ResolvedMods))
	}
	if len(res.DependenciesAdded) != 1 || res.DependenciesAdded[0] != "fabric-api" {
		t.Fatalf("expected fabric-api as added dependency, got %+v", res.DependenciesAdded)
	}
	if len(res.SelectedFiles) != 2 {
		t.Fatalf("expected 2 selected files, got %d", len(res.SelectedFiles))
	}

	// Verify download task building
	tempModsDir := filepath.Join("C:", "Nord", "instances", "test", "mods")
	tasks := r.BuildDownloadTasks(res.SelectedFiles, tempModsDir)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 download tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if filepath.Dir(task.DestPath) != tempModsDir {
			t.Errorf("expected destination dir %s, got %s", tempModsDir, task.DestPath)
		}
		if task.ExpectedSHA1 == "" {
			t.Errorf("missing expected sha1 in task: %+v", task)
		}
	}
}

func TestResolver_IncompatibleConflict(t *testing.T) {
	fetcher := &mockFetcher{
		modVersions: map[string][]content.ModVersion{
			"sodium": {
				{
					ID:        "sodium-v1",
					ProjectID: "sodium",
					Files:     []content.ModFile{{FileName: "sodium.jar", Primary: true}},
					Dependencies: []content.ModDependency{
						{ProjectID: "optifine", Type: content.DepIncompatible},
					},
				},
			},
			"optifine": {
				{
					ID:        "optifine-v1",
					ProjectID: "optifine",
					Files:     []content.ModFile{{FileName: "optifine.jar", Primary: true}},
				},
			},
		},
	}

	r := resolver.NewDependencyResolver(fetcher)
	// Try install sodium when optifine is already installed
	_, err := r.Resolve(context.Background(), []string{"sodium"}, "1.21.1", "fabric", []string{"optifine"})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	var conflictErr *resolver.ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestResolver_AlreadyInstalledSkip(t *testing.T) {
	fetcher := &mockFetcher{
		modVersions: map[string][]content.ModVersion{
			"sodium": {
				{
					ID:        "sodium-v1",
					ProjectID: "sodium",
					Files:     []content.ModFile{{FileName: "sodium.jar", Primary: true}},
					Dependencies: []content.ModDependency{
						{ProjectID: "fabric-api", Type: content.DepRequired},
					},
				},
			},
			"fabric-api": {
				{
					ID:        "fabric-api-v1",
					ProjectID: "fabric-api",
					Files:     []content.ModFile{{FileName: "fabric-api.jar", Primary: true}},
				},
			},
		},
	}

	r := resolver.NewDependencyResolver(fetcher)
	// fabric-api is already installed
	res, err := r.Resolve(context.Background(), []string{"sodium"}, "1.21.1", "fabric", []string{"fabric-api"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Only sodium needs to be downloaded
	if len(res.SelectedFiles) != 1 || res.SelectedFiles[0].FileName != "sodium.jar" {
		t.Fatalf("expected only sodium to be selected, got %+v", res.SelectedFiles)
	}
	if len(res.DependenciesAdded) != 0 {
		t.Fatalf("expected 0 new dependencies, got %+v", res.DependenciesAdded)
	}
}

func TestConflictError(t *testing.T) {
	err := &resolver.ConflictError{ModA: "ModA", ModB: "ModB"}
	if err.Error() == "" {
		t.Errorf("expected non-empty error string")
	}
}