package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ContentCacheRepository manages persistent SQLite storage for content query results.
type ContentCacheRepository struct {
	db *sql.DB
}

// NewContentCacheRepository creates a new ContentCacheRepository using an existing Database instance.
func NewContentCacheRepository(db *Database) *ContentCacheRepository {
	return &ContentCacheRepository{db: db.DB()}
}

// NewContentCacheRepositoryFromDB creates a new ContentCacheRepository from a raw *sql.DB.
func NewContentCacheRepositoryFromDB(db *sql.DB) *ContentCacheRepository {
	return &ContentCacheRepository{db: db}
}

// Get retrieves a cached payload by kind and key. Returns ok=false if not found or expired.
func (r *ContentCacheRepository) Get(ctx context.Context, kind, key string) (string, time.Time, bool, error) {
	var payload string
	var expiresAt time.Time
	query := `SELECT payload, expires_at FROM content_cache WHERE kind = ? AND key = ?;`
	err := r.db.QueryRowContext(ctx, query, kind, key).Scan(&payload, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, fmt.Errorf("get content cache: %w", err)
	}
	return payload, expiresAt, true, nil
}

// Set stores a payload under kind and key with a specific time-to-live duration.
func (r *ContentCacheRepository) Set(ctx context.Context, kind, key, payload string, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)
	query := `
	INSERT INTO content_cache (kind, key, payload, expires_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(kind, key) DO UPDATE SET
		payload = excluded.payload,
		expires_at = excluded.expires_at;
	`
	_, err := r.db.ExecContext(ctx, query, kind, key, payload, expiresAt)
	if err != nil {
		return fmt.Errorf("set content cache: %w", err)
	}
	return nil
}

// PruneExpired removes all cache records that have expired relative to current time.
func (r *ContentCacheRepository) PruneExpired(ctx context.Context) error {
	query := `DELETE FROM content_cache WHERE expires_at <= ?;`
	_, err := r.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return fmt.Errorf("prune content cache: %w", err)
	}
	return nil
}

// Clear purges all records in the content_cache table.
func (r *ContentCacheRepository) Clear(ctx context.Context) error {
	query := `DELETE FROM content_cache;`
	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("clear content cache: %w", err)
	}
	return nil
}
