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

var (
	ErrInvalidMrPack   = errors.New("invalid .mrpack file: missing modrinth.index.json")
	ErrUnsupportedGame = errors.New("unsupported game in modpack")
)

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
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

// ParseMrPack reads and parses modrinth.index.json from a .mrpack ZIP archive.
func ParseMrPack(r io.ReaderAt, size int64) (*MrPackIndex, error) {
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

	if index.Game != "" && index.Game != "minecraft" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedGame, index.Game)
	}

	return &index, nil
}

// ExtractMrPackOverrides unpacks the overrides/ directory into the instance root.
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

		targetPath := filepath.Join(destDir, relPath)

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
