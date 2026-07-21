-- +goose Up
-- Per-playlist corner picture-in-picture video: which video widget supplies the
-- videos, plus its presentation (corner/size/interval/muted) as JSON.
ALTER TABLE playlists ADD COLUMN pip_widget_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE playlists ADD COLUMN pip_config_json TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE playlists DROP COLUMN pip_config_json;
ALTER TABLE playlists DROP COLUMN pip_widget_id;
