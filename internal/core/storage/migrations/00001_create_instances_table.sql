-- +goose Up
CREATE TABLE IF NOT EXISTS instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    game_version TEXT NOT NULL,
    loader TEXT NOT NULL,
    loader_version TEXT NOT NULL DEFAULT '',
    icon_path TEXT NOT NULL DEFAULT '',
    java_path TEXT NOT NULL DEFAULT '',
    min_ram_mb INTEGER NOT NULL DEFAULT 2048,
    max_ram_mb INTEGER NOT NULL DEFAULT 4096,
    jvm_args TEXT NOT NULL DEFAULT '[]',
    state TEXT NOT NULL DEFAULT 'idle',
    last_played_at DATETIME,
    total_play_seconds INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_instances_game_version ON instances(game_version);

-- +goose Down
DROP TABLE IF EXISTS instances;