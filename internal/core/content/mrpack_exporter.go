package content

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/manifest"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// ExportMrPackOptions configures the modpack export operation.
type ExportMrPackOptions struct {
	InstanceID       string `json:"instance_id"`
	PackName         string `json:"pack_name,omitempty"`
	PackVersion      string `json:"pack_version,omitempty"`
	Summary          string `json:"summary,omitempty"`
	ExportPath       string `json:"export_path,omitempty"`
	IncludeShaders   bool   `json:"include_shaders,omitempty"`
	IncludeResources bool   `json:"include_resources,omitempty"`
}

// MrPackExporter handles packaging an installed game instance into a standard .mrpack archive.
type MrPackExporter struct {
	instRepo     ports.InstanceRepository
	contentCache ports.ContentCache
	instancesDir string
	exportDir    string
}

// NewMrPackExporter creates a new MrPackExporter instance.
func NewMrPackExporter(
	instRepo ports.InstanceRepository,
	contentCache ports.ContentCache,
	instancesDir, exportDir string,
) *MrPackExporter {
	if instancesDir == "" {
		instancesDir = "instances"
	}
	if exportDir == "" {
		exportDir = "exports"
	}
	return &MrPackExporter{
		instRepo:     instRepo,
		contentCache: contentCache,
		instancesDir: instancesDir,
		exportDir:    exportDir,
	}
}

func sanitizeFileName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' {
			sb.WriteRune('-')
		}
	}
	res := strings.Trim(sb.String(), "-_")
	if res == "" {
		return "modpack"
	}
	return res
}

func isBlacklistedExportFile(relPath string) bool {
	norm := filepath.ToSlash(strings.ToLower(relPath))
	parts := strings.Split(norm, "/")

	for _, p := range parts {
		if p == "logs" || p == "crash-reports" || p == "screenshots" || p == "saves" || p == "natives" {
			return true
		}
	}

	base := filepath.Base(norm)
	if base == "nord-launcher.log" || base == "nord-installs.json" {
		return true
	}
	if strings.HasSuffix(base, ".log") || strings.HasSuffix(base, ".part") || strings.HasSuffix(base, ".tmp") || strings.HasSuffix(base, ".bak") {
		return true
	}
	if strings.HasSuffix(base, ".disabled") {
		return true
	}

	return false
}

// ExportMrPack builds a standard .mrpack archive from an existing instance.
func (m *MrPackExporter) ExportMrPack(ctx context.Context, opts ExportMrPackOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if opts.InstanceID == "" {
		return "", fmt.Errorf("instance id cannot be empty")
	}

	if m.instRepo == nil {
		return "", fmt.Errorf("instance repository is required")
	}

	inst, err := m.instRepo.GetByID(ctx, opts.InstanceID)
	if err != nil {
		return "", fmt.Errorf("get instance %s: %w", opts.InstanceID, err)
	}

	instDir := filepath.Join(m.instancesDir, inst.ID)
	if _, err := os.Stat(instDir); err != nil {
		return "", fmt.Errorf("instance directory does not exist: %w", err)
	}

	packName := opts.PackName
	if packName == "" {
		packName = inst.Name
	}
	packVersion := opts.PackVersion
	if packVersion == "" {
		packVersion = "1.0.0"
	}

	deps := make(map[string]string)
	deps["minecraft"] = inst.GameVersion

	loaderVer := inst.LoaderVer
	if loaderVer == "" {
		loaderVer = "latest"
	}

	switch inst.Loader {
	case domain.LoaderFabric:
		deps["fabric-loader"] = loaderVer
	case domain.LoaderQuilt:
		deps["quilt-loader"] = loaderVer
	case domain.LoaderForge:
		deps["forge"] = loaderVer
	case domain.LoaderNeoForge:
		deps["neoforge"] = loaderVer
	}

	modsDir := filepath.Join(instDir, "mods")
	var instManifest *manifest.InstallsManifest
	if _, err := os.Stat(modsDir); err == nil {
		instManifest, _ = manifest.LoadManifest(modsDir)
	}

	type overrideItem struct {
		relPath  string
		fullPath string
	}
	var overrides []overrideItem
	var indexFiles []MrPackFile

	if entries, err := os.ReadDir(modsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fileName := e.Name()
			if !strings.HasSuffix(strings.ToLower(fileName), ".jar") {
				continue
			}
			if strings.HasSuffix(strings.ToLower(fileName), ".disabled") {
				continue
			}

			filePath := filepath.Join(modsDir, fileName)
			fi, statErr := os.Stat(filePath)
			if statErr != nil {
				continue
			}

			s1, sha1Err := CalculateFileSHA1(filePath)
			if sha1Err != nil {
				continue
			}
			s512, sha512Err := CalculateFileSHA512(filePath)
			if sha512Err != nil {
				continue
			}

			downloadURL := ""
			if instManifest != nil {
				rec := instManifest.GetRecord(fileName)
				if rec != nil && rec.Source == "modrinth" && rec.ModID != "" && rec.VersionID != "" {
					downloadURL = fmt.Sprintf("https://cdn.modrinth.com/data/%s/versions/%s/%s", rec.ModID, rec.VersionID, fileName)
				}
			}

			if downloadURL == "" && m.contentCache != nil {
				if payload, _, ok, _ := m.contentCache.Get(ctx, "mod_hash", s1); ok && payload != "" {
					downloadURL = payload
				}
			}

			if downloadURL != "" {
				indexFiles = append(indexFiles, MrPackFile{
					Path: "mods/" + fileName,
					Hashes: map[string]string{
						"sha1":   s1,
						"sha512": s512,
					},
					Env: &MrPackEnv{
						Client: "required",
						Server: "required",
					},
					Downloads: []string{downloadURL},
					FileSize:  fi.Size(),
				})
			} else {
				// Local or untracked mod: package into overrides/mods/
				overrides = append(overrides, overrideItem{
					relPath:  filepath.Join("mods", fileName),
					fullPath: filePath,
				})
			}
		}
	}

	// Whitelist copy of config/
	configDir := filepath.Join(instDir, "config")
	if _, err := os.Stat(configDir); err == nil {
		_ = filepath.WalkDir(configDir, func(path string, d fs.DirEntry, walkErr error) error { // errcheck:ok best effort walk
			if walkErr != nil || d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(instDir, path)
			if err != nil || isBlacklistedExportFile(rel) {
				return nil
			}
			overrides = append(overrides, overrideItem{
				relPath:  rel,
				fullPath: path,
			})
			return nil
		})
	}

	// Optional resourcepacks/
	resDir := filepath.Join(instDir, "resourcepacks")
	if opts.IncludeResources {
		if _, err := os.Stat(resDir); err == nil {
			_ = filepath.WalkDir(resDir, func(path string, d fs.DirEntry, walkErr error) error { // errcheck:ok best effort walk
				if walkErr != nil || d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(instDir, path)
				if err != nil || isBlacklistedExportFile(rel) {
					return nil
				}
				overrides = append(overrides, overrideItem{
					relPath:  rel,
					fullPath: path,
				})
				return nil
			})
		}
	}

	// Optional shaderpacks/
	shaderDir := filepath.Join(instDir, "shaderpacks")
	if opts.IncludeShaders {
		if _, err := os.Stat(shaderDir); err == nil {
			_ = filepath.WalkDir(shaderDir, func(path string, d fs.DirEntry, walkErr error) error { // errcheck:ok best effort walk
				if walkErr != nil || d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(instDir, path)
				if err != nil || isBlacklistedExportFile(rel) {
					return nil
				}
				overrides = append(overrides, overrideItem{
					relPath:  rel,
					fullPath: path,
				})
				return nil
			})
		}
	}

	exportPath := opts.ExportPath
	if exportPath == "" {
		safeName := sanitizeFileName(packName)
		exportPath = filepath.Join(m.exportDir, fmt.Sprintf("%s-%d.mrpack", safeName, time.Now().Unix()))
	}

	if err := os.MkdirAll(filepath.Dir(exportPath), 0755); err != nil {
		return "", fmt.Errorf("create export directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(exportPath), ".mrpack-export-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp export file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok safe to ignore close error if already closed
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath) // errcheck:ok cleanup temp on failure
		}
	}()

	zw := zip.NewWriter(tempFile)

	index := MrPackIndex{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     packVersion,
		Name:          packName,
		Summary:       opts.Summary,
		Files:         indexFiles,
		Dependencies:  deps,
	}

	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal modrinth.index.json: %w", err)
	}

	wIndex, err := zw.Create("modrinth.index.json")
	if err != nil {
		return "", fmt.Errorf("create modrinth.index.json in zip: %w", err)
	}
	if _, err := wIndex.Write(indexData); err != nil {
		return "", fmt.Errorf("write modrinth.index.json: %w", err)
	}

	for _, item := range overrides {
		if isBlacklistedExportFile(item.relPath) {
			continue
		}

		zipEntryPath := "overrides/" + filepath.ToSlash(item.relPath)
		wEntry, err := zw.Create(zipEntryPath)
		if err != nil {
			return "", fmt.Errorf("create zip entry %s: %w", zipEntryPath, err)
		}

		fSrc, err := os.Open(item.fullPath)
		if err != nil {
			continue
		}

		_, copyErr := io.Copy(wEntry, fSrc)
		_ = fSrc.Close() // errcheck:ok safe close of source file
		if copyErr != nil {
			return "", fmt.Errorf("write zip entry %s: %w", zipEntryPath, copyErr)
		}
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("close zip writer: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close temp export file: %w", err)
	}

	if _, err := os.Stat(exportPath); err == nil {
		_ = os.Remove(exportPath) // errcheck:ok remove existing before rename on Windows
	}

	if err := os.Rename(tempPath, exportPath); err != nil {
		return "", fmt.Errorf("finalize export file rename: %w", err)
	}

	return exportPath, nil
}
