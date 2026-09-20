package storage_test

import (
	"context"
	"testing"
	"time"

	storageadapter "github.com/nord-launcher/launcher/internal/adapters/storage"
	corestorage "github.com/nord-launcher/launcher/internal/core/storage"
)

func TestContentCacheAdapter(t *testing.T) {
	db, err := corestorage.OpenDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	repo := corestorage.NewContentCacheRepository(db)
	adapter := storageadapter.NewContentCacheAdapter(repo)
	ctx := context.Background()

	// Test Set & Get through adapter
	err = adapter.Set(ctx, "search", "sodium", `{"hits":[]}`, 2*time.Hour)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	payload, expiresAt, ok, err := adapter.Get(ctx, "search", "sodium")
	if err != nil || !ok || payload != `{"hits":[]}` {
		t.Fatalf("Get failed: ok=%v payload=%q err=%v", ok, payload, err)
	}
	if expiresAt.Before(time.Now()) {
		t.Fatalf("expected future expiresAt, got %v", expiresAt)
	}

	// Test Prune & Clear
	if err := adapter.PruneExpired(ctx); err != nil {
		t.Fatalf("PruneExpired failed: %v", err)
	}
	if err := adapter.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	_, _, ok, err = adapter.Get(ctx, "search", "sodium")
	if err != nil || ok {
		t.Fatalf("expected cleared cache, got ok=%v err=%v", ok, err)
	}

	// Nil repo safety check
	nilAdapter := storageadapter.NewContentCacheAdapter(nil)
	_, _, ok, err = nilAdapter.Get(ctx, "kind", "key")
	if err != nil || ok {
		t.Fatalf("nilAdapter.Get failed: %v", err)
	}
	if err := nilAdapter.Set(ctx, "k", "k", "p", time.Hour); err != nil {
		t.Fatalf("nilAdapter.Set failed: %v", err)
	}
	if err := nilAdapter.PruneExpired(ctx); err != nil {
		t.Fatalf("nilAdapter.PruneExpired failed: %v", err)
	}
	if err := nilAdapter.Clear(ctx); err != nil {
		t.Fatalf("nilAdapter.Clear failed: %v", err)
	}
}
