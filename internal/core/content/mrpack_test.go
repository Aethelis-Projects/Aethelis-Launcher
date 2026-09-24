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

	// Add overrides directory entries
	_, _ = zw.Create("overrides/")
	_, _ = zw.Create("overrides/config/")

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

	// Unsupported game
	badGameBuf := new(bytes.Buffer)
	zwBad := zip.NewWriter(badGameBuf)
	fBad, _ := zwBad.Create("modrinth.index.json")
	_, _ = fBad.Write([]byte(`{"formatVersion": 1, "game": "othergame"}`))
	_ = zwBad.Close()
	badGameData := badGameBuf.Bytes()
	_, err = content.ParseMrPack(bytes.NewReader(badGameData), int64(len(badGameData)))
	if err == nil {
		t.Fatalf("expected ErrUnsupportedGame, got nil")
	}

	// Extract overrides with invalid zip
	err = content.ExtractMrPackOverrides(bytes.NewReader(badData), int64(len(badData)), t.TempDir())
	if err == nil {
		t.Fatalf("expected error on extract invalid zip, got nil")
	}
}

func TestExtractMrPackOverrides_ZipSlip(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// Create a malicious override entry trying to traverse above destDir
	fBad, err := zw.Create("overrides/../../escaped.txt")
	if err != nil {
		t.Fatalf("create bad entry: %v", err)
	}
	_, _ = fBad.Write([]byte("malicious content"))
	_ = zw.Close()

	destDir := filepath.Join(t.TempDir(), "instance")
	_ = os.MkdirAll(destDir, 0755)

	zipBytes := buf.Bytes()
	err = content.ExtractMrPackOverrides(bytes.NewReader(zipBytes), int64(len(zipBytes)), destDir)
	if err == nil {
		t.Fatalf("CRITICAL SECURITY ERROR: expected Zip-Slip path traversal to fail, but got nil")
	}

	// Verify nothing was written outside destDir
	escapedPath := filepath.Join(destDir, "..", "..", "escaped.txt")
	if _, statErr := os.Stat(escapedPath); statErr == nil {
		t.Fatalf("CRITICAL SECURITY ERROR: escaped.txt was written outside destination directory!")
	}
}

func TestMrPack_FormatVersionValidation(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("modrinth.index.json")
	_, _ = f.Write([]byte(`{"formatVersion": 2, "game": "minecraft"}`))
	_ = zw.Close()

	data := buf.Bytes()
	_, err := content.ParseMrPack(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatalf("expected error for formatVersion 2, got nil")
	}
}

func TestMrPack_MagicBytesValidation(t *testing.T) {
	data := []byte("INVALID_BYTES_NOT_ZIP_MAGIC_HEADER")
	_, err := content.ParseMrPack(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatalf("expected error on missing zip magic bytes, got nil")
	}
}

func TestMrPack_MaxArchiveSizeCap(t *testing.T) {
	dummyReader := bytes.NewReader([]byte{0x50, 0x4B, 0x03, 0x04})
	hugeSize := int64(600 * 1024 * 1024) // 600 MB > 512 MB
	_, err := content.ParseMrPack(dummyReader, hugeSize)
	if err == nil {
		t.Fatalf("expected error for size exceeding 512 MB, got nil")
	}
}

func TestMrPack_GetImportPlan(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	indexJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Fabulously Optimized",
		"summary": "Simple optimization pack",
		"files": [
			{
				"path": "mods/sodium.jar",
				"hashes": {"sha1": "abcd"},
				"env": {"client": "required"},
				"downloads": ["https://cdn.modrinth.com/sodium.jar"],
				"fileSize": 204800
			},
			{
				"path": "mods/iris.jar",
				"hashes": {"sha1": "efgh"},
				"env": {"client": "optional"},
				"downloads": ["https://cdn.modrinth.com/iris.jar"],
				"fileSize": 102400
			}
		],
		"dependencies": {
			"minecraft": "1.20.1",
			"fabric-loader": "0.15.11"
		}
	}`

	fIndex, _ := zw.Create("modrinth.index.json")
	_, _ = fIndex.Write([]byte(indexJSON))
	_ = zw.Close()

	data := buf.Bytes()
	plan, err := content.GetMrPackImportPlan(bytes.NewReader(data), int64(len(data)), []string{"Fabulously Optimized"})
	if err != nil {
		t.Fatalf("GetMrPackImportPlan failed: %v", err)
	}

	if plan.Name != "Fabulously Optimized" {
		t.Errorf("expected pack name 'Fabulously Optimized', got %s", plan.Name)
	}
	if plan.GameVersion != "1.20.1" {
		t.Errorf("expected mc 1.20.1, got %s", plan.GameVersion)
	}
	if plan.Loader != "fabric" || plan.LoaderVersion != "0.15.11" {
		t.Errorf("expected fabric 0.15.11, got %s %s", plan.Loader, plan.LoaderVersion)
	}
	if plan.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", plan.TotalFiles)
	}
	if plan.RequiredFiles != 1 || plan.OptionalFiles != 1 {
		t.Errorf("expected 1 required and 1 optional file, got req=%d, opt=%d", plan.RequiredFiles, plan.OptionalFiles)
	}
	if plan.TotalBytes != 307200 {
		t.Errorf("expected 307200 bytes, got %d", plan.TotalBytes)
	}
	if len(plan.Conflicts) == 0 {
		t.Errorf("expected conflict detected for existing instance name, got 0 conflicts")
	}
}

