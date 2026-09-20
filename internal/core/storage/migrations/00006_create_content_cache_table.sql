-- +goose Up
CREATE TABLE IF NOT EXISTS content_cache (
    kind TEXT NOT NULL,
    key TEXT NOT NULL,
    payload TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    PRIMARY KEY (kind, key)
);
CREATE INDEX IF NOT EXISTS idx_content_cache_expires_at ON content_cache(expires_at);

-- +goose Down
DROP TABLE IF EXISTS content_cache;
