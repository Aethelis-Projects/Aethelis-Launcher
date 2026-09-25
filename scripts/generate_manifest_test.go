package main

import (
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/updater"
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
	platforms := map[string]updater.PlatformAsset{
		"windows-amd64": {
			URL:    "https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/v0.6.0/NordLauncher.exe",
			SHA256: "eeaa238a49b08f078dcd93d9833d8a326d22ad3f3bbe135eedac57dd30b1e672",
			Size:   18067072,
		},
		"windows-setup": {
			URL:    "https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/v0.6.0/NordLauncher-Setup.exe",
			SHA256: "d0e6dbac4c5e599bd751b2ea62269c53efb2b091b5d09f71566b55fa48f05b00",
			Size:   7264832,
		},
		"linux-amd64": {
			URL:    "https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/v0.6.0/nord-launcher-v0.6.0-linux-amd64.tar.gz",
			SHA256: "53b64c7eb6b1347f5d6b32032dbebf6fcbbb97d2ea4b64228ae13095495066fd",
			Size:   7152226,
		},
	}

	body := BuildReleaseBody("0.6.0", "stable", "### Added\n- Real feature", platforms)

	// 1. Title Header
	if !strings.Contains(body, "## Nord Launcher v0.6.0 (stable channel release)") {
		t.Errorf("expected body to contain title header, got: %s", body)
	}

	// 2. Overview Section
	if !strings.Contains(body, "### Overview") {
		t.Errorf("expected body to contain '### Overview' header, got: %s", body)
	}
	if !strings.Contains(body, "modpack round-trip support") {
		t.Errorf("expected body to contain v0.6.0 specific overview, got: %s", body)
	}

	// 3. Changelog Section
	if !strings.Contains(body, "### Changelog") || !strings.Contains(body, "### Added\n- Real feature") {
		t.Errorf("expected body to contain changelog section, got: %s", body)
	}

	// 4. Published Artifacts Table
	if !strings.Contains(body, "### Published Artifacts") {
		t.Errorf("expected body to contain '### Published Artifacts' header, got: %s", body)
	}
	if !strings.Contains(body, "| Asset | Platform | Size | SHA-256 Checksum |") {
		t.Errorf("expected body to contain artifact table header, got: %s", body)
	}
	if !strings.Contains(body, "NordLauncher.exe") || !strings.Contains(body, "NordLauncher-Setup.exe") || !strings.Contains(body, "nord-launcher-v0.6.0-linux-amd64.tar.gz") {
		t.Errorf("expected body table to contain all platform assets, got: %s", body)
	}

	// 5. Verification & Forensic Integrity Section
	if !strings.Contains(body, "### Verification & Forensic Integrity") {
		t.Errorf("expected body to contain '### Verification & Forensic Integrity' header, got: %s", body)
	}
	if !strings.Contains(body, updater.DefaultPublicKeyHex) {
		t.Errorf("expected body to contain default public key, got: %s", body)
	}

	// 6. Full Changelog Link
	if !strings.Contains(body, "compare/v0.1.0...v0.6.0") {
		t.Errorf("expected body to contain full changelog link, got: %s", body)
	}
}

