-- +goose Up
-- Per-data-source refresh interval (seconds). 0 = use the global default
-- (settings key "refresh_interval_secs", itself defaulting to 900 = 15 min).
ALTER TABLE data_sources ADD COLUMN refresh_interval_secs INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE data_sources DROP COLUMN refresh_interval_secs;
