-- name: GetWidgetCache :one
SELECT * FROM widget_cache WHERE widget_id = ?;

-- name: UpsertWidgetCache :exec
INSERT INTO widget_cache (widget_id, data_json, fetched_at, expires_at, status, error_msg)
VALUES (?, ?, datetime('now'), ?, ?, ?)
ON CONFLICT(widget_id) DO UPDATE SET
    data_json = excluded.data_json,
    fetched_at = excluded.fetched_at,
    expires_at = excluded.expires_at,
    status = excluded.status,
    error_msg = excluded.error_msg;

-- name: MarkWidgetCacheStale :exec
UPDATE widget_cache SET status = 'stale', error_msg = ? WHERE widget_id = ?;

-- name: ListExpiredWidgetIDs :many
SELECT id FROM widgets
WHERE id NOT IN (SELECT widget_id FROM widget_cache)
   OR id IN (SELECT widget_id FROM widget_cache WHERE expires_at <= datetime('now'));
