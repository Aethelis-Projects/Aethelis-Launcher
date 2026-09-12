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
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/updater"
)

// DefaultStagingPrivateKeyHex is used for local builds/tests when ED25519_PRIVATE_KEY is not set.
const DefaultStagingPrivateKeyHex = "88ec59f652844aded5ef72635fd0621042ffff0b75ec7c0e20185255b374f9af"

func main() {
	var (
		versionFlag   = flag.String("version", "0.1.0", "Release version")
		channelFlag   = flag.String("channel", "stable", "Release channel: stable or beta")
		distDirFlag   = flag.String("dist", "dist", "Directory containing build distribution artifacts")
		outputFile          = flag.String("out", "", "Output manifest path (default: dist/manifest-{channel}.json)")
		privKeyEnv          = flag.String("privkey-hex", "", "Hex-encoded Ed25519 private key (optional, falls back to ED25519_PRIVATE_KEY)")
		allowInsecureDevKey = flag.Bool("allow-insecure-dev-key", false, "Allow fallback to insecure hardcoded staging private key for local development")
	)
	flag.Parse()

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
	privKey := ed25519.NewKeyFromSeed(privSeed)

	rawVersion := *versionFlag
	cleanVersion := strings.TrimPrefix(rawVersion, "v")
	tagVersion := "v" + cleanVersion

	manifest := updater.UpdateManifest{
		Version:     cleanVersion,
		ReleaseDate: time.Now().UTC(),
		Changelog:   fmt.Sprintf("Nord Launcher %s (%s channel) release.", tagVersion, *channelFlag),
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
		if fi, err := os.Stat(winPortable); err == nil {
			hash, err := calcSHA256(winPortable)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to hash %s: %v\n", winPortable, err)
				os.Exit(1)
			}
			sig := ed25519.Sign(privKey, []byte(hash))
			manifest.Platforms["windows-amd64"] = updater.PlatformAsset{
				URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(winPortable)),
				SHA256:    hash,
				Signature: base64.StdEncoding.EncodeToString(sig),
				Size:      fi.Size(),
			}
			fmt.Printf("[Manifest] Added windows-amd64: %s (SHA256: %s)\n", filepath.Base(winPortable), hash)
		}
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
		if fi, err := os.Stat(winSetup); err == nil {
			hash, err := calcSHA256(winSetup)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to hash %s: %v\n", winSetup, err)
				os.Exit(1)
			}
			sig := ed25519.Sign(privKey, []byte(hash))
			manifest.Platforms["windows-setup"] = updater.PlatformAsset{
				URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(winSetup)),
				SHA256:    hash,
				Signature: base64.StdEncoding.EncodeToString(sig),
				Size:      fi.Size(),
			}
			fmt.Printf("[Manifest] Added windows-setup: %s (SHA256: %s)\n", filepath.Base(winSetup), hash)
		}
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
		if fi, err := os.Stat(linuxTarball); err == nil {
			hash, err := calcSHA256(linuxTarball)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to hash %s: %v\n", linuxTarball, err)
				os.Exit(1)
			}
			sig := ed25519.Sign(privKey, []byte(hash))
			manifest.Platforms["linux-amd64"] = updater.PlatformAsset{
				URL:       fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/download/%s/%s", tagVersion, filepath.Base(linuxTarball)),
				SHA256:    hash,
				Signature: base64.StdEncoding.EncodeToString(sig),
				Size:      fi.Size(),
			}
			fmt.Printf("[Manifest] Added linux-amd64: %s (SHA256: %s)\n", filepath.Base(linuxTarball), hash)
		}
	} else {
		fmt.Printf("[Manifest] Warning: Linux artifact not found in %s\n", *distDirFlag)
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to serialize manifest: %v\n", err)
		os.Exit(1)
	}

	outPath := *outputFile
	if outPath == "" {
		outPath = filepath.Join(*distDirFlag, fmt.Sprintf("manifest-%s.json", *channelFlag))
	}

	_ = os.MkdirAll(filepath.Dir(outPath), 0755)
	if err := os.WriteFile(outPath, manifestBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write manifest to %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("[Manifest] Generated signed update manifest: %s\n", outPath)
}

func calcSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
