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
	"os/exec"
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

// DefaultPublicKeyHex is the official release manifest Ed25519 public key.
const DefaultPublicKeyHex = "c11aa844849500fb8bc9d6dd006ee3fcafafecac638ae3fedcce6a4553be0af3"

// GetDefaultPublicKey returns the decoded Ed25519 public key.
func GetDefaultPublicKey() ed25519.PublicKey {
	b, err := hex.DecodeString(DefaultPublicKeyHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("invalid default ed25519 public key")
	}
	return ed25519.PublicKey(b)
}

// SignPayload signs the raw binary payload using Ed25519.
// The signature domain is strictly the exact binary bytes of the target artifact.
func SignPayload(privKey ed25519.PrivateKey, payload []byte) []byte {
	return ed25519.Sign(privKey, payload)
}

// VerifyPayload verifies the Ed25519 signature against raw binary payload bytes.
func VerifyPayload(pubKey ed25519.PublicKey, payload []byte, sig []byte) bool {
	return ed25519.Verify(pubKey, payload, sig)
}

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
	ReleaseDate time.Time      `json:"release_date"`
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

// CurrentVersion returns the currently configured version string.
func (u *AutoUpdater) CurrentVersion() string {
	if u == nil {
		return ""
	}
	return u.currentVersion
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
			Available:   false,
			CurrentVer:  u.currentVersion,
			Version:     manifest.Version,
			ReleaseDate: manifest.ReleaseDate,
		}, nil
	}

	platKey := CurrentPlatformKey()
	asset, exists := manifest.Platforms[platKey]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatform, platKey)
	}

	return &UpdateInfo{
		Available:   true,
		Version:     manifest.Version,
		CurrentVer:  u.currentVersion,
		ReleaseDate: manifest.ReleaseDate,
		Changelog:   manifest.Changelog,
		Asset:       asset,
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
		_ = os.Remove(newExePath) // errcheck:ok cleanup failed download target
		return fmt.Errorf("read update payload: %w", err)
	}

	// 2. Verify SHA-256
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if asset.SHA256 != "" && !strings.EqualFold(actualSHA256, asset.SHA256) {
		_ = os.Remove(newExePath) // errcheck:ok cleanup failed download target
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, asset.SHA256, actualSHA256)
	}

	// 3. Verify Ed25519 signature
	if len(u.publicKey) > 0 && asset.Signature != "" {
		sigBytes, err := base64.StdEncoding.DecodeString(asset.Signature)
		if err != nil {
			_ = os.Remove(newExePath) // errcheck:ok cleanup failed download target
			return fmt.Errorf("decode signature: %w", err)
		}

		if !VerifyPayload(u.publicKey, payloadBytes, sigBytes) {
			_ = os.Remove(newExePath) // errcheck:ok cleanup failed download target
			return ErrSignatureInvalid
		}
	}

	// 4. Atomic replacement (Windows and Unix compatible)
	// On Windows, a running executable can be renamed, but not overwritten or deleted.
	_ = os.Remove(oldExePath) // errcheck:ok clean up any stale previous backup
	if err := os.Rename(currentExePath, oldExePath); err != nil {
		_ = os.Remove(newExePath) // errcheck:ok cleanup failed download target
		return fmt.Errorf("backup running executable: %w", err)
	}

	if err := os.Rename(newExePath, currentExePath); err != nil {
		// Rollback
		_ = os.Rename(oldExePath, currentExePath) // errcheck:ok best-effort rollback to original binary
		return fmt.Errorf("stage new executable: %w", err)
	}

	return nil
}

// ApplyUpdate downloads the asset for an update info and stages replacement.
func (u *AutoUpdater) ApplyUpdate(ctx context.Context, info *UpdateInfo) error {
	if info == nil || !info.Available {
		return ErrNoUpdateAvailable
	}
	return u.DownloadAndApply(ctx, info.Asset, "")
}

// RelauncherFunc defines the signature for application restart functions.
type RelauncherFunc func() error

// DefaultRelauncher is the standard relaunch implementation using os/exec and os.Exit(0).
var DefaultRelauncher RelauncherFunc = defaultRelaunch

func defaultRelaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determine executable: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch executable: %w", err)
	}
	os.Exit(0)
	return nil
}

// Relaunch restarts the current application executable using DefaultRelauncher.
func Relaunch() error {
	return DefaultRelauncher()
}

// CleanupStaleBackup removes <exe>.old left over from previous updates.
func CleanupStaleBackup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := exe + ".old"
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.Remove(oldPath) // errcheck:ok best-effort stale backup cleanup
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