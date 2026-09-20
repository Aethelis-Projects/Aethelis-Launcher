package storage

import (
	"context"
	"time"

	"github.com/nord-launcher/launcher/internal/core/ports"
	corestorage "github.com/nord-launcher/launcher/internal/core/storage"
)

// ContentCacheAdapter adapts corestorage.ContentCacheRepository to the ports.ContentCache interface.
type ContentCacheAdapter struct {
	repo *corestorage.ContentCacheRepository
}

// NewContentCacheAdapter creates a new ContentCacheAdapter wrapping ContentCacheRepository.
func NewContentCacheAdapter(repo *corestorage.ContentCacheRepository) ports.ContentCache {
	return &ContentCacheAdapter{repo: repo}
}

func (a *ContentCacheAdapter) Get(ctx context.Context, kind, key string) (string, time.Time, bool, error) {
	if a.repo == nil {
		return "", time.Time{}, false, nil
	}
	return a.repo.Get(ctx, kind, key)
}

func (a *ContentCacheAdapter) Set(ctx context.Context, kind, key, payload string, ttl time.Duration) error {
	if a.repo == nil {
		return nil
	}
	return a.repo.Set(ctx, kind, key, payload, ttl)
}

func (a *ContentCacheAdapter) PruneExpired(ctx context.Context) error {
	if a.repo == nil {
		return nil
	}
	return a.repo.PruneExpired(ctx)
}

func (a *ContentCacheAdapter) Clear(ctx context.Context) error {
	if a.repo == nil {
		return nil
	}
	return a.repo.Clear(ctx)
}
