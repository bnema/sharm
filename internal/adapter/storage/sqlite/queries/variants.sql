-- name: InsertVariant :one
INSERT INTO media_variants (media_id, codec, status, created_at)
VALUES (?, ?, 'pending', datetime('now'))
RETURNING *;

-- name: DemotePrimaryVariants :exec
UPDATE media_variants SET is_primary = 0 WHERE media_id = ? AND is_primary = 1;

-- name: InsertPrimaryVariant :one
INSERT INTO media_variants (
    media_id, codec, container, video_codec, audio_codec, has_audio,
    profile, level, mime_type, origin, is_primary, progress, duration_seconds,
    path, file_size, width, height, status, error_message, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, 'done', '', ?)
ON CONFLICT (media_id, codec) DO UPDATE SET
    container = excluded.container,
    video_codec = excluded.video_codec,
    audio_codec = excluded.audio_codec,
    has_audio = excluded.has_audio,
    profile = excluded.profile,
    level = excluded.level,
    mime_type = excluded.mime_type,
    origin = excluded.origin,
    is_primary = 1,
    progress = excluded.progress,
    duration_seconds = excluded.duration_seconds,
    path = excluded.path,
    file_size = excluded.file_size,
    width = excluded.width,
    height = excluded.height,
    status = excluded.status,
    error_message = excluded.error_message
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
    height = ?,
    progress = 100,
    error_message = '',
    is_primary = ?,
    origin = ?
WHERE id = ?;

-- name: UpdateVariantProgress :exec
UPDATE media_variants SET
    status = ?,
    progress = ?,
    error_message = ?
WHERE id = ? AND status NOT IN ('done', 'failed', 'unsupported');

-- name: DeleteVariantsByMedia :exec
DELETE FROM media_variants WHERE media_id = ?;
