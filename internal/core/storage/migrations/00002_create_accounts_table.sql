-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    uuid TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    type TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_accounts_active ON accounts(is_active);

-- +goose Down
DROP TABLE IF EXISTS accounts;