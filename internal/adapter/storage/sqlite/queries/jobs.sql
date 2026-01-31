-- name: GetJob :one
SELECT * FROM jobs WHERE id = ? LIMIT 1;

-- name: ListJobsByMedia :many
SELECT * FROM jobs WHERE media_id = ? ORDER BY created_at ASC;

-- name: ListPendingJobs :many
SELECT * FROM jobs WHERE status = 'pending' ORDER BY created_at ASC;

-- name: InsertJob :one
INSERT INTO jobs (media_id, type, codec, fps, status, created_at)
VALUES (?, ?, ?, ?, 'pending', datetime('now'))
RETURNING *;

-- name: ClaimNextJob :one
UPDATE jobs SET
    status = 'running',
    started_at = datetime('now'),
    attempts = attempts + 1
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'pending'
    ORDER BY created_at ASC
    LIMIT 1
)
RETURNING *;

-- name: CompleteJob :exec
UPDATE jobs SET
    status = 'done',
    completed_at = datetime('now')
WHERE id = ?;

-- name: FailJob :exec
UPDATE jobs SET
    status = 'failed',
    error_message = ?,
    completed_at = datetime('now')
WHERE id = ?;

-- name: ResetStalledJobs :exec
UPDATE jobs SET
    status = 'pending',
    started_at = NULL
WHERE status = 'running';
