-- +goose Up
CREATE TABLE data_sources (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL,
    type              TEXT NOT NULL,                  -- ical | ms_graph | ...
    config_json       TEXT NOT NULL DEFAULT '{}',
    secret_ciphertext TEXT NOT NULL DEFAULT '',       -- encrypted OAuth tokens (later)
    oauth_status      TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE widget_sources (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    widget_id      INTEGER NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    data_source_id INTEGER NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    filter         TEXT NOT NULL DEFAULT '', -- per-(widget,source) filter expression
    position       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX widget_sources_widget_idx ON widget_sources(widget_id, position);

-- +goose Down
DROP TABLE IF EXISTS widget_sources;
DROP TABLE IF EXISTS data_sources;
