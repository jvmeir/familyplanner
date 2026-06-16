-- Schema used by sqlc for type generation. Keep in sync with migrations/.
-- (The sessions table is managed by scs and intentionally omitted here.)

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
    layout_json    TEXT NOT NULL DEFAULT '',
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
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL DEFAULT '',
    token_hash  TEXT NOT NULL UNIQUE,
    playlist_id INTEGER NOT NULL DEFAULT 0, -- 0 = unassigned (use default playlist)
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen   TEXT NOT NULL DEFAULT ''
);

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
    dwell_seconds INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE widget_cache (
    widget_id  INTEGER PRIMARY KEY REFERENCES widgets(id) ON DELETE CASCADE,
    data_json  TEXT NOT NULL DEFAULT 'null',
    fetched_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL DEFAULT (datetime('now')),
    status     TEXT NOT NULL DEFAULT 'ok',
    error_msg  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE data_sources (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL,
    type              TEXT NOT NULL,
    config_json       TEXT NOT NULL DEFAULT '{}',
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    oauth_status      TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE widget_sources (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    widget_id      INTEGER NOT NULL REFERENCES widgets(id) ON DELETE CASCADE,
    data_source_id INTEGER NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    filter         TEXT NOT NULL DEFAULT '',
    resource       TEXT NOT NULL DEFAULT '',
    position       INTEGER NOT NULL DEFAULT 0
);
