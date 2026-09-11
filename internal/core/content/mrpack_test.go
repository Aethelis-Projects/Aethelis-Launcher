package content_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/content"
)

func createTestMrPack(t *testing.T) ([]byte, int64) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// Add modrinth.index.json
	indexJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "v1.0.0",
		"name": "Nordic Performance",
		"summary": "High FPS optimization pack",
		"files": [
			{
				"path": "mods/sodium-fabric.jar",
				"hashes": {
					"sha1": "da39a3ee5e6b4b0d3255bfef95601890afd80709"
				},
				"downloads": [
					"https://cdn.modrinth.com/sodium-fabric.jar"
				],
				"fileSize": 102400
			}
		],
		"dependencies": {
			"minecraft": "1.21.1",
			"fabric-loader": "0.16.5"
		}
	}`

	fIndex, err := zw.Create("modrinth.index.json")
	if err != nil {
		t.Fatalf("create index in zip: %v", err)
	}
	_, _ = fIndex.Write([]byte(indexJSON))

	// Add overrides/config/sodium-options.json
	fOverride, err := zw.Create("overrides/config/sodium-options.json")
	if err != nil {
		t.Fatalf("create override file in zip: %v", err)
	}
	_, _ = fOverride.Write([]byte(`{"render_distance": 12}`))

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	data := buf.Bytes()
	return data, int64(len(data))
}

func TestMrPack_ParseAndExtractOverrides(t *testing.T) {
	zipBytes, size := createTestMrPack(t)
	r := bytes.NewReader(zipBytes)

	// 1. Parse .mrpack index
	index, err := content.ParseMrPack(r, size)
	if err != nil {
		t.Fatalf("ParseMrPack failed: %v", err)
	}

	if index.Name != "Nordic Performance" {
		t.Errorf("expected name 'Nordic Performance', got %s", index.Name)
	}
	if index.Dependencies["minecraft"] != "1.21.1" {
		t.Errorf("expected mc 1.21.1, got %s", index.Dependencies["minecraft"])
	}
	if len(index.Files) != 1 || index.Files[0].Path != "mods/sodium-fabric.jar" {
		t.Errorf("unexpected files in index: %+v", index.Files)
	}

	// 2. Extract overrides into temporary instance dir
	tempDir := t.TempDir()
	if err := content.ExtractMrPackOverrides(r, size, tempDir); err != nil {
		t.Fatalf("ExtractMrPackOverrides failed: %v", err)
	}

	extractedFile := filepath.Join(tempDir, "config", "sodium-options.json")
	contentBytes, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("read extracted override file: %v", err)
	}

	if string(contentBytes) != `{"render_distance": 12}` {
		t.Errorf("unexpected override content: %s", string(contentBytes))
	}
}

func TestMrPack_InvalidZip(t *testing.T) {
	badData := []byte("not-a-zip-archive")
	_, err := content.ParseMrPack(bytes.NewReader(badData), int64(len(badData)))
	if err == nil {
		t.Fatalf("expected error on invalid zip, got nil")
	}

	// Valid zip without modrinth.index.json
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	_, _ = zw.Create("random.txt")
	_ = zw.Close()

	emptyData := buf.Bytes()
	_, err = content.ParseMrPack(bytes.NewReader(emptyData), int64(len(emptyData)))
	if err == nil {
		t.Fatalf("expected ErrInvalidMrPack, got nil")
	}
}
