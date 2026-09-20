package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/storage"
)

func TestContentCacheRepository_CRUD_And_Expiry(t *testing.T) {
	db, err := storage.OpenDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	repo := storage.NewContentCacheRepository(db)
	ctx := context.Background()

	// 1. Miss on empty cache
	payload, _, ok, err := repo.Get(ctx, "search", "jei|1.21.1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if ok || payload != "" {
		t.Fatalf("expected miss on empty cache, got ok=%v payload=%q", ok, payload)
	}

	// 2. Set item with 1 hour TTL
	err = repo.Set(ctx, "search", "jei|1.21.1", `{"data":["item1"]}`, time.Hour)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// 3. Hit on cached item
	payload, expiresAt, ok, err := repo.Get(ctx, "search", "jei|1.21.1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !ok || payload != `{"data":["item1"]}` {
		t.Fatalf("expected hit with correct payload, got ok=%v payload=%q", ok, payload)
	}
	if expiresAt.Before(time.Now()) {
		t.Fatalf("expected future expiresAt, got %v", expiresAt)
	}

	// 4. Overwrite (upsert) existing item
	err = repo.Set(ctx, "search", "jei|1.21.1", `{"data":["item1_updated"]}`, time.Hour)
	if err != nil {
		t.Fatalf("Set update error: %v", err)
	}
	payload, _, ok, err = repo.Get(ctx, "search", "jei|1.21.1")
	if err != nil || !ok || payload != `{"data":["item1_updated"]}` {
		t.Fatalf("expected updated payload, got ok=%v payload=%q err=%v", ok, payload, err)
	}

	// 5. Test PruneExpired
	// Insert an expired item (negative TTL)
	err = repo.Set(ctx, "files", "101|1.21.1", `{"files":[]}`, -10*time.Minute)
	if err != nil {
		t.Fatalf("Set expired item error: %v", err)
	}

	err = repo.PruneExpired(ctx)
	if err != nil {
		t.Fatalf("PruneExpired error: %v", err)
	}

	// The expired item should have been deleted
	_, _, ok, err = repo.Get(ctx, "files", "101|1.21.1")
	if err != nil {
		t.Fatalf("Get pruned error: %v", err)
	}
	if ok {
		t.Fatalf("expected pruned item to be missing")
	}

	// The valid item should still be present
	_, _, ok, err = repo.Get(ctx, "search", "jei|1.21.1")
	if err != nil || !ok {
		t.Fatalf("expected valid item to remain, got ok=%v err=%v", ok, err)
	}

	// 6. Test Clear
	if err := repo.Clear(ctx); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	_, _, ok, err = repo.Get(ctx, "search", "jei|1.21.1")
	if err != nil || ok {
		t.Fatalf("expected cache to be completely cleared, got ok=%v", ok)
	}
}
