-- name: InsertVariant :one
INSERT INTO media_variants (media_id, codec, status, created_at)
VALUES (?, ?, 'pending', datetime('now'))
RETURNING *;

-- name: GetVariant :one
SELECT * FROM media_variants WHERE id = ? LIMIT 1;

-- name: GetVariantByMediaAndCodec :one
SELECT * FROM media_variants WHERE media_id = ? AND codec = ? LIMIT 1;

-- name: ListVariantsByMedia :many
SELECT * FROM media_variants WHERE media_id = ? ORDER BY created_at ASC;

-- name: UpdateVariantStatus :exec
UPDATE media_variants SET status = ?, error_message = ? WHERE id = ?;

-- name: UpdateVariantDone :exec
UPDATE media_variants SET
    status = 'done',
    path = ?,
    file_size = ?,
    width = ?,
    height = ?
WHERE id = ?;

-- name: DeleteVariantsByMedia :exec
DELETE FROM media_variants WHERE media_id = ?;
