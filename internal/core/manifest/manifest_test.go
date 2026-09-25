package manifest_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/manifest"
)

func TestManifest_EmptyAndSave(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Loading non-existent manifest returns empty manifest
	m, err := manifest.LoadManifest(tempDir)
	if err != nil {
		t.Fatalf("expected nil error on missing manifest, got: %v", err)
	}
	if m.SchemaVersion != manifest.CurrentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", manifest.CurrentSchemaVersion, m.SchemaVersion)
	}
	if len(m.Mods) != 0 {
		t.Errorf("expected 0 mods in empty manifest, got %d", len(m.Mods))
	}

	// 2. Add record and save
	rec := &manifest.ModRecord{
		ModID:       "sodium",
		ModName:     "Sodium",
		FileName:    "sodium-1.0.0.jar",
		Source:      "modrinth",
		VersionID:   "ver-1",
		ReleaseType: "release",
		InstalledAt: time.Now(),
	}
	m.AddOrUpdate(rec)

	if err := m.Save(tempDir); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}

	// 3. Reload and verify persistence
	loaded, err := manifest.LoadManifest(tempDir)
	if err != nil {
		t.Fatalf("load saved manifest failed: %v", err)
	}
	gotRec := loaded.GetRecord("sodium-1.0.0.jar")
	if gotRec == nil || gotRec.ModID != "sodium" || gotRec.ReleaseType != "release" {
		t.Fatalf("unexpected loaded record: %+v", gotRec)
	}
}

func TestManifest_SchemaVersionGuard(t *testing.T) {
	tempDir := t.TempDir()
	manifestFile := filepath.Join(tempDir, manifest.ManifestFileName)

	futureJSON := `{"schema_version": 999, "mods": {}}`
	if err := os.WriteFile(manifestFile, []byte(futureJSON), 0644); err != nil {
		t.Fatalf("failed to write future manifest: %v", err)
	}

	_, err := manifest.LoadManifest(tempDir)
	if err == nil || !errors.Is(err, manifest.ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion for schema 999, got %v", err)
	}
}

func TestManifest_ReconcileWithDisk(t *testing.T) {
	tempDir := t.TempDir()

	// Initial manifest with modA and modB
	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:    "mod-a",
		FileName: "mod-a-1.0.jar",
		Source:   "modrinth",
	})
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:    "mod-b",
		FileName: "mod-b-1.0.jar",
		Source:   "curseforge",
	})

	// Disk only has modA (renamed to disabled) and a new manual drop modC.jar
	// modB was deleted on disk!
	if err := os.WriteFile(filepath.Join(tempDir, "mod-a-1.0.jar.disabled"), []byte("dummy-a"), 0644); err != nil {
		t.Fatalf("write modA: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "mod-c-2.0.jar"), []byte("dummy-c"), 0644); err != nil {
		t.Fatalf("write modC: %v", err)
	}

	res, err := m.ReconcileWithDisk(tempDir)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if !res.Changed {
		t.Fatalf("expected changed=true from reconcile")
	}

	// Verify modB was purged
	if m.GetRecord("mod-b-1.0.jar") != nil {
		t.Errorf("expected modB to be removed after reconcile")
	}

	// Verify modA had its filename updated to .disabled
	recA := m.GetRecord("mod-a-1.0.jar")
	if recA == nil || recA.FileName != "mod-a-1.0.jar.disabled" {
		t.Errorf("expected modA filename updated to .disabled, got %+v", recA)
	}

	// Verify modC was synthesized from disk
	recC := m.GetRecord("mod-c-2.0.jar")
	if recC == nil || recC.Source != "local" {
		t.Errorf("expected synthesized local record for modC, got %+v", recC)
	}
}

func TestManifest_ReconcileWithDisk_DuplicateSelfHeal(t *testing.T) {
	tempDir := t.TempDir()

	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "modmenu",
		FileName:    "modmenu-11.0.4.jar",
		Source:      "modrinth",
		InstalledAt: time.Now().Add(-10 * time.Minute),
	})
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:       "modmenu",
		FileName:    "modmenu-11.0.5.jar",
		Source:      "modrinth",
		InstalledAt: time.Now(),
	})

	// Create both files on disk as active jars
	oldFile := filepath.Join(tempDir, "modmenu-11.0.4.jar")
	newFile := filepath.Join(tempDir, "modmenu-11.0.5.jar")
	if err := os.WriteFile(oldFile, []byte("modmenu-old"), 0644); err != nil {
		t.Fatalf("write old jar: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("modmenu-new"), 0644); err != nil {
		t.Fatalf("write new jar: %v", err)
	}

	res, err := m.ReconcileWithDisk(tempDir)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if !res.Changed {
		t.Errorf("expected changed=true on duplicate self-heal")
	}
	if len(res.DisabledDuplicates) != 1 || res.DisabledDuplicates[0] != "modmenu-11.0.4.jar" {
		t.Errorf("expected deleted duplicate ['modmenu-11.0.4.jar'], got: %v", res.DisabledDuplicates)
	}

	// Verify new file is still active
	if _, err := os.Stat(newFile); err != nil {
		t.Errorf("expected newer file to remain active, but stat failed: %v", err)
	}

	// Verify old file is DELETED (not renamed to .disabled)
	if _, err := os.Stat(oldFile); err == nil || !os.IsNotExist(err) {
		t.Errorf("expected old active jar to be deleted")
	}
	disabledOld := filepath.Join(tempDir, "modmenu-11.0.4.jar.disabled")
	if _, err := os.Stat(disabledOld); err == nil || !os.IsNotExist(err) {
		t.Errorf("expected old jar to not exist as .disabled")
	}

	// Idempotency: second reconcile should find 0 disabled duplicates
	res2, err := m.ReconcileWithDisk(tempDir)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if len(res2.DisabledDuplicates) != 0 {
		t.Errorf("expected 0 disabled duplicates on second run, got: %v", res2.DisabledDuplicates)
	}
}

func TestCanonicalModBase(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"modmenu-11.0.4.jar", "modmenu"},
		{"modmenu-11.0.5.jar.disabled", "modmenu"},
		{"fabric-api-0.92.0+1.20.4.jar", "fabric-api"},
		{"sodium-fabric-0.5.8.jar", "sodium-fabric"},
		{"OptiFine_1.20.4_HD_U_I7.jar", "optifine"},
		{"simplemod.jar", "simplemod"},
	}

	for _, c := range cases {
		got := manifest.CanonicalModBase(c.input)
		if got != c.expected {
			t.Errorf("CanonicalModBase(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestManifest_RemoveAndNilGuards(t *testing.T) {
	m := manifest.NewManifest()
	m.AddOrUpdate(&manifest.ModRecord{
		ModID:    "mod-x",
		FileName: "mod-x.jar",
	})
	if m.GetRecord("mod-x.jar") == nil {
		t.Fatalf("expected record to exist")
	}

	m.Remove("mod-x.jar")
	if m.GetRecord("mod-x.jar") != nil {
		t.Fatalf("expected record to be removed")
	}

	// Nil / empty guards
	m.AddOrUpdate(nil)
	m.AddOrUpdate(&manifest.ModRecord{})
	m.Remove("non-existent.jar")
}

