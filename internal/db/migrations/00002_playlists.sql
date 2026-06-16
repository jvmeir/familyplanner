-- +goose Up
CREATE TABLE playlists (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT NOT NULL,
    is_default            INTEGER NOT NULL DEFAULT 0,
    default_dwell_seconds INTEGER NOT NULL DEFAULT 30,
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE playlist_items (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id   INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    view_id       INTEGER NOT NULL REFERENCES views(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL DEFAULT 0,
    dwell_seconds INTEGER NOT NULL DEFAULT 0 -- 0 = use the playlist's default_dwell_seconds
);
CREATE INDEX playlist_items_playlist_idx ON playlist_items(playlist_id, position);

-- 0 = unassigned -> device shows the default playlist.
ALTER TABLE kiosk_devices ADD COLUMN playlist_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE kiosk_devices DROP COLUMN playlist_id;
DROP TABLE IF EXISTS playlist_items;
DROP TABLE IF EXISTS playlists;
