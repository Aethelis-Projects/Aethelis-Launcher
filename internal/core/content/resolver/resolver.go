package resolver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/downloader"
)

var (
	ErrCircularDependency = errors.New("circular dependency detected")
	ErrNoCompatibleVersion = errors.New("no compatible mod version found for target game/loader")
)

type ConflictError struct {
	ModA string
	ModB string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("incompatible mod conflict: '%s' is incompatible with '%s'", e.ModA, e.ModB)
}

// VersionFetcher abstracts fetching available mod versions from Modrinth or CurseForge.
type VersionFetcher interface {
	GetVersions(ctx context.Context, projectID string, gameVersion, loader string) ([]content.ModVersion, error)
}

type DependencyResolver struct {
	fetcher VersionFetcher
}

func NewDependencyResolver(fetcher VersionFetcher) *DependencyResolver {
	return &DependencyResolver{fetcher: fetcher}
}

type ResolutionResult struct {
	SelectedFiles    []content.ModFile
	ResolvedMods     []string
	DependenciesAdded []string
}

// Resolve processes an initial list of project IDs, recursively fetches required dependencies,
// detects conflicts/incompatibilities, and generates resolved downloadable mod files.
func (r *DependencyResolver) Resolve(
	ctx context.Context,
	initialProjectIDs []string,
	gameVersion string,
	loader string,
	alreadyInstalledIDs []string,
) (*ResolutionResult, error) {
	resolvedProjects := make(map[string]bool)
	for _, id := range alreadyInstalledIDs {
		resolvedProjects[id] = true
	}

	selectedFilesMap := make(map[string]content.ModFile)
	dependenciesAdded := make([]string, 0)
	incompatibilities := make(map[string]string) // incompatibleID -> originatingModID

	queue := append([]string(nil), initialProjectIDs...)
	inInitialList := make(map[string]bool)
	for _, id := range initialProjectIDs {
		inInitialList[id] = true
	}

	visitedInPath := make(map[string]bool)

	for len(queue) > 0 {
		currentProject := queue[0]
		queue = queue[1:]

		if resolvedProjects[currentProject] && !inInitialList[currentProject] {
			continue
		}

		if visitedInPath[currentProject] {
			return nil, fmt.Errorf("%w: at %s", ErrCircularDependency, currentProject)
		}
		visitedInPath[currentProject] = true

		// Check for conflict with previously registered incompatibilities
		if origin, isConflict := incompatibilities[currentProject]; isConflict {
			return nil, &ConflictError{ModA: origin, ModB: currentProject}
		}

		versions, err := r.fetcher.GetVersions(ctx, currentProject, gameVersion, loader)
		if err != nil {
			return nil, fmt.Errorf("fetch versions for mod %s: %w", currentProject, err)
		}
		if len(versions) == 0 {
			return nil, fmt.Errorf("%w: mod %s (game: %s, loader: %s)", ErrNoCompatibleVersion, currentProject, gameVersion, loader)
		}

		// Pick newest matching version
		selectedVer := versions[0]
		if len(selectedVer.Files) == 0 {
			return nil, fmt.Errorf("no downloadable files in version %s for mod %s", selectedVer.ID, currentProject)
		}

		// Primary file
		primaryFile := selectedVer.Files[0]
		for _, f := range selectedVer.Files {
			if f.Primary {
				primaryFile = f
				break
			}
		}
		selectedFilesMap[currentProject] = primaryFile

		// Process dependencies declared in this version
		for _, dep := range selectedVer.Dependencies {
			if dep.Type == content.DepIncompatible {
				incompatibilities[dep.ProjectID] = currentProject
				// Check if we already have the incompatible mod installed or resolved
				if resolvedProjects[dep.ProjectID] {
					return nil, &ConflictError{ModA: currentProject, ModB: dep.ProjectID}
				}
			} else if dep.Type == content.DepRequired {
				if !resolvedProjects[dep.ProjectID] {
					queue = append(queue, dep.ProjectID)
					dependenciesAdded = append(dependenciesAdded, dep.ProjectID)
				}
			}
		}

		resolvedProjects[currentProject] = true
	}

	res := &ResolutionResult{
		DependenciesAdded: dependenciesAdded,
	}

	for id, file := range selectedFilesMap {
		res.ResolvedMods = append(res.ResolvedMods, id)
		res.SelectedFiles = append(res.SelectedFiles, file)
	}

	return res, nil
}

// BuildDownloadTasks converts resolved mod files into prioritized DownloadTasks for the instance mods directory.
func (r *DependencyResolver) BuildDownloadTasks(
	resolvedFiles []content.ModFile,
	modsDir string,
) []*downloader.DownloadTask {
	tasks := make([]*downloader.DownloadTask, 0, len(resolvedFiles))
	for _, f := range resolvedFiles {
		destPath := filepath.Join(modsDir, f.FileName)
		tasks = append(tasks, &downloader.DownloadTask{
			ID:             f.ID,
			URL:            f.URL,
			DestPath:       destPath,
			ExpectedSHA1:   f.SHA1,
			ExpectedSHA256: "", // Modrinth provides sha512 or sha1
			ExpectedSize:   f.Size,
			Priority:       downloader.PriorityNormal,
		})
	}
	return tasks
}