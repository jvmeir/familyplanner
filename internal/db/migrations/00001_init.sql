-- +goose Up
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE views (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    cols           INTEGER NOT NULL DEFAULT 3,
    rows           INTEGER NOT NULL DEFAULT 2,
    theme_id       TEXT NOT NULL DEFAULT '',
    in_rotation    INTEGER NOT NULL DEFAULT 1,
    rotation_order INTEGER NOT NULL DEFAULT 0,
    dwell_seconds  INTEGER NOT NULL DEFAULT 30,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE widgets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE placements (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    view_id               INTEGER NOT NULL REFERENCES views(id) ON DELETE CASCADE,
    widget_id             INTEGER NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    col                   INTEGER NOT NULL DEFAULT 1,
    row                   INTEGER NOT NULL DEFAULT 1,
    col_span              INTEGER NOT NULL DEFAULT 1,
    row_span              INTEGER NOT NULL DEFAULT 1,
    placement_config_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE kiosk_devices (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen  TEXT NOT NULL DEFAULT ''
);

-- session store for alexedwards/scs
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expiry);

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS kiosk_devices;
DROP TABLE IF EXISTS placements;
DROP TABLE IF EXISTS widgets;
DROP TABLE IF EXISTS views;
DROP TABLE IF EXISTS settings;
