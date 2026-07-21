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

-- name: ClearWidgetCache :exec
DELETE FROM widget_cache;

-- name: ListExpiredWidgetIDs :many
SELECT id FROM widgets
WHERE id NOT IN (SELECT widget_id FROM widget_cache)
   OR id IN (SELECT widget_id FROM widget_cache WHERE expires_at <= datetime('now'));

-- name: ListWidgetHealth :many
SELECT w.id AS widget_id, w.name AS widget_name, w.type AS widget_type,
       COALESCE(wc.status, '') AS status,
       COALESCE(wc.error_msg, '') AS error_msg,
       COALESCE(wc.fetched_at, '') AS fetched_at
FROM widgets w
LEFT JOIN widget_cache wc ON wc.widget_id = w.id
ORDER BY w.name;
