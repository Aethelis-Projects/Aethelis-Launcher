package game

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

type ServiceOption func(*GameService)

// WithManifestURL sets an alternative Mojang version manifest URL.
func WithManifestURL(url string) ServiceOption {
	return func(s *GameService) {
		s.manifestURL = url
	}
}

// WithLibrariesBaseURL sets an alternative Maven libraries base URL.
func WithLibrariesBaseURL(url string) ServiceOption {
	return func(s *GameService) {
		s.librariesBaseURL = url
	}
}

// WithResourcesBaseURL sets an alternative Minecraft resources base URL.
func WithResourcesBaseURL(url string) ServiceOption {
	return func(s *GameService) {
		s.resourcesBaseURL = url
	}
}

// WithPlatform sets explicit OS and architecture strings for testing.
func WithPlatform(osName, archName string) ServiceOption {
	return func(s *GameService) {
		s.currentOS = osName
		s.currentArch = archName
	}
}

// GameService implements ports.GameProvisioner for resolving and downloading game assets.
type GameService struct {
	http             ports.HTTPClient
	fs               ports.FileSystem
	dataDir          string
	manifestURL      string
	librariesBaseURL string
	resourcesBaseURL string
	currentOS        string
	currentArch      string
}

// NewGameService creates a new GameService instance.
func NewGameService(
	http ports.HTTPClient,
	fs ports.FileSystem,
	dataDir string,
	opts ...ServiceOption,
) *GameService {
	s := &GameService{
		http:             http,
		fs:               fs,
		dataDir:          dataDir,
		manifestURL:      PistonMetaManifestURL,
		librariesBaseURL: MojangLibrariesURL,
		resourcesBaseURL: MojangResourcesURL,
		currentOS:        runtime.GOOS,
		currentArch:      runtime.GOARCH,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// MavenCoordinatesToPath converts group:name:version[:classifier][@ext] to a relative file path.
func MavenCoordinatesToPath(coords string) string {
	ext := "jar"
	if atIdx := strings.Index(coords, "@"); atIdx != -1 {
		ext = coords[atIdx+1:]
		coords = coords[:atIdx]
	}

	parts := strings.Split(coords, ":")
	if len(parts) < 3 {
		return coords + "." + ext
	}

	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact := parts[1]
	version := parts[2]

	filename := fmt.Sprintf("%s-%s", artifact, version)
	if len(parts) >= 4 && parts[3] != "" {
		filename += fmt.Sprintf("-%s", parts[3])
	}
	filename += "." + ext

	return fmt.Sprintf("%s/%s/%s/%s", group, artifact, version, filename)
}

func (s *GameService) verifyFileSHA1(filePath, expectedSHA1 string) bool {
	if !s.fs.Exists(filePath) {
		return false
	}
	if expectedSHA1 == "" {
		return true
	}
	data, err := s.fs.ReadFile(filePath)
	if err != nil {
		return false
	}
	hasher := sha1.New()
	hasher.Write(data)
	actual := hex.EncodeToString(hasher.Sum(nil))
	return strings.EqualFold(actual, expectedSHA1)
}

// Provision executes the 6-step game download and validation pipeline.
func (s *GameService) Provision(
	ctx context.Context,
	inst *domain.Instance,
	acc *domain.Account,
) (*domain.LaunchConfig, error) {
	if inst == nil {
		return nil, fmt.Errorf("%w: instance cannot be nil", domain.ErrInvalidConfig)
	}
	if acc == nil {
		return nil, domain.ErrNoActiveAccount
	}

	// 1. Resolve version from manifest catalog
	manifest, err := FetchVersionManifest(ctx, s.http, s.manifestURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDownloadFailed, err)
	}

	entry, err := FindVersionEntry(manifest, inst.GameVersion)
	if err != nil {
		return nil, err
	}

	versionDir := filepath.Join(s.dataDir, "versions", inst.GameVersion)
	if err := s.fs.MkdirAll(versionDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir versions dir: %w", err)
	}
	versionFilePath := filepath.Join(versionDir, inst.GameVersion+".json")

	if !s.verifyFileSHA1(versionFilePath, entry.SHA1) {
		if err := s.http.DownloadFile(ctx, entry.URL, versionFilePath, entry.SHA1, nil); err != nil {
			return nil, fmt.Errorf("%w: download version metadata: %v", domain.ErrDownloadFailed, err)
		}
	}

	versionBytes, err := s.fs.ReadFile(versionFilePath)
	if err != nil {
		return nil, fmt.Errorf("read version json: %w", err)
	}

	var versionMeta domain.VersionJSON
	if err := json.Unmarshal(versionBytes, &versionMeta); err != nil {
		return nil, fmt.Errorf("unmarshal version json: %w", err)
	}

	// 2. Download client.jar
	clientJarPath := filepath.Join(versionDir, inst.GameVersion+".jar")
	if versionMeta.Downloads.Client != nil && versionMeta.Downloads.Client.URL != "" {
		if !s.verifyFileSHA1(clientJarPath, versionMeta.Downloads.Client.SHA1) {
			if err := s.http.DownloadFile(ctx, versionMeta.Downloads.Client.URL, clientJarPath, versionMeta.Downloads.Client.SHA1, nil); err != nil {
				return nil, fmt.Errorf("%w: download client jar: %v", domain.ErrDownloadFailed, err)
			}
		}
	}

	// 3. Resolve & download libraries, extract natives
	librariesDir := filepath.Join(s.dataDir, "libraries")
	nativesDir := filepath.Join(s.dataDir, "instances", inst.ID, "natives")
	var libraryJarList []string

	for _, lib := range versionMeta.Libraries {
		if lib.ServerReq {
			continue
		}
		if !domain.EvaluateRules(lib.Rules, s.currentOS, s.currentArch, nil) {
			continue
		}

		// Download artifact
		var relPath, url, expectedSHA1 string
		if lib.Downloads.Artifact != nil && lib.Downloads.Artifact.URL != "" {
			relPath = lib.Downloads.Artifact.Path
			url = lib.Downloads.Artifact.URL
			expectedSHA1 = lib.Downloads.Artifact.SHA1
		} else if len(lib.Natives) == 0 && lib.Name != "" {
			relPath = MavenCoordinatesToPath(lib.Name)
			url = fmt.Sprintf("%s/%s", strings.TrimRight(s.librariesBaseURL, "/"), relPath)
		}

		if relPath != "" {
			fullPath := filepath.Join(librariesDir, filepath.FromSlash(relPath))
			if !s.verifyFileSHA1(fullPath, expectedSHA1) {
				if err := s.fs.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					return nil, fmt.Errorf("mkdir library dir: %w", err)
				}
				if err := s.http.DownloadFile(ctx, url, fullPath, expectedSHA1, nil); err != nil {
					return nil, fmt.Errorf("%w: download library %s: %v", domain.ErrDownloadFailed, lib.Name, err)
				}
			}
			libraryJarList = append(libraryJarList, fullPath)
		}

		// Check natives classifier
		if len(lib.Natives) > 0 {
			osKey := s.currentOS
			if osKey == "darwin" {
				osKey = "osx"
			}
			if classifierTemplate, ok := lib.Natives[osKey]; ok {
				archKey := "64"
				if s.currentArch == "386" {
					archKey = "32"
				}
				classifier := strings.ReplaceAll(classifierTemplate, "${arch}", archKey)
				if nativeArt, ok := lib.Downloads.Classifiers[classifier]; ok && nativeArt.URL != "" {
					nativePath := filepath.Join(librariesDir, filepath.FromSlash(nativeArt.Path))
					if !s.verifyFileSHA1(nativePath, nativeArt.SHA1) {
						if err := s.fs.MkdirAll(filepath.Dir(nativePath), 0755); err != nil {
							return nil, fmt.Errorf("mkdir native lib dir: %w", err)
						}
						if err := s.http.DownloadFile(ctx, nativeArt.URL, nativePath, nativeArt.SHA1, nil); err != nil {
							return nil, fmt.Errorf("%w: download native %s: %v", domain.ErrDownloadFailed, classifier, err)
						}
					}
					_ = s.extractNatives(nativePath, nativesDir) // slop:ok best-effort unpack of platform native libraries
				}
			}
		}
	}

	// 4. Resolve & download assets
	assetsDir := filepath.Join(s.dataDir, "assets")
	if versionMeta.AssetIndex.URL != "" {
		indexPath := filepath.Join(assetsDir, "indexes", versionMeta.AssetIndex.ID+".json")
		if !s.verifyFileSHA1(indexPath, versionMeta.AssetIndex.SHA1) {
			if err := s.fs.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
				return nil, fmt.Errorf("mkdir asset index dir: %w", err)
			}
			if err := s.http.DownloadFile(ctx, versionMeta.AssetIndex.URL, indexPath, versionMeta.AssetIndex.SHA1, nil); err != nil {
				return nil, fmt.Errorf("%w: download asset index: %v", domain.ErrDownloadFailed, err)
			}
		}

		indexBytes, err := s.fs.ReadFile(indexPath)
		if err == nil {
			var assetIndex AssetIndex
			if err := json.Unmarshal(indexBytes, &assetIndex); err == nil {
				for _, obj := range assetIndex.Objects {
					if len(obj.Hash) < 2 {
						continue
					}
					sub := obj.Hash[:2]
					objPath := filepath.Join(assetsDir, "objects", sub, obj.Hash)
					objURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.resourcesBaseURL, "/"), sub, obj.Hash)
					if !s.verifyFileSHA1(objPath, obj.Hash) {
						if err := s.fs.MkdirAll(filepath.Dir(objPath), 0755); err == nil {
							_ = s.http.DownloadFile(ctx, objURL, objPath, obj.Hash, nil) // slop:ok non-fatal individual asset object download
						}
					}
				}
			}
		}
	}

	// 5. Assemble complete LaunchConfig
	instDir := filepath.Join(s.dataDir, "instances", inst.ID)
	if err := s.fs.MkdirAll(instDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir instance dir: %w", err)
	}

	return &domain.LaunchConfig{
		Instance:       inst,
		Account:        acc,
		VersionMeta:    &versionMeta,
		GameDir:        instDir,
		AssetsDir:      assetsDir,
		LibrariesDir:   librariesDir,
		NativesDir:     nativesDir,
		ClientJarPath:  clientJarPath,
		LibraryJarList: libraryJarList,
		ResolutionW:    854,
		ResolutionH:    480,
		IsDemo:         false,
	}, nil
}

func (s *GameService) extractNatives(jarPath, destDir string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("open native jar: %w", err)
	}
	defer r.Close()

	if err := s.fs.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("mkdir natives dir: %w", err)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(f.Name, "META-INF") || strings.HasPrefix(f.Name, ".") {
			continue
		}
		destFile := filepath.Join(destDir, filepath.Base(f.Name))
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		if err := s.fs.WriteFile(destFile, data, 0755); err != nil {
			return fmt.Errorf("write extracted native %s: %w", destFile, err)
		}
	}
	return nil
}
