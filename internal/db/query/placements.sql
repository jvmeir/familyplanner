-- name: ListPlacementsByView :many
SELECT * FROM placements WHERE view_id = ? ORDER BY row, col;

-- name: CreatePlacement :one
INSERT INTO placements (view_id, widget_id, col, row, col_span, row_span, placement_config_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;
