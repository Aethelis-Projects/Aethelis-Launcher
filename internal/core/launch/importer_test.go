package launch_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

type mockRepo struct {
	instances map[string]*domain.Instance
}

func newMockRepo() *mockRepo {
	return &mockRepo{instances: make(map[string]*domain.Instance)}
}

func (m *mockRepo) Save(ctx context.Context, inst *domain.Instance) error {
	m.instances[inst.ID] = inst
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.Instance, error) {
	inst, ok := m.instances[id]
	if !ok {
		return nil, domain.ErrInstanceNotFound
	}
	return inst, nil
}

func (m *mockRepo) ListAll(ctx context.Context) ([]*domain.Instance, error) {
	list := make([]*domain.Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		list = append(list, inst)
	}
	return list, nil
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	delete(m.instances, id)
	return nil
}

func (m *mockRepo) UpdateState(ctx context.Context, id string, state domain.InstanceState) error {
	if inst, ok := m.instances[id]; ok {
		inst.State = state
	}
	return nil
}

type mockClock struct{}

func (mockClock) Now() time.Time {
	return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
}

func (mockClock) Since(t time.Time) time.Duration {
	return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC).Sub(t)
}

func (mockClock) Sleep(d time.Duration) {}

func setupImporterTest(t *testing.T) (*launch.InstanceService, *launch.InstanceImporter, string) {
	t.Helper()
	repo := newMockRepo()
	instancesDir := t.TempDir()
	svc := launch.NewInstanceService(repo, nil, nil, nil, mockClock{})
	importer := launch.NewInstanceImporter(svc, instancesDir)
	return svc, importer, instancesDir
}

func TestInstanceImporter_ScanOfficialMinecraft(t *testing.T) {
	_, importer, _ := setupImporterTest(t)
	srcDir := t.TempDir()

	// Setup structure
	if err := os.MkdirAll(filepath.Join(srcDir, "versions", "1.21.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "saves", "MyWorld"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "saves", "MyWorld", "level.dat"), []byte("dummy level data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "resourcepacks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "resourcepacks", "textures.zip"), []byte("dummy zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "screenshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "screenshots", "2026-09-26.png"), []byte("dummy png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "options.txt"), []byte("fov:70.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "servers.dat"), []byte("dummy servers"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := importer.ScanOfficialMinecraft(srcDir)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}

	if summary.WorldCount != 1 {
		t.Errorf("expected 1 world, got %d", summary.WorldCount)
	}
	if summary.ResourcePacks != 1 {
		t.Errorf("expected 1 resource pack, got %d", summary.ResourcePacks)
	}
	if summary.Screenshots != 1 {
		t.Errorf("expected 1 screenshot, got %d", summary.Screenshots)
	}
	if !summary.HasOptions {
		t.Errorf("expected HasOptions true")
	}
	if !summary.HasServers {
		t.Errorf("expected HasServers true")
	}
	if summary.DefaultVersion != "1.21.1" {
		t.Errorf("expected default version 1.21.1, got %s", summary.DefaultVersion)
	}
}

func TestInstanceImporter_ImportOfficialMinecraft(t *testing.T) {
	_, importer, instancesDir := setupImporterTest(t)
	srcDir := t.TempDir()

	// Setup content
	if err := os.MkdirAll(filepath.Join(srcDir, "saves", "SurvivalWorld"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "saves", "SurvivalWorld", "level.dat"), []byte("world data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "options.txt"), []byte("renderDistance:12\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Secret credential file that MUST NOT be copied
	if err := os.WriteFile(filepath.Join(srcDir, "launcher_accounts.json"), []byte(`{"accessToken":"supersecret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	inst, err := importer.ImportOfficialMinecraft(context.Background(), launch.ImportOfficialRequest{
		SourceDir:         srcDir,
		InstanceName:      "Official Vanilla",
		GameVersion:       "1.21.1",
		Loader:            "vanilla",
		CopySaves:         true,
		CopyOptions:       true,
		CopyResourcePacks: false,
	})
	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	if inst == nil || inst.Name != "Official Vanilla" {
		t.Fatalf("expected instance created with name 'Official Vanilla', got %+v", inst)
	}

	destDir := filepath.Join(instancesDir, inst.ID)

	// Verify world was copied
	levelPath := filepath.Join(destDir, "saves", "SurvivalWorld", "level.dat")
	if _, err := os.Stat(levelPath); err != nil {
		t.Errorf("expected level.dat copied to %s: %v", levelPath, err)
	}

	// Verify options was copied
	optionsPath := filepath.Join(destDir, "options.txt")
	if _, err := os.Stat(optionsPath); err != nil {
		t.Errorf("expected options.txt copied to %s: %v", optionsPath, err)
	}

	// Verify security: credentials NEVER copied
	credsPath := filepath.Join(destDir, "launcher_accounts.json")
	if _, err := os.Stat(credsPath); !os.IsNotExist(err) {
		t.Errorf("SECURITY LEAK: launcher_accounts.json was copied to %s!", credsPath)
	}
}

func TestInstanceImporter_ScanAndImportPrism(t *testing.T) {
	_, importer, instancesDir := setupImporterTest(t)
	prismDir := t.TempDir()

	// Setup Prism instance
	instanceCfg := `name = My Prism Pack
IntendedVersion = 1.20.1
iconKey = default
`
	if err := os.WriteFile(filepath.Join(prismDir, "instance.cfg"), []byte(instanceCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	mmcPackJSON := `{
		"components": [
			{ "cachedName": "Minecraft", "uid": "net.minecraft", "version": "1.20.1" },
			{ "cachedName": "Fabric Loader", "uid": "net.fabricmc.fabric-loader", "version": "0.15.11" }
		],
		"formatVersion": 1
	}`
	if err := os.WriteFile(filepath.Join(prismDir, "mmc-pack.json"), []byte(mmcPackJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Setup .minecraft subfolder
	mcSubDir := filepath.Join(prismDir, ".minecraft")
	if err := os.MkdirAll(filepath.Join(mcSubDir, "saves", "PrismSave"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcSubDir, "saves", "PrismSave", "level.dat"), []byte("save data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mcSubDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcSubDir, "mods", "sodium-0.5.8.jar"), []byte("dummy jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Scan
	summary, err := importer.ScanPrismInstance(prismDir)
	if err != nil {
		t.Fatalf("scan prism error: %v", err)
	}

	if summary.InstanceName != "My Prism Pack" {
		t.Errorf("expected name 'My Prism Pack', got %s", summary.InstanceName)
	}
	if summary.GameVersion != "1.20.1" {
		t.Errorf("expected version 1.20.1, got %s", summary.GameVersion)
	}
	if summary.Loader != "fabric" {
		t.Errorf("expected loader fabric, got %s", summary.Loader)
	}
	if summary.WorldCount != 1 {
		t.Errorf("expected 1 world, got %d", summary.WorldCount)
	}
	if summary.ModCount != 1 {
		t.Errorf("expected 1 mod, got %d", summary.ModCount)
	}

	// Import
	inst, err := importer.ImportPrismInstance(context.Background(), launch.ImportPrismRequest{
		SourceDir:    prismDir,
		InstanceName: "Imported Prism",
		CopySaves:    true,
		CopyMods:     true,
	})
	if err != nil {
		t.Fatalf("import prism error: %v", err)
	}

	if inst.Loader != domain.LoaderFabric {
		t.Errorf("expected instance loader fabric, got %s", inst.Loader)
	}
	if inst.GameVersion != "1.20.1" {
		t.Errorf("expected instance version 1.20.1, got %s", inst.GameVersion)
	}

	destDir := filepath.Join(instancesDir, inst.ID)
	modFile := filepath.Join(destDir, "mods", "sodium-0.5.8.jar")
	if _, err := os.Stat(modFile); err != nil {
		t.Errorf("expected mod copied to %s: %v", modFile, err)
	}
}

func TestDefaultOfficialMinecraftPath(t *testing.T) {
	p := launch.DefaultOfficialMinecraftPath()
	if p == "" {
		t.Errorf("expected non-empty default official minecraft path")
	}
}

func TestInstanceImporter_ScanOfficialMinecraft_ErrorsAndEdgeCases(t *testing.T) {
	_, importer, _ := setupImporterTest(t)

	// Nonexistent dir
	_, err := importer.ScanOfficialMinecraft(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatalf("expected error for nonexistent dir")
	}

	// Empty dir
	emptyDir := t.TempDir()
	summary, err := importer.ScanOfficialMinecraft(emptyDir)
	if err != nil {
		t.Fatalf("unexpected error on empty dir: %v", err)
	}
	if summary.WorldCount != 0 || summary.ResourcePacks != 0 || summary.Screenshots != 0 || summary.ModCount != 0 {
		t.Errorf("expected 0 counts on empty dir, got %+v", summary)
	}
}

func TestInstanceImporter_ImportOfficialMinecraft_AllComponentsAndErrors(t *testing.T) {
	_, importer, instancesDir := setupImporterTest(t)

	// 1. Error on nonexistent dir
	_, err := importer.ImportOfficialMinecraft(context.Background(), launch.ImportOfficialRequest{
		SourceDir: filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatalf("expected error for nonexistent source dir")
	}

	// 2. All components copy
	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "saves", "World1"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "saves", "World1", "level.dat"), []byte("dat"), 0o644)
	_ = os.MkdirAll(filepath.Join(srcDir, "resourcepacks", "Pack1"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "resourcepacks", "Pack1.zip"), []byte("zip"), 0o644)
	_ = os.MkdirAll(filepath.Join(srcDir, "screenshots"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "screenshots", "shot.png"), []byte("png"), 0o644)
	_ = os.MkdirAll(filepath.Join(srcDir, "mods"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "mods", "test.jar"), []byte("jar"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "options.txt"), []byte("opt"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "servers.dat"), []byte("srv"), 0o644)

	inst, err := importer.ImportOfficialMinecraft(context.Background(), launch.ImportOfficialRequest{
		SourceDir:         srcDir,
		CopySaves:         true,
		CopyResourcePacks: true,
		CopyScreenshots:   true,
		CopyMods:          true,
		CopyOptions:       true,
		CopyServers:       true,
	})
	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	destDir := filepath.Join(instancesDir, inst.ID)
	if _, err := os.Stat(filepath.Join(destDir, "saves", "World1", "level.dat")); err != nil {
		t.Errorf("expected level.dat copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "resourcepacks", "Pack1.zip")); err != nil {
		t.Errorf("expected Pack1.zip copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "screenshots", "shot.png")); err != nil {
		t.Errorf("expected shot.png copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "mods", "test.jar")); err != nil {
		t.Errorf("expected test.jar copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "options.txt")); err != nil {
		t.Errorf("expected options.txt copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "servers.dat")); err != nil {
		t.Errorf("expected servers.dat copied")
	}
}

func TestInstanceImporter_Prism_LoadersAndSubdirVariants(t *testing.T) {
	_, importer, instancesDir := setupImporterTest(t)

	// 1. Error on nonexistent dir
	_, err := importer.ScanPrismInstance(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatalf("expected error on nonexistent prism dir")
	}
	_, err = importer.ImportPrismInstance(context.Background(), launch.ImportPrismRequest{
		SourceDir: filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatalf("expected error on nonexistent prism dir import")
	}

	// 2. Test Quilt and NeoForge and Forge components
	loaders := []struct {
		uid      string
		expected string
	}{
		{"org.quiltmc.quilt-loader", "quilt"},
		{"net.neoforged.neoforge", "neoforge"},
		{"net.minecraftforge", "forge"},
	}

	for _, l := range loaders {
		pDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(pDir, "instance.cfg"), []byte("name = Test\nIntendedVersion = 1.20.4\n"), 0o644)
		pack := fmt.Sprintf(`{"components":[{"uid":"net.minecraft","version":"1.20.4"},{"uid":"%s","version":"1.0.0"}]}`, l.uid)
		_ = os.WriteFile(filepath.Join(pDir, "mmc-pack.json"), []byte(pack), 0o644)

		// Test using "minecraft" folder instead of ".minecraft"
		mcDir := filepath.Join(pDir, "minecraft")
		_ = os.MkdirAll(filepath.Join(mcDir, "resourcepacks"), 0o755)
		_ = os.WriteFile(filepath.Join(mcDir, "resourcepacks", "pack.zip"), []byte("zip"), 0o644)
		_ = os.MkdirAll(filepath.Join(mcDir, "screenshots"), 0o755)
		_ = os.WriteFile(filepath.Join(mcDir, "screenshots", "screen.png"), []byte("png"), 0o644)
		_ = os.WriteFile(filepath.Join(mcDir, "options.txt"), []byte("opt"), 0o644)
		_ = os.WriteFile(filepath.Join(mcDir, "servers.dat"), []byte("srv"), 0o644)

		summary, err := importer.ScanPrismInstance(pDir)
		if err != nil {
			t.Fatalf("failed scan: %v", err)
		}
		if summary.Loader != l.expected {
			t.Errorf("expected loader %s, got %s", l.expected, summary.Loader)
		}
		if summary.ResourcePacks != 1 {
			t.Errorf("expected 1 resource pack in minecraft/ dir, got %d", summary.ResourcePacks)
		}

		inst, err := importer.ImportPrismInstance(context.Background(), launch.ImportPrismRequest{
			SourceDir:         pDir,
			CopyResourcePacks: true,
			CopyScreenshots:   true,
			CopyOptions:       true,
			CopyServers:       true,
		})
		if err != nil {
			t.Fatalf("failed import: %v", err)
		}
		dest := filepath.Join(instancesDir, inst.ID)
		if _, err := os.Stat(filepath.Join(dest, "options.txt")); err != nil {
			t.Errorf("expected options.txt in dest")
		}
		if _, err := os.Stat(filepath.Join(dest, "servers.dat")); err != nil {
			t.Errorf("expected servers.dat in dest")
		}
	}
}

