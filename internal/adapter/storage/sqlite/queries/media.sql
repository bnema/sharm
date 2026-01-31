-- name: GetMedia :one
SELECT * FROM media WHERE id = ? LIMIT 1;

-- name: ListAllMedia :many
SELECT * FROM media ORDER BY created_at DESC;

-- name: ListExpiredMedia :many
SELECT * FROM media WHERE expires_at < datetime('now');

-- name: ListMediaByStatus :many
SELECT * FROM media WHERE status = ? ORDER BY created_at DESC;

-- name: InsertMedia :exec
INSERT INTO media (
    id, type, original_name, original_path, converted_path,
    status, codec, error_message, retention_days, file_size,
    width, height, thumb_path, created_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateMediaStatus :exec
UPDATE media SET status = ?, error_message = ? WHERE id = ?;

-- name: UpdateMediaDone :exec
UPDATE media SET
    status = 'done',
    converted_path = ?,
    codec = ?,
    width = ?,
    height = ?,
    thumb_path = ?,
    file_size = ?
WHERE id = ?;

-- name: DeleteMedia :exec
DELETE FROM media WHERE id = ?;
