-- name: GetMediaAssetByMediaAndRole :one
SELECT * FROM media_assets
WHERE media_id = ? AND role = ?
LIMIT 1;

-- name: ListMediaAssets :many
SELECT * FROM media_assets
WHERE media_id = ?
ORDER BY created_at ASC, id ASC;

-- name: InsertMediaAsset :one
INSERT INTO media_assets (
    id, media_id, role, filename, path, size_bytes, sha256, status, error_message, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateMediaAsset :exec
UPDATE media_assets SET
    filename = ?,
    path = ?,
    size_bytes = ?,
    sha256 = ?,
    status = ?,
    error_message = ?,
    updated_at = ?
WHERE media_id = ? AND role = ?;

-- name: UpsertMediaAsset :exec
INSERT INTO media_assets (
    id, media_id, role, filename, path, size_bytes, sha256, status, error_message, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (media_id, role) DO UPDATE SET
    filename = excluded.filename,
    path = excluded.path,
    size_bytes = excluded.size_bytes,
    sha256 = excluded.sha256,
    status = excluded.status,
    error_message = excluded.error_message,
    updated_at = excluded.updated_at;

-- name: DeleteMediaAssetByRole :exec
DELETE FROM media_assets
WHERE media_id = ? AND role = ?;

-- name: DeleteMediaAssetsByMedia :exec
DELETE FROM media_assets
WHERE media_id = ?;
