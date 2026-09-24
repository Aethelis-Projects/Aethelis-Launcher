package content

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxMrPackSizeBytes sets a strict 512 MB cap on .mrpack archives to prevent decompression bombs and memory exhaustion.
	MaxMrPackSizeBytes int64 = 512 * 1024 * 1024
)

var (
	ErrInvalidMrPack   = errors.New("invalid .mrpack file: missing modrinth.index.json")
	ErrUnsupportedGame = errors.New("unsupported game in modpack")
)

type MrPackEnv struct {
	Client string `json:"client,omitempty"` // "required", "optional", "unsupported"
	Server string `json:"server,omitempty"`
}

type MrPackIndex struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary"`
	Files         []MrPackFile      `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type MrPackFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       *MrPackEnv        `json:"env,omitempty"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

type MrPackImportPlan struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Summary       string   `json:"summary"`
	GameVersion   string   `json:"game_version"`
	Loader        string   `json:"loader"`
	LoaderVersion string   `json:"loader_version"`
	TotalFiles    int      `json:"total_files"`
	TotalBytes    int64    `json:"total_bytes"`
	RequiredFiles int      `json:"required_files"`
	OptionalFiles int      `json:"optional_files"`
	Conflicts     []string `json:"conflicts"`
}

// SafeRelPath validates that untrustedRelPath resolves strictly within baseDir, preventing Zip-Slip (R1).
func SafeRelPath(baseDir, untrustedRelPath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	normalizedRel := strings.ReplaceAll(untrustedRelPath, "\\", "/")
	joined := filepath.Join(cleanBase, filepath.FromSlash(normalizedRel))
	cleanTarget := filepath.Clean(joined)

	prefix := cleanBase + string(filepath.Separator)
	if cleanTarget != cleanBase && !strings.HasPrefix(cleanTarget, prefix) {
		return "", fmt.Errorf("security error: path traversal detected: %q resolves outside destination", untrustedRelPath)
	}
	return cleanTarget, nil
}

// ParseMrPack reads and parses modrinth.index.json from a .mrpack ZIP archive.
func ParseMrPack(r io.ReaderAt, size int64) (*MrPackIndex, error) {
	if size > MaxMrPackSizeBytes {
		return nil, fmt.Errorf("mrpack archive size (%d bytes) exceeds maximum limit (%d bytes)", size, MaxMrPackSizeBytes)
	}

	var magic [4]byte
	if _, err := r.ReadAt(magic[:], 0); err != nil || magic != [4]byte{0x50, 0x4B, 0x03, 0x04} {
		return nil, fmt.Errorf("invalid archive: missing zip magic header")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open zip archive: %w", err)
	}

	var indexFile *zip.File
	for _, f := range zr.File {
		if f.Name == "modrinth.index.json" {
			indexFile = f
			break
		}
	}

	if indexFile == nil {
		return nil, ErrInvalidMrPack
	}

	rc, err := indexFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open modrinth.index.json: %w", err)
	}
	defer rc.Close()

	var index MrPackIndex
	if err := json.NewDecoder(rc).Decode(&index); err != nil {
		return nil, fmt.Errorf("decode modrinth.index.json: %w", err)
	}

	if index.FormatVersion != 1 {
		return nil, fmt.Errorf("unsupported .mrpack format version %d (only version 1 is supported)", index.FormatVersion)
	}

	if index.Game != "" && index.Game != "minecraft" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedGame, index.Game)
	}

	return &index, nil
}

// GetMrPackImportPlan parses the mrpack archive and produces an actionable import plan with file counts and conflict detection.
func GetMrPackImportPlan(r io.ReaderAt, size int64, existingInstanceNames []string) (*MrPackImportPlan, error) {
	index, err := ParseMrPack(r, size)
	if err != nil {
		return nil, err
	}

	gameVersion := index.Dependencies["minecraft"]
	loader := "vanilla"
	loaderVersion := ""
	for depKey, depVer := range index.Dependencies {
		switch strings.ToLower(depKey) {
		case "fabric-loader", "fabric":
			loader = "fabric"
			loaderVersion = depVer
		case "quilt-loader", "quilt":
			loader = "quilt"
			loaderVersion = depVer
		case "forge":
			loader = "forge"
			loaderVersion = depVer
		case "neoforge":
			loader = "neoforge"
			loaderVersion = depVer
		}
	}

	plan := &MrPackImportPlan{
		Name:          index.Name,
		Version:       index.VersionID,
		Summary:       index.Summary,
		GameVersion:   gameVersion,
		Loader:        loader,
		LoaderVersion: loaderVersion,
		TotalFiles:    len(index.Files),
	}

	for _, f := range index.Files {
		plan.TotalBytes += f.FileSize
		if f.Env != nil && f.Env.Client == "optional" {
			plan.OptionalFiles++
		} else if f.Env == nil || f.Env.Client != "unsupported" {
			plan.RequiredFiles++
		}
	}

	for _, existing := range existingInstanceNames {
		if strings.EqualFold(existing, index.Name) {
			plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("Сборка с именем %q уже существует", index.Name))
			break
		}
	}

	if gameVersion == "" {
		plan.Conflicts = append(plan.Conflicts, "Отсутствует версия Minecraft в манифесте модпака")
	}

	return plan, nil
}

// ExtractMrPackOverrides unpacks the overrides/ directory into the instance root with strict Zip-Slip protection (R1).
func ExtractMrPackOverrides(r io.ReaderAt, size int64, destDir string) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "overrides/") {
			continue
		}

		relPath := strings.TrimPrefix(f.Name, "overrides/")
		if relPath == "" {
			continue
		}

		targetPath, err := SafeRelPath(destDir, relPath)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if copyErr != nil {
			return copyErr
		}
	}

	return nil
}
