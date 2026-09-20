package content_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/content"
)

func TestSelectBestModFile_Grid(t *testing.T) {
	now := time.Now()

	// 1. Level 1: Release beats newer Beta; newer Release beats older Release
	filesL1 := []content.ModFile{
		{
			ID:           "file-alpha",
			FileName:     "mod-1.20.1-alpha.jar",
			ReleaseType:  content.ReleaseTypeAlpha,
			FileDate:     now.Add(2 * time.Hour),
			GameVersions: []string{"1.20.1", "fabric"},
			Loaders:      []string{"fabric"},
		},
		{
			ID:           "file-beta-new",
			FileName:     "mod-1.20.1-beta.jar",
			ReleaseType:  content.ReleaseTypeBeta,
			FileDate:     now.Add(1 * time.Hour),
			GameVersions: []string{"1.20.1", "fabric"},
			Loaders:      []string{"fabric"},
		},
		{
			ID:           "file-rel-old",
			FileName:     "mod-1.20.1-rel-1.0.jar",
			ReleaseType:  content.ReleaseTypeRelease,
			FileDate:     now.Add(-24 * time.Hour),
			GameVersions: []string{"1.20.1", "fabric"},
			Loaders:      []string{"fabric"},
		},
		{
			ID:           "file-rel-new",
			FileName:     "mod-1.20.1-rel-1.1.jar",
			ReleaseType:  content.ReleaseTypeRelease,
			FileDate:     now,
			GameVersions: []string{"1.20.1", "fabric"},
			Loaders:      []string{"fabric"},
			Primary:      true,
		},
	}

	res, err := content.SelectBestModFile(filesL1, "1.20.1", "fabric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FallbackLevel != 1 {
		t.Errorf("expected FallbackLevel 1, got %d", res.FallbackLevel)
	}
	if res.File.ID != "file-rel-new" {
		t.Errorf("expected file-rel-new, got %s", res.File.ID)
	}

	// 2. Level 2: Loader mismatch fallback (MC match only)
	filesL2 := []content.ModFile{
		{
			ID:           "file-forge-rel",
			FileName:     "mod-1.20.1-forge.jar",
			ReleaseType:  content.ReleaseTypeRelease,
			FileDate:     now,
			GameVersions: []string{"1.20.1", "forge"},
			Loaders:      []string{"forge"},
		},
	}
	resL2, err := content.SelectBestModFile(filesL2, "1.20.1", "neoforge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resL2.FallbackLevel != 2 {
		t.Errorf("expected FallbackLevel 2, got %d", resL2.FallbackLevel)
	}
	if resL2.File.ID != "file-forge-rel" {
		t.Errorf("expected file-forge-rel, got %s", resL2.File.ID)
	}

	// 3. Level 3: Unconstrained fallback with warning
	filesL3 := []content.ModFile{
		{
			ID:           "file-1.19-rel",
			FileName:     "mod-1.19.4-rel.jar",
			ReleaseType:  content.ReleaseTypeRelease,
			FileDate:     now,
			GameVersions: []string{"1.19.4"},
			Loaders:      []string{"forge"},
		},
	}
	resL3, err := content.SelectBestModFile(filesL3, "1.21.1", "fabric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resL3.FallbackLevel != 3 {
		t.Errorf("expected FallbackLevel 3, got %d", resL3.FallbackLevel)
	}
	if resL3.Warning == "" {
		t.Error("expected warning for Level 3 fallback, got empty")
	}

	// 4. Empty candidate pool
	_, errEmpty := content.SelectBestModFile([]content.ModFile{}, "1.20.1", "fabric")
	if !errors.Is(errEmpty, content.ErrNoCompatibleVersion) {
		t.Errorf("expected ErrNoCompatibleVersion, got %v", errEmpty)
	}

	// 5. Quilt compatibility with Fabric
	filesQuilt := []content.ModFile{
		{
			ID:           "file-fabric-for-quilt",
			FileName:     "mod-fabric.jar",
			ReleaseType:  content.ReleaseTypeRelease,
			FileDate:     now,
			GameVersions: []string{"1.20.1"},
			Loaders:      []string{"fabric"},
		},
	}
	resQuilt, err := content.SelectBestModFile(filesQuilt, "1.20.1", "quilt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resQuilt.FallbackLevel != 1 {
		t.Errorf("expected FallbackLevel 1 for quilt -> fabric compatibility, got %d", resQuilt.FallbackLevel)
	}
}

func TestSelectBestModVersion_Grid(t *testing.T) {
	now := time.Now()

	// 1. Level 1: Modrinth versions with primary files
	versionsL1 := []content.ModVersion{
		{
			ID:           "ver-beta",
			VersionType:  content.ReleaseTypeBeta,
			ReleaseDate:  now.Add(1 * time.Hour),
			GameVersions: []string{"1.20.1"},
			Loaders:      []string{"fabric"},
			Files: []content.ModFile{
				{ID: "f-beta", FileName: "beta.jar", Primary: true},
			},
		},
		{
			ID:           "ver-release",
			VersionType:  content.ReleaseTypeRelease,
			ReleaseDate:  now,
			GameVersions: []string{"1.20.1"},
			Loaders:      []string{"fabric"},
			Files: []content.ModFile{
				{ID: "f-sec", FileName: "release-sources.jar", Primary: false},
				{ID: "f-prim", FileName: "release.jar", Primary: true},
			},
		},
	}

	res, err := content.SelectBestModVersion(versionsL1, "1.20.1", "fabric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FallbackLevel != 1 {
		t.Errorf("expected FallbackLevel 1, got %d", res.FallbackLevel)
	}
	if res.Version.ID != "ver-release" {
		t.Errorf("expected ver-release, got %s", res.Version.ID)
	}
	if res.File.ID != "f-prim" {
		t.Errorf("expected primary file f-prim, got %s", res.File.ID)
	}

	// 2. Level 2: Loader mismatch fallback
	versionsL2 := []content.ModVersion{
		{
			ID:           "ver-forge-only",
			VersionType:  content.ReleaseTypeRelease,
			ReleaseDate:  now,
			GameVersions: []string{"1.20.1"},
			Loaders:      []string{"forge"},
			Files: []content.ModFile{
				{ID: "f-forge", FileName: "forge.jar", Primary: true},
			},
		},
	}
	resL2, err := content.SelectBestModVersion(versionsL2, "1.20.1", "fabric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resL2.FallbackLevel != 2 {
		t.Errorf("expected FallbackLevel 2, got %d", resL2.FallbackLevel)
	}

	// 3. Level 3: Unconstrained fallback with warning
	versionsL3 := []content.ModVersion{
		{
			ID:           "ver-1.18",
			VersionType:  content.ReleaseTypeRelease,
			ReleaseDate:  now,
			GameVersions: []string{"1.18.2"},
			Loaders:      []string{"forge"},
			Files: []content.ModFile{
				{ID: "f-1.18", FileName: "1.18.jar", Primary: true},
			},
		},
	}
	resL3, err := content.SelectBestModVersion(versionsL3, "1.20.1", "fabric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resL3.FallbackLevel != 3 {
		t.Errorf("expected FallbackLevel 3, got %d", resL3.FallbackLevel)
	}
	if resL3.Warning == "" {
		t.Error("expected non-empty warning at Level 3")
	}

	// 4. Empty candidate pool
	_, errEmpty := content.SelectBestModVersion([]content.ModVersion{}, "1.20.1", "fabric")
	if !errors.Is(errEmpty, content.ErrNoCompatibleVersion) {
		t.Errorf("expected ErrNoCompatibleVersion, got %v", errEmpty)
	}

	// 5. Versions with no files -> ErrNoCompatibleVersion
	noFiles := []content.ModVersion{
		{ID: "v-empty", GameVersions: []string{"1.20.1"}, Loaders: []string{"fabric"}, Files: nil},
	}
	_, errNoFiles := content.SelectBestModVersion(noFiles, "1.20.1", "fabric")
	if !errors.Is(errNoFiles, content.ErrNoCompatibleVersion) {
		t.Errorf("expected ErrNoCompatibleVersion, got %v", errNoFiles)
	}
}
