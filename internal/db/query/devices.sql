-- name: CreateDevice :one
INSERT INTO kiosk_devices (name, token_hash)
VALUES (?, ?)
RETURNING *;

-- name: GetDeviceByTokenHash :one
SELECT * FROM kiosk_devices WHERE token_hash = ?;

-- name: TouchDevice :exec
UPDATE kiosk_devices SET last_seen = ? WHERE id = ?;

-- name: ListDevices :many
SELECT * FROM kiosk_devices ORDER BY id;

-- name: GetDevice :one
SELECT * FROM kiosk_devices WHERE id = ?;

-- name: DeleteDevice :exec
DELETE FROM kiosk_devices WHERE id = ?;

-- name: UpdateDeviceName :exec
UPDATE kiosk_devices SET name = ? WHERE id = ?;
