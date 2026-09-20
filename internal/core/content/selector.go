package content

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrNoCompatibleVersion = errors.New("no compatible mod version found for target game/loader")

type SelectionResult struct {
	File          *ModFile
	Version       *ModVersion
	FallbackLevel int // 1 = exact MC+loader, 2 = MC only, 3 = unconstrained
	Warning       string
}

// SelectBestModFile deterministically selects the best file from a slice of candidate ModFiles
// using the 3-level fallback grid (Level 1: gv+loader, Level 2: gv only, Level 3: unrestricted).
func SelectBestModFile(files []ModFile, targetMC, targetLoader string) (*SelectionResult, error) {
	if len(files) == 0 {
		return nil, ErrNoCompatibleVersion
	}

	// Level 1: exact gv + loader match
	var l1Candidates []ModFile
	for _, f := range files {
		if matchGameVersion(f.GameVersions, targetMC) && matchLoader(f.Loaders, targetLoader) {
			l1Candidates = append(l1Candidates, f)
		}
	}
	if len(l1Candidates) > 0 {
		sortFiles(l1Candidates)
		best := l1Candidates[0]
		return &SelectionResult{
			File:          &best,
			FallbackLevel: 1,
		}, nil
	}

	// Level 2: gv match only (loader relaxed)
	var l2Candidates []ModFile
	for _, f := range files {
		if matchGameVersion(f.GameVersions, targetMC) {
			l2Candidates = append(l2Candidates, f)
		}
	}
	if len(l2Candidates) > 0 {
		sortFiles(l2Candidates)
		best := l2Candidates[0]
		return &SelectionResult{
			File:          &best,
			FallbackLevel: 2,
		}, nil
	}

	// Level 3: unconstrained fallback
	l3Candidates := make([]ModFile, len(files))
	copy(l3Candidates, files)
	sortFiles(l3Candidates)
	best := l3Candidates[0]
	return &SelectionResult{
		File:          &best,
		FallbackLevel: 3,
		Warning:       fmt.Sprintf("не проверено под %s", targetMC),
	}, nil
}

// SelectBestModVersion deterministically selects the best version and its primary file from ModVersions.
func SelectBestModVersion(versions []ModVersion, targetMC, targetLoader string) (*SelectionResult, error) {
	if len(versions) == 0 {
		return nil, ErrNoCompatibleVersion
	}

	// Level 1: exact gv + loader match
	var l1 []ModVersion
	for _, v := range versions {
		if len(v.Files) > 0 && matchGameVersion(v.GameVersions, targetMC) && matchLoader(v.Loaders, targetLoader) {
			l1 = append(l1, v)
		}
	}
	if len(l1) > 0 {
		sortVersions(l1)
		bestVer := l1[0]
		bestFile := pickBestFileFromVersion(&bestVer)
		return &SelectionResult{
			File:          bestFile,
			Version:       &bestVer,
			FallbackLevel: 1,
		}, nil
	}

	// Level 2: gv match only
	var l2 []ModVersion
	for _, v := range versions {
		if len(v.Files) > 0 && matchGameVersion(v.GameVersions, targetMC) {
			l2 = append(l2, v)
		}
	}
	if len(l2) > 0 {
		sortVersions(l2)
		bestVer := l2[0]
		bestFile := pickBestFileFromVersion(&bestVer)
		return &SelectionResult{
			File:          bestFile,
			Version:       &bestVer,
			FallbackLevel: 2,
		}, nil
	}

	// Level 3: unconstrained
	var l3 []ModVersion
	for _, v := range versions {
		if len(v.Files) > 0 {
			l3 = append(l3, v)
		}
	}
	if len(l3) == 0 {
		return nil, ErrNoCompatibleVersion
	}
	sortVersions(l3)
	bestVer := l3[0]
	bestFile := pickBestFileFromVersion(&bestVer)
	return &SelectionResult{
		File:          bestFile,
		Version:       &bestVer,
		FallbackLevel: 3,
		Warning:       fmt.Sprintf("не проверено под %s", targetMC),
	}, nil
}

func sortFiles(files []ModFile) {
	sort.SliceStable(files, func(i, j int) bool {
		rI := ReleaseTypeRank(files[i].ReleaseType)
		rJ := ReleaseTypeRank(files[j].ReleaseType)
		if rI != rJ {
			return rI < rJ // Release (0) > Beta (1) > Alpha (2)
		}
		if !files[i].FileDate.Equal(files[j].FileDate) {
			return files[i].FileDate.After(files[j].FileDate)
		}
		if files[i].Primary != files[j].Primary {
			return files[i].Primary
		}
		return files[i].ID < files[j].ID
	})
}

func sortVersions(versions []ModVersion) {
	sort.SliceStable(versions, func(i, j int) bool {
		rI := ReleaseTypeRank(versions[i].VersionType)
		rJ := ReleaseTypeRank(versions[j].VersionType)
		if rI != rJ {
			return rI < rJ
		}
		if !versions[i].ReleaseDate.Equal(versions[j].ReleaseDate) {
			return versions[i].ReleaseDate.After(versions[j].ReleaseDate)
		}
		return versions[i].ID < versions[j].ID
	})
}

func pickBestFileFromVersion(v *ModVersion) *ModFile {
	if len(v.Files) == 0 {
		return nil
	}
	for i := range v.Files {
		if v.Files[i].Primary {
			return &v.Files[i]
		}
	}
	return &v.Files[0]
}

func matchGameVersion(versions []string, target string) bool {
	if target == "" {
		return true
	}
	t := strings.TrimSpace(strings.ToLower(target))
	for _, v := range versions {
		if strings.TrimSpace(strings.ToLower(v)) == t {
			return true
		}
	}
	return false
}

func matchLoader(loaders []string, target string) bool {
	if target == "" {
		return true
	}
	t := strings.TrimSpace(strings.ToLower(target))
	for _, l := range loaders {
		lNorm := strings.TrimSpace(strings.ToLower(l))
		if lNorm == t {
			return true
		}
		if t == "quilt" && lNorm == "fabric" {
			return true
		}
	}
	return false
}
