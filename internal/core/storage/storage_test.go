package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

func TestStorage_SQLiteWALAndCRUD(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_launcher.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	defer db.Close()

	// 1. Verify PRAGMAs (WAL mode & FK=ON)
	var journalMode string
	if err := db.DB().QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode 'wal', got %q", journalMode)
	}

	var foreignKeys int
	if err := db.DB().QueryRow("PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		t.Fatalf("failed to query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("expected foreign_keys 1, got %d", foreignKeys)
	}

	// 2. Test InstanceRepository
	instRepo := storage.NewInstanceRepository(db)
	now := time.Now().Truncate(time.Second)

	testInst := &domain.Instance{
		ID:           "test-inst-1",
		Name:         "Nordic Fabric",
		GameVersion:  "1.21.1",
		Loader:       domain.LoaderFabric,
		LoaderVer:    "0.16.5",
		MinRAMMB:     2048,
		MaxRAMMB:     4096,
		JVMArgs:      []string{"-XX:+UseG1GC", "-Dnord=true"},
		State:        domain.StateIdle,
		TotalPlaySec: 120,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := instRepo.Save(ctx, testInst); err != nil {
		t.Fatalf("failed to save instance: %v", err)
	}

	fetched, err := instRepo.GetByID(ctx, "test-inst-1")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if fetched.Name != "Nordic Fabric" || len(fetched.JVMArgs) != 2 || fetched.JVMArgs[0] != "-XX:+UseG1GC" {
		t.Fatalf("unexpected fetched instance: %+v", fetched)
	}

	if err := instRepo.UpdateState(ctx, "test-inst-1", domain.StateRunning); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	list, err := instRepo.ListAll(ctx)
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(list) != 1 || list[0].State != domain.StateRunning {
		t.Fatalf("expected 1 running instance, got %+v", list)
	}

	// 3. Test AccountRepository
	accRepo := storage.NewAccountRepository(db)
	acc1 := &domain.Account{
		UUID:      "uuid-1111",
		Username:  "PlayerOne",
		Type:      domain.AccountMicrosoft,
		ExpiresAt: now.Add(24 * time.Hour),
		IsActive:  false,
	}
	acc2 := &domain.Account{
		UUID:      "uuid-2222",
		Username:  "PlayerTwo",
		Type:      domain.AccountOffline,
		ExpiresAt: now.Add(24 * time.Hour),
		IsActive:  false,
	}

	if err := accRepo.Save(ctx, acc1); err != nil {
		t.Fatalf("failed to save acc1: %v", err)
	}
	if err := accRepo.Save(ctx, acc2); err != nil {
		t.Fatalf("failed to save acc2: %v", err)
	}

	if err := accRepo.SetActive(ctx, "uuid-2222"); err != nil {
		t.Fatalf("failed to set active account: %v", err)
	}

	active, err := accRepo.GetByUUID(ctx, "uuid-2222")
	if err != nil {
		t.Fatalf("failed to get active account: %v", err)
	}
	if !active.IsActive {
		t.Fatal("expected uuid-2222 to be active")
	}

	other, err := accRepo.GetByUUID(ctx, "uuid-1111")
	if err != nil {
		t.Fatalf("failed to get other account: %v", err)
	}
	if other.IsActive {
		t.Fatal("expected uuid-1111 to be inactive")
	}

	// Test AccountRepository GetActive, ListAll, Delete, and errors
	activeAcc, err := accRepo.GetActive(ctx)
	if err != nil || activeAcc.UUID != "uuid-2222" {
		t.Fatalf("expected active account uuid-2222, got %+v, err: %v", activeAcc, err)
	}

	allAccs, err := accRepo.ListAll(ctx)
	if err != nil || len(allAccs) != 2 {
		t.Fatalf("expected 2 accounts, got %d, err: %v", len(allAccs), err)
	}

	if err := accRepo.SetActive(ctx, "non-existent-uuid"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}

	_, err = accRepo.GetByUUID(ctx, "non-existent-uuid")
	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound on non-existent get, got %v", err)
	}

	if err := accRepo.Delete(ctx, "uuid-1111"); err != nil {
		t.Fatalf("failed to delete account: %v", err)
	}
	remainingAccs, err := accRepo.ListAll(ctx)
	if err != nil || len(remainingAccs) != 1 {
		t.Fatalf("expected 1 remaining account, got %d", len(remainingAccs))
	}

	// 4. Test SettingsRepository
	settingsRepo := storage.NewSettingsRepository(db)
	if err := settingsRepo.Set(ctx, "theme", "nordic-dark"); err != nil {
		t.Fatalf("failed to save setting: %v", err)
	}
	if err := settingsRepo.Set(ctx, "update_channel", "stable"); err != nil {
		t.Fatalf("failed to save setting: %v", err)
	}

	theme, err := settingsRepo.Get(ctx, "theme")
	if err != nil || theme != "nordic-dark" {
		t.Fatalf("expected 'nordic-dark', got %q, err: %v", theme, err)
	}

	val, err := settingsRepo.Get(ctx, "non-existent-key")
	if err != nil || val != "" {
		t.Fatalf("expected empty string and nil error on non-existent setting get, got %q, err: %v", val, err)
	}

	allSettings, err := settingsRepo.GetAll(ctx)
	if err != nil || len(allSettings) != 2 {
		t.Fatalf("expected 2 settings, got %+v", allSettings)
	}

	// 5. Test Instance with LastPlayedAt and Delete & Not Found
	playedAt := now.Add(-1 * time.Hour)
	testInst.LastPlayedAt = &playedAt
	if err := instRepo.Save(ctx, testInst); err != nil {
		t.Fatalf("failed to update instance with LastPlayedAt: %v", err)
	}
	fetchedWithPlayedAt, err := instRepo.GetByID(ctx, "test-inst-1")
	if err != nil || fetchedWithPlayedAt.LastPlayedAt == nil {
		t.Fatalf("expected instance with LastPlayedAt, got %+v, err: %v", fetchedWithPlayedAt, err)
	}

	if err := instRepo.Delete(ctx, "test-inst-1"); err != nil {
		t.Fatalf("failed to delete instance: %v", err)
	}
	_, err = instRepo.GetByID(ctx, "test-inst-1")
	if !errors.Is(err, domain.ErrInstanceNotFound) {
		t.Fatalf("expected ErrInstanceNotFound, got %v", err)
	}
}

func TestOpenDatabase_And_Migrate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migrate_test.db")

	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
}

func TestInstanceRepository_EnsureDefaultInstance(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "seed_test.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	instRepo := storage.NewInstanceRepository(db)

	// 1. Initial run on empty DB seeds Nordic Fabric 1.21 (N8)
	seeded, err := instRepo.EnsureDefaultInstance(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultInstance failed: %v", err)
	}
	if seeded == nil || seeded.Name != "Nordic Fabric 1.21" || seeded.Loader != domain.LoaderFabric {
		t.Fatalf("unexpected seeded instance: %+v", seeded)
	}

	list, err := instRepo.ListAll(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d (err: %v)", len(list), err)
	}

	// 2. Subsequent run on non-empty DB is a no-op (strictly when COUNT == 0)
	secondCall, err := instRepo.EnsureDefaultInstance(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultInstance second call failed: %v", err)
	}
	if secondCall != nil {
		t.Fatalf("expected nil when instances already exist, got: %+v", secondCall)
	}

	listAfter, err := instRepo.ListAll(ctx)
	if err != nil || len(listAfter) != 1 {
		t.Fatalf("expected still exactly 1 instance, got %d", len(listAfter))
	}
}