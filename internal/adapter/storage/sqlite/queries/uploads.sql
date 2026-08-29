-- name: GetUploadSession :one
SELECT * FROM upload_sessions
WHERE id = ?
LIMIT 1;

-- name: GetUploadAssetsBySession :many
SELECT * FROM upload_assets
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: GetUploadAsset :one
SELECT * FROM upload_assets
WHERE id = ?
LIMIT 1;

-- name: GetUploadAssetBySessionAndRole :one
SELECT * FROM upload_assets
WHERE session_id = ? AND role = ?
LIMIT 1;

-- name: GetUploadChunk :one
SELECT * FROM upload_chunks
WHERE asset_id = ? AND chunk_index = ?
LIMIT 1;

-- name: ListUploadChunks :many
SELECT * FROM upload_chunks
WHERE asset_id = ?
ORDER BY chunk_index ASC;

-- name: InsertUploadSession :execrows
INSERT INTO upload_sessions (
    id, media_id, user_id, filename, retention_days, keep_original,
    expected_bytes, reserved_bytes, status, expires_at, created_at, updated_at
)
SELECT sqlc.arg(id), sqlc.arg(media_id), sqlc.arg(user_id), sqlc.arg(filename),
       sqlc.arg(retention_days), sqlc.arg(keep_original), sqlc.arg(expected_bytes),
       sqlc.arg(reserved_bytes), sqlc.arg(status), sqlc.arg(expires_at),
       sqlc.arg(created_at), sqlc.arg(updated_at)
WHERE sqlc.arg(reserved_bytes) <= sqlc.arg(max_reserved_bytes) - (
    SELECT COALESCE(SUM(upload_sessions.reserved_bytes), 0)
    FROM upload_sessions
    WHERE user_id = sqlc.arg(user_id)
      AND status IN ('active', 'failed', 'expired', 'canceled')
);

-- name: InsertUploadAsset :exec
INSERT INTO upload_assets (
    id, session_id, media_id, role, filename, expected_size, chunk_size,
    total_chunks, expected_sha256, status, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertUploadChunk :execrows
INSERT INTO upload_chunks (asset_id, chunk_index, size_bytes, sha256, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (asset_id, chunk_index) DO NOTHING;

-- name: IncrementUploadAssetReceived :exec
UPDATE upload_assets SET
    received_bytes = received_bytes + ?,
    updated_at = ?
WHERE id = ? AND status = 'uploading';

-- name: ClaimUploadAssetFinalization :execrows
UPDATE upload_assets SET status = 'finalizing', updated_at = ?
WHERE id = ? AND status = 'uploading';

-- name: ReleaseUploadAssetFinalization :exec
UPDATE upload_assets SET
    status = 'uploading',
    error_message = ?,
    updated_at = ?
WHERE id = ? AND status = 'finalizing';

-- name: CompleteUploadAsset :execrows
UPDATE upload_assets SET
    received_bytes = ?,
    sha256 = ?,
    status = 'available',
    path = ?,
    error_message = '',
    updated_at = ?,
    completed_at = ?
WHERE id = ? AND status = 'finalizing';

-- name: FailUploadAsset :execrows
UPDATE upload_assets SET
    status = 'failed',
    error_message = ?,
    updated_at = ?
WHERE id = ? AND status IN ('uploading', 'finalizing');

-- name: UpdateUploadSessionStatus :exec
UPDATE upload_sessions SET status = ?, updated_at = ?
WHERE id = ?;

-- name: ListExpiredUploadSessions :many
SELECT * FROM upload_sessions
WHERE expires_at <= ? AND status IN ('active', 'failed', 'expired', 'canceled');

-- name: DeleteUploadSession :exec
DELETE FROM upload_sessions
WHERE id = ?;
