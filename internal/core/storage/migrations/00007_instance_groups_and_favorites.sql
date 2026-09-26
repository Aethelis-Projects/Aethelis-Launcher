-- +goose Up
ALTER TABLE instances ADD COLUMN group_name TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN is_favorite INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE instances DROP COLUMN group_name;
ALTER TABLE instances DROP COLUMN is_favorite;
