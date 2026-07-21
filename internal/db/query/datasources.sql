-- name: CreateDataSource :one
INSERT INTO data_sources (name, type, config_json, secret_ciphertext)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListDataSources :many
SELECT * FROM data_sources ORDER BY name;

-- name: GetDataSource :one
SELECT * FROM data_sources WHERE id = ?;

-- name: DeleteDataSource :exec
DELETE FROM data_sources WHERE id = ?;

-- name: UpdateDataSourceSecret :exec
UPDATE data_sources SET secret_ciphertext = ?, oauth_status = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateDataSourceConfig :exec
UPDATE data_sources SET config_json = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateDataSourceName :exec
UPDATE data_sources SET name = ?, updated_at = datetime('now') WHERE id = ?;

-- name: UpdateDataSourceHealth :exec
UPDATE data_sources
SET access_expiry = ?, last_error = ?, health = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: MarkDataSourceSynced :exec
UPDATE data_sources SET last_sync = datetime('now') WHERE id = ?;

-- name: AddWidgetSource :one
INSERT INTO widget_sources (widget_id, data_source_id, filter, position)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListWidgetSources :many
SELECT ws.id, ws.widget_id, ws.data_source_id, ws.filter, ws.resource, ws.color, ws.position,
       ds.name AS source_name, ds.type AS source_type, ds.config_json AS source_config,
       ds.secret_ciphertext AS source_secret
FROM widget_sources ws
JOIN data_sources ds ON ds.id = ws.data_source_id
WHERE ws.widget_id = ?
ORDER BY ws.position, ws.id;

-- name: UpdateWidgetSourceColor :exec
UPDATE widget_sources SET color = ? WHERE id = ?;

-- name: DeleteWidgetSource :exec
DELETE FROM widget_sources WHERE id = ?;

-- name: UpdateWidgetSourceResource :exec
UPDATE widget_sources SET resource = ? WHERE id = ?;

-- name: MaxWidgetSourcePosition :one
SELECT CAST(COALESCE(MAX(position), -1) AS INTEGER) FROM widget_sources WHERE widget_id = ?;
