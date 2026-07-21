-- name: CountViews :one
SELECT count(*) FROM views;

-- name: ListViews :many
SELECT * FROM views ORDER BY rotation_order, id;

-- name: ListRotationViews :many
SELECT * FROM views WHERE in_rotation = 1 ORDER BY rotation_order, id;

-- name: GetView :one
SELECT * FROM views WHERE id = ?;

-- name: CreateView :one
INSERT INTO views (name, cols, rows, theme_id, in_rotation, rotation_order, dwell_seconds)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: SetViewLayout :exec
UPDATE views SET layout_json = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateView :exec
UPDATE views SET name = ?, cols = ?, rows = ?, theme_id = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateViewName :exec
UPDATE views SET name = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateViewMode :exec
UPDATE views SET render_mode = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateViewAdvance :exec
UPDATE views SET advance_mode = ?, updated_at = datetime('now') WHERE id = ?;

-- name: DeleteView :exec
DELETE FROM views WHERE id = ?;
