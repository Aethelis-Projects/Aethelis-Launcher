package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/storage"
)

func TestInstalledModsRepository_CRUDAndSync(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "nord_test.db")

	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	repo := storage.NewInstalledModsRepository(db)
	if repo == nil {
		t.Fatalf("expected non-nil repo")
	}

	ctx := context.Background()
	instID := "test-instance-1"

	// 1. Save
	rec1 := storage.InstalledModRecord{
		InstanceID:  instID,
		ModID:       "sodium",
		FileName:    "sodium-mc1.20.1-0.5.8.jar",
		Source:      "modrinth",
		VersionID:   "v0.5.8",
		ReleaseType: "release",
		InstalledAt: time.Now(),
	}
	if err := repo.Save(ctx, rec1); err != nil {
		t.Fatalf("failed to save mod: %v", err)
	}

	rec2 := storage.InstalledModRecord{
		InstanceID:  instID,
		ModID:       "iris",
		FileName:    "iris-mc1.20.1-1.6.4.jar",
		Source:      "curseforge",
		VersionID:   "v1.6.4",
		ReleaseType: "release",
		InstalledAt: time.Now(),
	}
	if err := repo.Save(ctx, rec2); err != nil {
		t.Fatalf("failed to save mod 2: %v", err)
	}

	// 2. Query
	list, err := repo.GetByInstance(ctx, instID)
	if err != nil {
		t.Fatalf("failed to get mods by instance: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(list))
	}

	// 3. Update file name (e.g. toggle to .disabled)
	if err := repo.UpdateFileName(ctx, instID, "sodium-mc1.20.1-0.5.8.jar", "sodium-mc1.20.1-0.5.8.jar.disabled"); err != nil {
		t.Fatalf("failed to update file name: %v", err)
	}
	listAfterToggle, err := repo.GetByInstance(ctx, instID)
	if err != nil {
		t.Fatalf("failed to get mods: %v", err)
	}
	foundDisabled := false
	for _, m := range listAfterToggle {
		if m.FileName == "sodium-mc1.20.1-0.5.8.jar.disabled" {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatalf("expected updated disabled filename in repo")
	}

	// 4. DeleteByCleanName
	if err := repo.DeleteByCleanName(ctx, instID, "sodium-mc1.20.1-0.5.8"); err != nil {
		t.Fatalf("failed to delete by clean name: %v", err)
	}
	listAfterDel, _ := repo.GetByInstance(ctx, instID)
	if len(listAfterDel) != 1 || listAfterDel[0].ModID != "iris" {
		t.Fatalf("expected only iris remaining, got %+v", listAfterDel)
	}

	// 5. SyncInstance
	if err := repo.SyncInstance(ctx, instID, []string{"iris-mc1.20.1-1.6.4.jar"}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	listAfterSync, _ := repo.GetByInstance(ctx, instID)
	if len(listAfterSync) != 1 {
		t.Fatalf("expected 1 mod after sync, got %d", len(listAfterSync))
	}

	// Prune all via empty sync
	if err := repo.SyncInstance(ctx, instID, []string{}); err != nil {
		t.Fatalf("empty sync failed: %v", err)
	}
	listEmpty, _ := repo.GetByInstance(ctx, instID)
	if len(listEmpty) != 0 {
		t.Fatalf("expected 0 mods after empty sync, got %d", len(listEmpty))
	}
}
