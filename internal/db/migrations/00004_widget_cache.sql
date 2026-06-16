-- +goose Up
CREATE TABLE widget_cache (
    widget_id  INTEGER PRIMARY KEY REFERENCES widgets(id) ON DELETE CASCADE,
    data_json  TEXT NOT NULL DEFAULT 'null',
    fetched_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL DEFAULT (datetime('now')),
    status     TEXT NOT NULL DEFAULT 'ok', -- ok | stale | error
    error_msg  TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS widget_cache;
