package main

import (
	"strings"
	"testing"
)

func TestExtractChangelog(t *testing.T) {
	sampleChangelog := `# Changelog

All notable changes to Nord Launcher are documented in this file.

## [0.5.0] - 2026-09-20

### Added
- Feature A
- Feature B

### Fixed
- Bug C

## [0.4.0] - 2026-09-20

### Added
- Old Feature
`

	// 1. Exact match with brackets
	extracted := ExtractChangelog(sampleChangelog, "0.5.0", 2800)
	if !strings.Contains(extracted, "Feature A") || !strings.Contains(extracted, "Bug C") {
		t.Fatalf("expected extracted changelog to contain Feature A and Bug C, got: %s", extracted)
	}
	if strings.Contains(extracted, "Old Feature") {
		t.Fatalf("extracted changelog should not contain notes from 0.4.0, got: %s", extracted)
	}
	if strings.HasPrefix(extracted, "## ") {
		t.Fatalf("extracted changelog should strip section header, got: %s", extracted)
	}

	// 2. Target version with 'v' prefix
	extractedV := ExtractChangelog(sampleChangelog, "v0.5.0", 2800)
	if extractedV != extracted {
		t.Fatalf("expected ExtractChangelog with 'v0.5.0' to match '0.5.0', got: %s", extractedV)
	}

	// 3. Target version 0.4.0
	extracted04 := ExtractChangelog(sampleChangelog, "0.4.0", 2800)
	if !strings.Contains(extracted04, "Old Feature") {
		t.Fatalf("expected extracted 0.4.0 notes to contain Old Feature, got: %s", extracted04)
	}

	// 4. Missing version returns empty string
	extractedMissing := ExtractChangelog(sampleChangelog, "0.9.9", 2800)
	if extractedMissing != "" {
		t.Fatalf("expected empty string for missing version, got: %s", extractedMissing)
	}

	// 5. Capping at maxBytes
	longChangelog := `## [1.0.0] - 2026-09-20
` + strings.Repeat("A", 3500) + `
## [0.9.0]
old
`
	extractedCapped := ExtractChangelog(longChangelog, "1.0.0", 100)
	if len(extractedCapped) > 105 {
		t.Fatalf("expected capped length around 100 bytes, got %d bytes", len(extractedCapped))
	}
	if !strings.HasSuffix(extractedCapped, "...") {
		t.Fatalf("expected capped changelog to end with '...', got: %s", extractedCapped)
	}
}

func TestBuildReleaseBody(t *testing.T) {
	body := BuildReleaseBody("0.5.0", "stable", "### Added\n- Real feature")
	if !strings.Contains(body, "## Nord Launcher v0.5.0 (stable channel release)") {
		t.Errorf("expected body to contain title header, got: %s", body)
	}
	if !strings.Contains(body, "### Added\n- Real feature") {
		t.Errorf("expected body to contain changelog, got: %s", body)
	}
	if !strings.Contains(body, "compare/v0.1.0...v0.5.0") {
		t.Errorf("expected body to contain full changelog link, got: %s", body)
	}
}

