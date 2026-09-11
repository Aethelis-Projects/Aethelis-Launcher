package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNoUpdateAvailable   = errors.New("no update available")
	ErrSignatureInvalid     = errors.New("cryptographic signature verification failed")
	ErrChecksumMismatch    = errors.New("update payload sha256 checksum mismatch")
	ErrUnsupportedPlatform = errors.New("current platform is not supported in update manifest")
)

type PlatformAsset struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"` // base64 encoded Ed25519 signature
	Size      int64  `json:"size"`
}

type UpdateManifest struct {
	Version     string                   `json:"version"`
	ReleaseDate time.Time                `json:"release_date"`
	Changelog   string                   `json:"changelog"`
	Platforms   map[string]PlatformAsset `json:"platforms"`
}

type UpdateInfo struct {
	Available   bool           `json:"available"`
	Version     string         `json:"version"`
	CurrentVer  string         `json:"current_version"`
	Changelog   string         `json:"changelog"`
	Asset       PlatformAsset  `json:"asset"`
}

type AutoUpdater struct {
	currentVersion string
	manifestURL    string
	publicKey      ed25519.PublicKey
	httpClient     *http.Client
}

func NewAutoUpdater(currentVersion, manifestURL string, publicKey ed25519.PublicKey, client *http.Client) *AutoUpdater {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &AutoUpdater{
		currentVersion: currentVersion,
		manifestURL:    manifestURL,
		publicKey:      publicKey,
		httpClient:     client,
	}
}

// CurrentPlatformKey returns platform key e.g. "windows-amd64" or "linux-amd64".
func CurrentPlatformKey() string {
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
}

// CheckForUpdates fetches manifest and evaluates if a newer version exists.
func (u *AutoUpdater) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create update check request: %w", err)
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute update check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update manifest returned HTTP %d", resp.StatusCode)
	}

	var manifest UpdateManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode update manifest: %w", err)
	}

	if !isVersionNewer(u.currentVersion, manifest.Version) {
		return &UpdateInfo{
			Available:  false,
			CurrentVer: u.currentVersion,
			Version:    manifest.Version,
		}, nil
	}

	platKey := CurrentPlatformKey()
	asset, exists := manifest.Platforms[platKey]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, platKey)
	}

	return &UpdateInfo{
		Available:  true,
		Version:    manifest.Version,
		CurrentVer: u.currentVersion,
		Changelog:  manifest.Changelog,
		Asset:      asset,
	}, nil
}

// DownloadAndApply downloads the update binary, validates its SHA256 & Ed25519 signature,
// and stages an atomic replacement of the running executable.
func (u *AutoUpdater) DownloadAndApply(ctx context.Context, asset PlatformAsset, currentExePath string) error {
	if currentExePath == "" {
		var err error
		currentExePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("determine current executable: %w", err)
		}
	}
	currentExePath, _ = filepath.EvalSymlinks(currentExePath)

	newExePath := currentExePath + ".new"
	oldExePath := currentExePath + ".old"

	// 1. Download payload to temporary .new file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download update returned HTTP %d", resp.StatusCode)
	}

	outFile, err := os.OpenFile(newExePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("create update file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	payloadBytes, err := io.ReadAll(io.TeeReader(resp.Body, writer))
	outFile.Close()
	if err != nil {
		_ = os.Remove(newExePath)
		return fmt.Errorf("read update payload: %w", err)
	}

	// 2. Verify SHA-256
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if asset.SHA256 != "" && !strings.EqualFold(actualSHA256, asset.SHA256) {
		_ = os.Remove(newExePath)
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, asset.SHA256, actualSHA256)
	}

	// 3. Verify Ed25519 signature
	if len(u.publicKey) > 0 && asset.Signature != "" {
		sigBytes, err := base64.StdEncoding.DecodeString(asset.Signature)
		if err != nil {
			_ = os.Remove(newExePath)
			return fmt.Errorf("decode signature: %w", err)
		}

		if !ed25519.Verify(u.publicKey, payloadBytes, sigBytes) {
			_ = os.Remove(newExePath)
			return ErrSignatureInvalid
		}
	}

	// 4. Atomic replacement (Windows and Unix compatible)
	// On Windows, a running executable can be renamed, but not overwritten or deleted.
	_ = os.Remove(oldExePath) // Clean up any stale previous backup
	if err := os.Rename(currentExePath, oldExePath); err != nil {
		_ = os.Remove(newExePath)
		return fmt.Errorf("backup running executable: %w", err)
	}

	if err := os.Rename(newExePath, currentExePath); err != nil {
		// Rollback
		_ = os.Rename(oldExePath, currentExePath)
		return fmt.Errorf("stage new executable: %w", err)
	}

	return nil
}

// CleanupStaleBackup removes <exe>.old left over from previous updates.
func CleanupStaleBackup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := exe + ".old"
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.Remove(oldPath)
	}
}

// isVersionNewer compares two SemVer versions (e.g., "0.1.0" vs "0.2.0").
func isVersionNewer(current, remote string) bool {
	cParts := parseSemVer(current)
	rParts := parseSemVer(remote)

	for i := 0; i < 3; i++ {
		if rParts[i] > cParts[i] {
			return true
		}
		if rParts[i] < cParts[i] {
			return false
		}
	}
	return false
}

func parseSemVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var res [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		clean := strings.Split(parts[i], "-")[0]
		res[i], _ = strconv.Atoi(clean)
	}
	return res
}