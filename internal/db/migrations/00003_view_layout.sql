-- +goose Up
-- Recursive split/merge layout tree (JSON). Empty string = use the legacy grid.
ALTER TABLE views ADD COLUMN layout_json TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE views DROP COLUMN layout_json;
