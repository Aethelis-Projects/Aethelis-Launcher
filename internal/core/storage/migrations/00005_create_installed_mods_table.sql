-- +goose Up
CREATE TABLE IF NOT EXISTS installed_mods (
    instance_id TEXT NOT NULL,
    mod_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    version_id TEXT NOT NULL DEFAULT '',
    release_type TEXT NOT NULL DEFAULT '',
    installed_at DATETIME NOT NULL,
    PRIMARY KEY (instance_id, file_name)
);
CREATE INDEX IF NOT EXISTS idx_installed_mods_instance ON installed_mods(instance_id);

-- +goose Down
DROP TABLE IF EXISTS installed_mods;
