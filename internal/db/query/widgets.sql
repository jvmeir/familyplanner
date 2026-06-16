-- name: GetWidget :one
SELECT * FROM widgets WHERE id = ?;

-- name: CreateWidget :one
INSERT INTO widgets (name, type, config_json)
VALUES (?, ?, ?)
RETURNING *;

-- name: ListWidgets :many
SELECT * FROM widgets ORDER BY name;

-- name: UpdateWidget :exec
UPDATE widgets SET name = ?, config_json = ?, updated_at = datetime('now') WHERE id = ?;

-- name: DeleteWidget :exec
DELETE FROM widgets WHERE id = ?;
