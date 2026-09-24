package content_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/manifest"
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

type mockHTTPClient struct {
	downloadFunc func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error
}

func (m *mockHTTPClient) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPClient) DownloadFile(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, url, destPath, expectedSHA1, onProgress)
	}
	return nil
}

type mockInstanceRepo struct {
	mu        sync.Mutex
	instances []*domain.Instance
}

func (m *mockInstanceRepo) Save(ctx context.Context, inst *domain.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.instances {
		if existing.ID == inst.ID {
			m.instances[i] = inst
			return nil
		}
	}
	m.instances = append(m.instances, inst)
	return nil
}

func (m *mockInstanceRepo) GetByID(ctx context.Context, id string) (*domain.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.instances {
		if inst.ID == id {
			return inst, nil
		}
	}
	return nil, fmt.Errorf("instance not found: %s", id)
}

func (m *mockInstanceRepo) ListAll(ctx context.Context) ([]*domain.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*domain.Instance, len(m.instances))
	copy(cp, m.instances)
	return cp, nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, inst := range m.instances {
		if inst.ID == id {
			m.instances = append(m.instances[:i], m.instances[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockInstanceRepo) UpdateState(ctx context.Context, id string, state domain.InstanceState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.instances {
		if inst.ID == id {
			inst.State = state
			return nil
		}
	}
	return fmt.Errorf("instance not found: %s", id)
}

func testSha1Hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

func testSha512Hex(b []byte) string {
	h := sha512.Sum512(b)
	return hex.EncodeToString(h[:])
}

func writeZipArchive(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func TestMrPack_ImportPipeline_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mrpackPath := filepath.Join(tmpDir, "test.mrpack")

	modContent := []byte("mod sodium jar content 1.0")
	s1 := testSha1Hex(modContent)
	s512 := testSha512Hex(modContent)
	overrideContent := []byte(`{"render_distance": 16}`)

	indexJSON := fmt.Sprintf(`{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Sodium Pack",
		"summary": "Pack with Sodium and config",
		"files": [
			{
				"path": "mods/sodium.jar",
				"hashes": {
					"sha1": "%s",
					"sha512": "%s"
				},
				"downloads": ["https://cdn.modrinth.com/data/sodium.jar"],
				"fileSize": %d
			}
		],
		"dependencies": {
			"minecraft": "1.21.1",
			"fabric-loader": "0.16.5"
		}
	}`, s1, s512, len(modContent))

	writeZipArchive(t, mrpackPath, map[string][]byte{
		"modrinth.index.json":           []byte(indexJSON),
		"overrides/config/sodium.json": overrideContent,
	})

	mockHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			if expectedSHA1 != "" && expectedSHA1 != s1 {
				return fmt.Errorf("unexpected expectedSHA1: %s", expectedSHA1)
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			return os.WriteFile(destPath, modContent, 0644)
		},
	}
	mockRepo := &mockInstanceRepo{}
	instancesDir := filepath.Join(tmpDir, "instances")

	importer := content.NewMrPackImporter(mockHTTP, mockRepo, instancesDir)

	var lastProgress content.MrPackImportProgress
	instID, err := importer.ImportMrPack(context.Background(), mrpackPath, content.ImportMrPackOptions{
		InstanceName:    "Sodium Custom Pack",
		IncludeOptional: true,
	}, func(p content.MrPackImportProgress) {
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("ImportMrPack failed: %v", err)
	}
	if instID == "" {
		t.Fatalf("expected non-empty instanceID, got empty")
	}

	// Verify instance in repo
	inst, err := mockRepo.GetByID(context.Background(), instID)
	if err != nil {
		t.Fatalf("instance not found in repo: %v", err)
	}
	if inst.Name != "Sodium Custom Pack" {
		t.Errorf("expected instance name 'Sodium Custom Pack', got %s", inst.Name)
	}
	if inst.GameVersion != "1.21.1" {
		t.Errorf("expected game version '1.21.1', got %s", inst.GameVersion)
	}
	if inst.Loader != domain.LoaderFabric || inst.LoaderVer != "0.16.5" {
		t.Errorf("expected fabric 0.16.5, got %s %s", inst.Loader, inst.LoaderVer)
	}
	if inst.State != domain.StateIdle {
		t.Errorf("expected state idle, got %s", inst.State)
	}

	// Verify downloaded mod file
	modFile := filepath.Join(instancesDir, instID, "mods", "sodium.jar")
	data, err := os.ReadFile(modFile)
	if err != nil {
		t.Fatalf("read downloaded mod: %v", err)
	}
	if string(data) != string(modContent) {
		t.Errorf("expected mod content %s, got %s", string(modContent), string(data))
	}

	// Verify override file
	configFile := filepath.Join(instancesDir, instID, "config", "sodium.json")
	cfgData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read override config: %v", err)
	}
	if string(cfgData) != string(overrideContent) {
		t.Errorf("expected config %s, got %s", string(overrideContent), string(cfgData))
	}

	// Verify nord-installs.json manifest
	modsDir := filepath.Join(instancesDir, instID, "mods")
	m, err := manifest.LoadManifest(modsDir)
	if err != nil {
		t.Fatalf("load manifest failed: %v", err)
	}
	rec := m.GetRecord("sodium.jar")
	if rec == nil {
		t.Fatalf("expected mod record for sodium.jar in manifest, got nil")
	}

	if lastProgress.Status != "complete" {
		t.Errorf("expected last progress status 'complete', got %s", lastProgress.Status)
	}
}

func TestMrPack_ImportPipeline_ChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	mrpackPath := filepath.Join(tmpDir, "corrupted.mrpack")

	indexJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Corrupted Pack",
		"files": [
			{
				"path": "mods/badmod.jar",
				"hashes": {
					"sha1": "0123456789abcdef0123456789abcdef01234567"
				},
				"downloads": ["https://cdn.modrinth.com/data/badmod.jar"],
				"fileSize": 500
			}
		],
		"dependencies": {
			"minecraft": "1.21.1"
		}
	}`

	writeZipArchive(t, mrpackPath, map[string][]byte{
		"modrinth.index.json": []byte(indexJSON),
	})

	mockHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			return fmt.Errorf("checksum mismatch: sha1 mismatch: expected %s, got corrupt", expectedSHA1)
		},
	}
	mockRepo := &mockInstanceRepo{}
	instancesDir := filepath.Join(tmpDir, "instances")

	importer := content.NewMrPackImporter(mockHTTP, mockRepo, instancesDir)
	instID, err := importer.ImportMrPack(context.Background(), mrpackPath, content.ImportMrPackOptions{
		InstanceName: "Corrupted Instance",
	}, nil)

	if err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected error to contain 'checksum mismatch', got: %v", err)
	}

	// Verify instance state was set to error
	if instID != "" {
		inst, getErr := mockRepo.GetByID(context.Background(), instID)
		if getErr == nil && inst.State != domain.StateError {
			t.Errorf("expected instance state to be error, got: %s", inst.State)
		}
	}
}

func TestMrPack_ImportPipeline_IdempotentResume(t *testing.T) {
	tmpDir := t.TempDir()
	mrpackPath := filepath.Join(tmpDir, "resume.mrpack")

	modAContent := []byte("mod A content")
	s1A := testSha1Hex(modAContent)
	modBContent := []byte("mod B content")
	s1B := testSha1Hex(modBContent)

	indexJSON := fmt.Sprintf(`{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Resume Pack",
		"files": [
			{
				"path": "mods/modA.jar",
				"hashes": {"sha1": "%s"},
				"downloads": ["https://cdn.example.com/modA.jar"],
				"fileSize": %d
			},
			{
				"path": "mods/modB.jar",
				"hashes": {"sha1": "%s"},
				"downloads": ["https://cdn.example.com/modB.jar"],
				"fileSize": %d
			}
		],
		"dependencies": {
			"minecraft": "1.20.1"
		}
	}`, s1A, len(modAContent), s1B, len(modBContent))

	writeZipArchive(t, mrpackPath, map[string][]byte{
		"modrinth.index.json": []byte(indexJSON),
	})

	var downloadCalls atomic.Int32
	var downloadedPaths []string
	var dlMu sync.Mutex

	mockHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			downloadCalls.Add(1)
			dlMu.Lock()
			downloadedPaths = append(downloadedPaths, filepath.Base(destPath))
			dlMu.Unlock()

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			if strings.Contains(url, "modA") {
				return os.WriteFile(destPath, modAContent, 0644)
			}
			return os.WriteFile(destPath, modBContent, 0644)
		},
	}

	mockRepo := &mockInstanceRepo{}
	instancesDir := filepath.Join(tmpDir, "instances")

	importer := content.NewMrPackImporter(mockHTTP, mockRepo, instancesDir)

	// Pre-create instance with fixed ID and pre-populate modA.jar
	instID := "resume-test-inst"
	inst := &domain.Instance{
		ID:          instID,
		Name:        "Resume Pack",
		GameVersion: "1.20.1",
		Loader:      domain.LoaderVanilla,
		State:       domain.StateImporting,
	}
	_ = mockRepo.Save(context.Background(), inst)

	// Pre-write modA.jar to instance disk with exact valid sha1
	modADir := filepath.Join(instancesDir, instID, "mods")
	_ = os.MkdirAll(modADir, 0755)
	_ = os.WriteFile(filepath.Join(modADir, "modA.jar"), modAContent, 0644)

	// Run import targeting existing instanceID (idempotent resume)
	resID, err := importer.ImportMrPack(context.Background(), mrpackPath, content.ImportMrPackOptions{
		InstanceID: instID,
	}, nil)
	if err != nil {
		t.Fatalf("ImportMrPack resume failed: %v", err)
	}
	if resID != instID {
		t.Errorf("expected resumed instance ID %s, got %s", instID, resID)
	}

	// Assert: DownloadFile was called EXACTLY ONCE (for modB), NOT for modA (it was skipped!)
	if downloadCalls.Load() != 1 {
		t.Errorf("expected exactly 1 download call (for modB), got %d: %v", downloadCalls.Load(), downloadedPaths)
	}
	if len(downloadedPaths) == 1 && downloadedPaths[0] != "modB.jar" {
		t.Errorf("expected modB.jar to be downloaded, got %s", downloadedPaths[0])
	}

	// Verify both files exist
	if _, err := os.Stat(filepath.Join(modADir, "modA.jar")); err != nil {
		t.Errorf("modA.jar missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modADir, "modB.jar")); err != nil {
		t.Errorf("modB.jar missing: %v", err)
	}
}

func TestMrPack_ImportPipeline_OverridesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	mrpackPath := filepath.Join(tmpDir, "override_prec.mrpack")

	dlContent := []byte("downloaded version of colliding mod")
	s1 := testSha1Hex(dlContent)
	overrideWinnerContent := []byte("override winner content")

	indexJSON := fmt.Sprintf(`{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Precedence Pack",
		"files": [
			{
				"path": "mods/collide.jar",
				"hashes": {"sha1": "%s"},
				"downloads": ["https://cdn.example.com/collide.jar"],
				"fileSize": %d
			}
		],
		"dependencies": {
			"minecraft": "1.21.1"
		}
	}`, s1, len(dlContent))

	writeZipArchive(t, mrpackPath, map[string][]byte{
		"modrinth.index.json":         []byte(indexJSON),
		"overrides/mods/collide.jar": overrideWinnerContent,
	})

	mockHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			return os.WriteFile(destPath, dlContent, 0644)
		},
	}
	mockRepo := &mockInstanceRepo{}
	instancesDir := filepath.Join(tmpDir, "instances")

	importer := content.NewMrPackImporter(mockHTTP, mockRepo, instancesDir)
	instID, err := importer.ImportMrPack(context.Background(), mrpackPath, content.ImportMrPackOptions{}, nil)
	if err != nil {
		t.Fatalf("ImportMrPack failed: %v", err)
	}

	finalPath := filepath.Join(instancesDir, instID, "mods", "collide.jar")
	contentBytes, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("read final colliding file: %v", err)
	}

	if string(contentBytes) != string(overrideWinnerContent) {
		t.Fatalf("overrides did not take precedence! expected %q, got %q", string(overrideWinnerContent), string(contentBytes))
	}
}

func TestMrPack_Importer_CoverageBoosters(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. NewMrPackImporter with empty instancesDir defaults to "instances"
	defaultImporter := content.NewMrPackImporter(&mockHTTPClient{}, &mockInstanceRepo{}, "")
	if defaultImporter == nil {
		t.Fatalf("expected non-nil importer")
	}

	// 2. GetStatus on non-existent instance
	status, ok := defaultImporter.GetStatus("non-existent-id")
	if ok || status != nil {
		t.Errorf("expected GetStatus to return false and nil, got ok=%v, status=%v", ok, status)
	}

	// 3. CalculateFileSHA1 / CalculateFileSHA512 error on non-existent file
	if _, err := content.CalculateFileSHA1(filepath.Join(tmpDir, "missing.jar")); err == nil {
		t.Errorf("expected error on missing file for SHA1")
	}
	if _, err := content.CalculateFileSHA512(filepath.Join(tmpDir, "missing.jar")); err == nil {
		t.Errorf("expected error on missing file for SHA512")
	}

	// 4. Cancelled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := defaultImporter.ImportMrPack(cancCtx, filepath.Join(tmpDir, "dummy.mrpack"), content.ImportMrPackOptions{}, nil)
	if err == nil {
		t.Errorf("expected error with cancelled context, got nil")
	}

	// 5. Non-existent mrpack file
	_, err = defaultImporter.ImportMrPack(context.Background(), filepath.Join(tmpDir, "missing.mrpack"), content.ImportMrPackOptions{}, nil)
	if err == nil {
		t.Errorf("expected error with missing mrpack file, got nil")
	}

	// 6. Missing minecraft version in dependencies
	noMCMrpack := filepath.Join(tmpDir, "no_mc.mrpack")
	writeZipArchive(t, noMCMrpack, map[string][]byte{
		"modrinth.index.json": []byte(`{"formatVersion": 1, "game": "minecraft", "dependencies": {}}`),
	})
	_, err = defaultImporter.ImportMrPack(context.Background(), noMCMrpack, content.ImportMrPackOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing minecraft version") {
		t.Errorf("expected missing minecraft error, got: %v", err)
	}

	// 7. Zip-slip in index file path
	slipMrpack := filepath.Join(tmpDir, "slip.mrpack")
	writeZipArchive(t, slipMrpack, map[string][]byte{
		"modrinth.index.json": []byte(`{
			"formatVersion": 1,
			"game": "minecraft",
			"dependencies": {"minecraft": "1.21.1"},
			"files": [{"path": "../../escape.jar", "hashes": {"sha1": "abc"}, "downloads": ["https://cdn.example.com"]}]
		}`),
	})
	mockRepo := &mockInstanceRepo{}
	importer := content.NewMrPackImporter(&mockHTTPClient{}, mockRepo, tmpDir)
	slipID, err := importer.ImportMrPack(context.Background(), slipMrpack, content.ImportMrPackOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
	if slipID != "" {
		inst, _ := mockRepo.GetByID(context.Background(), slipID)
		if inst != nil && inst.State != domain.StateError {
			t.Errorf("expected instance state to be error, got: %s", inst.State)
		}
	}

	// 8. File with no download URLs
	noDlMrpack := filepath.Join(tmpDir, "nodl.mrpack")
	writeZipArchive(t, noDlMrpack, map[string][]byte{
		"modrinth.index.json": []byte(`{
			"formatVersion": 1,
			"game": "minecraft",
			"dependencies": {"minecraft": "1.21.1"},
			"files": [{"path": "mods/nodl.jar", "hashes": {"sha1": "abc"}, "downloads": []}]
		}`),
	})
	_, err = importer.ImportMrPack(context.Background(), noDlMrpack, content.ImportMrPackOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no download URLs") {
		t.Errorf("expected no download URLs error, got: %v", err)
	}

	// 9. Client unsupported and optional filtering
	filterMrpack := filepath.Join(tmpDir, "filter.mrpack")
	modContent := []byte("filtered mod content")
	s1 := testSha1Hex(modContent)
	writeZipArchive(t, filterMrpack, map[string][]byte{
		"modrinth.index.json": []byte(fmt.Sprintf(`{
			"formatVersion": 1,
			"game": "minecraft",
			"dependencies": {"minecraft": "1.21.1", "quilt-loader": "0.26.0"},
			"files": [
				{
					"path": "mods/unsupported.jar",
					"hashes": {"sha1": "%s"},
					"env": {"client": "unsupported"},
					"downloads": ["https://cdn.example.com/unsupported.jar"]
				},
				{
					"path": "mods/optional.jar",
					"hashes": {"sha1": "%s"},
					"env": {"client": "optional"},
					"downloads": ["https://cdn.example.com/optional.jar"]
				}
			]
		}`, s1, s1)),
	})

	var downloaded []string
	filterHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			downloaded = append(downloaded, filepath.Base(destPath))
			return os.WriteFile(destPath, modContent, 0644)
		},
	}
	filterImporter := content.NewMrPackImporter(filterHTTP, mockRepo, tmpDir)
	filterInstID, err := filterImporter.ImportMrPack(context.Background(), filterMrpack, content.ImportMrPackOptions{
		IncludeOptional: false,
	}, nil)
	if err != nil {
		t.Fatalf("filter import failed: %v", err)
	}
	if len(downloaded) != 0 {
		t.Errorf("expected 0 files downloaded when optional=false and client=unsupported, got: %v", downloaded)
	}
	inst, _ := mockRepo.GetByID(context.Background(), filterInstID)
	if inst == nil || inst.Loader != domain.LoaderQuilt {
		t.Errorf("expected quilt loader, got: %+v", inst)
	}

	// Test GetStatus on completed import
	st, ok := filterImporter.GetStatus(filterInstID)
	if !ok || st == nil || st.Status != "complete" {
		t.Errorf("expected status complete, got ok=%v, st=%+v", ok, st)
	}

	// 10. Multiple download URLs: URL 1 fails, URL 2 succeeds
	fallbackMrpack := filepath.Join(tmpDir, "fallback.mrpack")
	writeZipArchive(t, fallbackMrpack, map[string][]byte{
		"modrinth.index.json": []byte(fmt.Sprintf(`{
			"formatVersion": 1,
			"game": "minecraft",
			"dependencies": {"minecraft": "1.21.1", "neoforge": "21.1.50"},
			"files": [
				{
					"path": "mods/fallback.jar",
					"hashes": {"sha1": "%s"},
					"downloads": [
						"https://broken.cdn.com/fallback.jar",
						"https://working.cdn.com/fallback.jar"
					]
				}
			]
		}`, s1)),
	})
	fallbackHTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			if strings.Contains(url, "broken") {
				return fmt.Errorf("connection refused")
			}
			return os.WriteFile(destPath, modContent, 0644)
		},
	}
	fallbackImporter := content.NewMrPackImporter(fallbackHTTP, mockRepo, tmpDir)
	fallbackInstID, err := fallbackImporter.ImportMrPack(context.Background(), fallbackMrpack, content.ImportMrPackOptions{}, nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	fInst, _ := mockRepo.GetByID(context.Background(), fallbackInstID)
	if fInst == nil || fInst.Loader != domain.LoaderNeoForge {
		t.Errorf("expected neoforge loader, got: %+v", fInst)
	}

	// 11. Secondary SHA-512 mismatch
	sha512MismatchMrpack := filepath.Join(tmpDir, "sha512mismatch.mrpack")
	writeZipArchive(t, sha512MismatchMrpack, map[string][]byte{
		"modrinth.index.json": []byte(fmt.Sprintf(`{
			"formatVersion": 1,
			"game": "minecraft",
			"dependencies": {"minecraft": "1.21.1", "forge": "47.2.0"},
			"files": [
				{
					"path": "mods/corrupt512.jar",
					"hashes": {
						"sha1": "%s",
						"sha512": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
					},
					"downloads": ["https://cdn.example.com/corrupt512.jar"]
				}
			]
		}`, s1)),
	})
	s512HTTP := &mockHTTPClient{
		downloadFunc: func(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
			return os.WriteFile(destPath, modContent, 0644)
		},
	}
	s512Importer := content.NewMrPackImporter(s512HTTP, mockRepo, tmpDir)
	_, err = s512Importer.ImportMrPack(context.Background(), sha512MismatchMrpack, content.ImportMrPackOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "sha512 mismatch") {
		t.Errorf("expected sha512 mismatch error, got: %v", err)
	}
}

func TestMrPack_GetImportPlan_LoadersAndConflicts(t *testing.T) {
	// 1. Quilt loader
	quiltJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"dependencies": {"minecraft": "1.20.1", "quilt-loader": "0.24.0"}
	}`
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("modrinth.index.json")
	_, _ = w.Write([]byte(quiltJSON))
	_ = zw.Close()
	plan, err := content.GetMrPackImportPlan(bytes.NewReader(buf.Bytes()), int64(buf.Len()), nil)
	if err != nil || plan.Loader != "quilt" {
		t.Errorf("expected quilt loader, got %s, err: %v", plan.Loader, err)
	}

	// 2. Forge loader
	forgeJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"dependencies": {"minecraft": "1.20.1", "forge": "47.1.0"}
	}`
	buf = new(bytes.Buffer)
	zw = zip.NewWriter(buf)
	w, _ = zw.Create("modrinth.index.json")
	_, _ = w.Write([]byte(forgeJSON))
	_ = zw.Close()
	plan, err = content.GetMrPackImportPlan(bytes.NewReader(buf.Bytes()), int64(buf.Len()), nil)
	if err != nil || plan.Loader != "forge" {
		t.Errorf("expected forge loader, got %s, err: %v", plan.Loader, err)
	}

	// 3. NeoForge loader
	neoJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"dependencies": {"minecraft": "1.21.1", "neoforge": "21.1.20"}
	}`
	buf = new(bytes.Buffer)
	zw = zip.NewWriter(buf)
	w, _ = zw.Create("modrinth.index.json")
	_, _ = w.Write([]byte(neoJSON))
	_ = zw.Close()
	plan, err = content.GetMrPackImportPlan(bytes.NewReader(buf.Bytes()), int64(buf.Len()), nil)
	if err != nil || plan.Loader != "neoforge" {
		t.Errorf("expected neoforge loader, got %s, err: %v", plan.Loader, err)
	}

	// 4. Missing minecraft version -> conflict
	noMCJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"dependencies": {"fabric-loader": "0.15.0"}
	}`
	buf = new(bytes.Buffer)
	zw = zip.NewWriter(buf)
	w, _ = zw.Create("modrinth.index.json")
	_, _ = w.Write([]byte(noMCJSON))
	_ = zw.Close()
	plan, err = content.GetMrPackImportPlan(bytes.NewReader(buf.Bytes()), int64(buf.Len()), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Conflicts) == 0 || !strings.Contains(plan.Conflicts[0], "Отсутствует версия Minecraft") {
		t.Errorf("expected missing minecraft conflict, got: %v", plan.Conflicts)
	}
}



