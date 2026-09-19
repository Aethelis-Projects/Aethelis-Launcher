-- +goose Up
ALTER TABLE instances ADD COLUMN skip_java_check INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE instances DROP COLUMN skip_java_check;
