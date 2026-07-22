-- name: CreatePlaylist :one
INSERT INTO playlists (name, is_default, default_dwell_seconds)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetPlaylist :one
SELECT * FROM playlists WHERE id = ?;

-- name: GetDefaultPlaylist :one
SELECT * FROM playlists WHERE is_default = 1 ORDER BY id LIMIT 1;

-- name: ListPlaylists :many
SELECT * FROM playlists ORDER BY name;

-- name: AddPlaylistItem :one
INSERT INTO playlist_items (playlist_id, view_id, position, dwell_seconds)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListPlaylistItems :many
SELECT * FROM playlist_items WHERE playlist_id = ? ORDER BY position, id;

-- name: SetDevicePlaylist :exec
UPDATE kiosk_devices SET playlist_id = ? WHERE id = ?;

-- name: SetDevicePip :exec
UPDATE kiosk_devices SET pip_playlist_id = ?, pip_config_json = ? WHERE id = ?;

-- name: UpdatePlaylist :exec
UPDATE playlists SET name = ?, default_dwell_seconds = ?, updated_at = datetime('now') WHERE id = ?;

-- name: DeletePlaylist :exec
DELETE FROM playlists WHERE id = ?;

-- name: ClearDefaultPlaylists :exec
UPDATE playlists SET is_default = 0;

-- name: SetDefaultPlaylist :exec
UPDATE playlists SET is_default = 1 WHERE id = ?;

-- name: GetPlaylistItem :one
SELECT * FROM playlist_items WHERE id = ?;

-- name: DeletePlaylistItem :exec
DELETE FROM playlist_items WHERE id = ?;

-- name: UpdatePlaylistItemPosition :exec
UPDATE playlist_items SET position = ? WHERE id = ?;

-- name: UpdatePlaylistItemDwell :exec
UPDATE playlist_items SET dwell_seconds = ? WHERE id = ?;

-- name: MaxPlaylistPosition :one
SELECT CAST(COALESCE(MAX(position), -1) AS INTEGER) FROM playlist_items WHERE playlist_id = ?;
