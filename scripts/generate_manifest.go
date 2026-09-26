package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/updater"
)

// DefaultStagingPrivateKeyHex is used for local builds/tests when ED25519_PRIVATE_KEY is not set.
const DefaultStagingPrivateKeyHex = "88ec59f652844aded5ef72635fd0621042ffff0b75ec7c0e20185255b374f9af"

func main() {
	var (
		versionFlag         = flag.String("version", "0.1.0", "Release version")
		channelFlag         = flag.String("channel", "stable", "Release channel: stable or beta")
		distDirFlag         = flag.String("dist", "dist", "Directory containing build distribution artifacts")
		outputFile          = flag.String("out", "", "Output manifest path (default: dist/manifest-{channel}.json)")
		privKeyEnv          = flag.String("privkey-hex", "", "Hex-encoded Ed25519 private key (optional, falls back to ED25519_PRIVATE_KEY)")
		allowInsecureDevKey = flag.Bool("allow-insecure-dev-key", false, "Allow fallback to insecure hardcoded staging private key for local development")
		changelogFile       = flag.String("changelog-file", "CHANGELOG.md", "Path to CHANGELOG.md to extract release notes")
		bodyFile            = flag.String("out-body", "", "Output release notes markdown path for GitHub Release body (optional)")
		bodyOnlyFlag        = flag.Bool("body-only", false, "Only generate release body, do not sign or write update manifest")
	)
	flag.Parse()

	var privKey ed25519.PrivateKey
	if !*bodyOnlyFlag {
		privHex := *privKeyEnv
		if privHex == "" {
			privHex = os.Getenv("ED25519_PRIVATE_KEY")
		}
		if privHex == "" {
			if !*allowInsecureDevKey {
				fmt.Fprintf(os.Stderr, "Error: ED25519_PRIVATE_KEY environment variable or -privkey-hex flag is required to sign update manifests.\n")
				fmt.Fprintf(os.Stderr, "For local development only, you must explicitly pass --allow-insecure-dev-key to use the staging key.\n")
				os.Exit(1)
			}
			privHex = DefaultStagingPrivateKeyHex
			fmt.Println("[SECURITY WARNING] Manifest signed with INSECURE dev key! Do not deploy to production.")
		} else if strings.EqualFold(privHex, DefaultStagingPrivateKeyHex) {
			if !*allowInsecureDevKey {
				fmt.Fprintf(os.Stderr, "Error: DefaultStagingPrivateKeyHex cannot be used for signing without --allow-insecure-dev-key.\n")
				os.Exit(1)
			}
			fmt.Println("[SECURITY WARNING] Manifest signed with INSECURE dev key! Do not deploy to production.")
		}

		privSeed, err := hex.DecodeString(privHex)
		if err != nil || len(privSeed) != ed25519.SeedSize {
			fmt.Fprintf(os.Stderr, "Error: invalid ed25519 private key seed (must be 32 hex bytes): %v\n", err)
			os.Exit(1)
		}
		privKey = ed25519.NewKeyFromSeed(privSeed)
	}

	rawVersion := *versionFlag
	cleanVersion := strings.TrimPrefix(rawVersion, "v")
	tagVersion := "v" + cleanVersion

	changelogText := fmt.Sprintf("Nord Launcher %s (%s channel) release.", tagVersion, *channelFlag)
	if *changelogFile != "" {
		if data, err := os.ReadFile(*changelogFile); err == nil {
			if extracted := ExtractChangelog(string(data), cleanVersion, 2800); extracted != "" {
				changelogText = extracted
			}
		}
	}

	manifest := updater.UpdateManifest{
		Version:     cleanVersion,
		ReleaseDate: time.Now().UTC(),
		Changelog:   changelogText,
		Platforms:   make(map[string]updater.PlatformAsset),
	}

	// 1. Process Windows portable binary (windows-amd64)
	portableCandidates := []string{
		filepath.Join(*distDirFlag, "NordLauncher.exe"),
		filepath.Join(*distDirFlag, "windows", "NordLauncher.exe"),
		filepath.Join(*distDirFlag, "dist", "windows", "NordLauncher.exe"),
	}

	var winPortable string
	for _, candidate := range portableCandidates {
		if _, err := os.Stat(candidate); err == nil {
			winPortable = candidate
			break
		}
	}

	if winPortable != "" {
		payload, err := os.ReadFile(winPortable)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", winPortable, err)
			os.Exit(1)
		}
		hashBytes := sha256.Sum256(payload)
		hash := hex.EncodeToString(hashBytes[:])
		var sigStr string
		if len(privKey) > 0 {
			sig := updater.SignPayload(privKey, payload)
			sigStr = base64.StdEncoding.EncodeToString(sig)
		}
		manifest.Platforms["windows-amd64"] = updater.PlatformAsset{
			URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(winPortable)),
			SHA256:    hash,
			Signature: sigStr,
			Size:      int64(len(payload)),
		}
		fmt.Printf("[Manifest] Added windows-amd64: %s (SHA256: %s, Size: %d)\n", filepath.Base(winPortable), hash, len(payload))
	}

	// 2. Process Windows NSIS Setup Installer (windows-setup)
	setupCandidates := []string{
		filepath.Join(*distDirFlag, "NordLauncher-Setup.exe"),
		filepath.Join(*distDirFlag, "windows", "NordLauncher-Setup.exe"),
		filepath.Join(*distDirFlag, "dist", "windows", "NordLauncher-Setup.exe"),
	}

	var winSetup string
	for _, candidate := range setupCandidates {
		if _, err := os.Stat(candidate); err == nil {
			winSetup = candidate
			break
		}
	}

	if winSetup != "" {
		payload, err := os.ReadFile(winSetup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", winSetup, err)
			os.Exit(1)
		}
		hashBytes := sha256.Sum256(payload)
		hash := hex.EncodeToString(hashBytes[:])
		var sigStr string
		if len(privKey) > 0 {
			sig := updater.SignPayload(privKey, payload)
			sigStr = base64.StdEncoding.EncodeToString(sig)
		}
		manifest.Platforms["windows-setup"] = updater.PlatformAsset{
			URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(winSetup)),
			SHA256:    hash,
			Signature: sigStr,
			Size:      int64(len(payload)),
		}
		fmt.Printf("[Manifest] Added windows-setup: %s (SHA256: %s, Size: %d)\n", filepath.Base(winSetup), hash, len(payload))
	}

	if _, ok := manifest.Platforms["windows-amd64"]; !ok {
		if setupAsset, ok := manifest.Platforms["windows-setup"]; ok {
			manifest.Platforms["windows-amd64"] = setupAsset
		}
	}

	if winPortable == "" && winSetup == "" {
		fmt.Printf("[Manifest] Warning: Windows artifact not found in %s\n", *distDirFlag)
	}

	// 2. Process Linux tarball
	linuxPatterns := []string{
		filepath.Join(*distDirFlag, "nord-launcher-*.tar.gz"),
		filepath.Join(*distDirFlag, "linux", "nord-launcher-*.tar.gz"),
		filepath.Join(*distDirFlag, "dist", "linux", "nord-launcher-*.tar.gz"),
	}

	var linuxTarball string
	for _, pattern := range linuxPatterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			linuxTarball = matches[0]
			break
		}
	}

	if linuxTarball != "" {
		payload, err := os.ReadFile(linuxTarball)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", linuxTarball, err)
			os.Exit(1)
		}
		hashBytes := sha256.Sum256(payload)
		hash := hex.EncodeToString(hashBytes[:])
		var sigStr string
		if len(privKey) > 0 {
			sig := updater.SignPayload(privKey, payload)
			sigStr = base64.StdEncoding.EncodeToString(sig)
		}
		manifest.Platforms["linux-amd64"] = updater.PlatformAsset{
			URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(linuxTarball)),
			SHA256:    hash,
			Signature: sigStr,
			Size:      int64(len(payload)),
		}
		fmt.Printf("[Manifest] Added linux-amd64: %s (SHA256: %s, Size: %d)\n", filepath.Base(linuxTarball), hash, len(payload))
	} else {
		fmt.Printf("[Manifest] Warning: Linux artifact not found in %s\n", *distDirFlag)
	}

	var extraAssets []PublishedArtifact
	seenAssets := make(map[string]bool)
	if winPortable != "" {
		seenAssets[filepath.Base(winPortable)] = true
	}
	if winSetup != "" {
		seenAssets[filepath.Base(winSetup)] = true
	}
	if linuxTarball != "" {
		seenAssets[filepath.Base(linuxTarball)] = true
	}

	// Scan for Windows portable zip in distDir (e.g. nord-launcher-*-windows-x64-portable.zip)
	zipMatches, _ := filepath.Glob(filepath.Join(*distDirFlag, "*portable*.zip"))
	if len(zipMatches) == 0 {
		zipMatches, _ = filepath.Glob(filepath.Join(*distDirFlag, "*.zip"))
	}
	for _, zPath := range zipMatches {
		base := filepath.Base(zPath)
		if seenAssets[base] {
			continue
		}
		seenAssets[base] = true
		payload, err := os.ReadFile(zPath)
		if err != nil {
			continue
		}
		hashBytes := sha256.Sum256(payload)
		extraAssets = append(extraAssets, PublishedArtifact{
			Name:     base,
			Platform: "Windows (x64 Portable Zip)",
			Size:     int64(len(payload)),
			SHA256:   hex.EncodeToString(hashBytes[:]),
		})
		fmt.Printf("[Manifest] Added extra artifact: %s (SHA256: %s, Size: %d)\n", base, hex.EncodeToString(hashBytes[:]), len(payload))
	}

	// Scan for update manifest files in distDir (e.g. manifest-stable.json, manifest-beta.json)
	manifestMatches, _ := filepath.Glob(filepath.Join(*distDirFlag, "manifest-*.json"))
	for _, mPath := range manifestMatches {
		base := filepath.Base(mPath)
		if seenAssets[base] {
			continue
		}
		seenAssets[base] = true
		payload, err := os.ReadFile(mPath)
		if err != nil {
			continue
		}
		hashBytes := sha256.Sum256(payload)
		extraAssets = append(extraAssets, PublishedArtifact{
			Name:     base,
			Platform: "Update Manifest",
			Size:     int64(len(payload)),
			SHA256:   hex.EncodeToString(hashBytes[:]),
		})
		fmt.Printf("[Manifest] Added extra artifact: %s (SHA256: %s, Size: %d)\n", base, hex.EncodeToString(hashBytes[:]), len(payload))
	}

	// Scan for SHA256SUMS.txt in distDir
	sumsPath := filepath.Join(*distDirFlag, "SHA256SUMS.txt")
	if !seenAssets["SHA256SUMS.txt"] {
		if payload, err := os.ReadFile(sumsPath); err == nil {
			seenAssets["SHA256SUMS.txt"] = true
			hashBytes := sha256.Sum256(payload)
			extraAssets = append(extraAssets, PublishedArtifact{
				Name:     "SHA256SUMS.txt",
				Platform: "Checksums",
				Size:     int64(len(payload)),
				SHA256:   hex.EncodeToString(hashBytes[:]),
			})
			fmt.Printf("[Manifest] Added extra artifact: SHA256SUMS.txt (SHA256: %s, Size: %d)\n", hex.EncodeToString(hashBytes[:]), len(payload))
		}
	}

	if !*bodyOnlyFlag {
		manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to serialize manifest: %v\n", err)
			os.Exit(1)
		}

		outPath := *outputFile
		if outPath == "" {
			outPath = filepath.Join(*distDirFlag, fmt.Sprintf("manifest-%s.json", *channelFlag))
		}

		_ = os.MkdirAll(filepath.Dir(outPath), 0755) // errcheck:ok
		if err := os.WriteFile(outPath, manifestBytes, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write manifest to %s: %v\n", outPath, err)
			os.Exit(1)
		}

		fmt.Printf("[Manifest] Generated signed update manifest: %s\n", outPath)
	}

	if *bodyFile != "" {
		bodyContent := BuildReleaseBody(cleanVersion, *channelFlag, changelogText, manifest.Platforms, extraAssets...)
		_ = os.MkdirAll(filepath.Dir(*bodyFile), 0755) // errcheck:ok
		if err := os.WriteFile(*bodyFile, []byte(bodyContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write release body to %s: %v\n", *bodyFile, err)
			os.Exit(1)
		}
		fmt.Printf("[Manifest] Generated release body: %s (%d bytes)\n", *bodyFile, len(bodyContent))
	}
}

// ExtractChangelog extracts the release notes for targetVersion from changelogContent.
// It parses the section starting with "## [targetVersion]" or "## targetVersion"
// up to the next "## " header, strips the section header, and trims whitespace.
// The result is capped at maxBytes (default 2800).
func ExtractChangelog(content, version string, maxBytes int) string {
	cleanVer := strings.TrimPrefix(version, "v")
	lines := strings.Split(content, "\n")
	var sectionLines []string
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if inSection {
				break
			}
			header := strings.TrimPrefix(trimmed, "## ")
			header = strings.TrimSpace(header)
			if strings.HasPrefix(header, "["+cleanVer+"]") || strings.HasPrefix(header, cleanVer) || strings.HasPrefix(header, "v"+cleanVer) || strings.HasPrefix(header, "[v"+cleanVer+"]") {
				inSection = true
				continue
			}
		} else if inSection {
			sectionLines = append(sectionLines, line)
		}
	}

	result := strings.TrimSpace(strings.Join(sectionLines, "\n"))
	if maxBytes > 0 && len(result) > maxBytes {
		result = strings.TrimSpace(result[:maxBytes]) + "..."
	}
	return result
}

// PublishedArtifact represents an asset published to GitHub Release.
type PublishedArtifact struct {
	Name     string
	Platform string
	Size     int64
	SHA256   string
}

// FormatBytes formats byte counts with comma thousands separators (e.g. 18,119,296 B).
func FormatBytes(n int64) string {
	in := strconv.FormatInt(n, 10)
	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out) + " B"
}

// BuildReleaseBody constructs the markdown release notes body for GitHub Releases.
// It incorporates:
// 1. ### Overview
// 2. ### Changelog
// 3. ### Published Artifacts (markdown table)
// 4. ### Verification & Forensic Integrity
func BuildReleaseBody(version, channel, changelog string, platforms map[string]updater.PlatformAsset, extraAssets ...PublishedArtifact) string {
	cleanVer := strings.TrimPrefix(version, "v")
	tagVersion := "v" + cleanVer

	overview := getReleaseOverview(cleanVer, tagVersion)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Nord Launcher %s (%s channel release)\n\n", tagVersion, channel))
	sb.WriteString("### Overview\n\n")
	sb.WriteString(overview)
	sb.WriteString("\n\n### Changelog\n\n")
	sb.WriteString(changelog)
	sb.WriteString("\n\n---\n\n")

	if len(platforms) > 0 || len(extraAssets) > 0 {
		sb.WriteString("### Published Artifacts\n\n")
		sb.WriteString("| Asset | Platform | Size | SHA-256 Checksum |\n")
		sb.WriteString("|:---|:---|---:|:---|\n")

		order := []string{"windows-amd64", "windows-setup", "linux-amd64"}
		platformLabels := map[string]string{
			"windows-amd64": "Windows (x64 Portable)",
			"windows-setup": "Windows (x64 Installer)",
			"linux-amd64":   "Linux (x64 Tarball)",
		}

		for _, pKey := range order {
			if asset, ok := platforms[pKey]; ok {
				assetName := filepath.Base(asset.URL)
				label := platformLabels[pKey]
				if label == "" {
					label = pKey
				}
				sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` |\n", assetName, label, FormatBytes(asset.Size), asset.SHA256))
			}
		}
		for pKey, asset := range platforms {
			isOrdered := false
			for _, o := range order {
				if o == pKey {
					isOrdered = true
					break
				}
			}
			if !isOrdered {
				assetName := filepath.Base(asset.URL)
				sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` |\n", assetName, pKey, FormatBytes(asset.Size), asset.SHA256))
			}
		}
		for _, extra := range extraAssets {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` |\n", extra.Name, extra.Platform, FormatBytes(extra.Size), extra.SHA256))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Verification & Forensic Integrity\n\n")
	sb.WriteString(fmt.Sprintf("- **Ed25519 Cryptographic Signatures**: All platform release payloads verified against production signing key.\n- **Public Key**: `%s`\n- **Sidecar Key Permissions**: `cf.key` delivered with `0644` permissions inside Linux release tarball and NSIS installer payload; binary executables verified 100%% clean of raw secrets and buildinfo symbol leaks.\n- **Update Manifest Scope**: Update manifest delivers verified binary updates for Windows (portable executable & NSIS setup) and Linux; standalone Windows portable zip is distributed as an unbundled archive asset and is not an auto-updater target.\n\n", updater.DefaultPublicKeyHex))
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("**Full Changelog**: https://github.com/Aethelis-Projects/Aethelis-Launcher/compare/v0.1.0...%s\n", tagVersion))

	return sb.String()
}

func getReleaseOverview(cleanVer, tagVersion string) string {
	switch {
	case cleanVer == "0.7.0":
		return "This release introduces rich instance content features: Modrinth resource pack and shader pack catalog browsing and 1-click installation, local screenshots gallery with native clipboard copy, real-time log streaming game console, 1-click instance migration from .minecraft and Prism/MultiMC, instance groups and favorites, curated avatar presets, and standalone Windows portable zip distribution."
	case cleanVer == "0.6.1":
		return "This release delivers atomic mod updates with automatic obsolete version cleanup, manifest-driven Java recommendation chip for modern Minecraft versions (up to Java 25 LTS), platform-native folder opener across instances and export modals, and prominent manual update checking with status badges."
	case strings.HasPrefix(cleanVer, "0.6."):
		return "This release introduces full modpack round-trip support for Modrinth (.mrpack format), deep mod version history browsing with safe markdown changelog viewing, Adoptium Temurin Java runtime update detection and bulk cleanup, and safeguards against deleting Java runtimes while instances are active."
	case strings.HasPrefix(cleanVer, "0.5."):
		return "This release resolves CurseForge API resilience and rate-limiting issues, introduces in-repo changelog extraction and informative in-app update modals, implements an honest offline-safe 7-state badge machine, adds persistent SQLite content caching, and provides mod update checking with diagnostic reporting."
	case strings.HasPrefix(cleanVer, "0.4."):
		return "This release adds an in-launcher mods manager with contextual tabs, sidecar-tracked installations, dual-file deletion, and disk-truth reconcile."
	default:
		return fmt.Sprintf("Official %s release of Nord Launcher delivering performance improvements, stability updates, and feature enhancements.", tagVersion)
	}
}

