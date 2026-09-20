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

	changed, err := m.ReconcileWithDisk(tempDir)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if !changed {
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
