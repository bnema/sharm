-- name: GetJob :one
SELECT * FROM jobs WHERE id = ? LIMIT 1;

-- name: ListJobsByMedia :many
SELECT * FROM jobs WHERE media_id = ? ORDER BY created_at ASC;

-- name: ListPendingJobs :many
SELECT * FROM jobs WHERE status = 'pending' ORDER BY created_at ASC;

-- name: GetActiveJob :one
SELECT * FROM jobs
WHERE media_id = ? AND type = ? AND codec = ? AND status IN ('pending', 'running')
ORDER BY created_at ASC
LIMIT 1;

-- name: InsertJob :one
INSERT INTO jobs (media_id, type, codec, fps, status, created_at)
VALUES (?, ?, ?, ?, 'pending', datetime('now'))
RETURNING *;

-- The five-minute lease must remain longer than service.jobHeartbeatInterval.
-- name: ClaimNextJob :one
UPDATE jobs SET
    status = 'running',
    started_at = datetime('now'),
    heartbeat_at = datetime('now'),
    lease_until = datetime('now', '+5 minutes'),
    attempts = attempts + 1,
    progress = CASE WHEN progress < 1 THEN 1 ELSE progress END
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'pending'
    ORDER BY created_at ASC
    LIMIT 1
)
RETURNING *;

-- name: CompleteJob :execrows
UPDATE jobs SET
    status = 'done',
    progress = 100,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = datetime('now')
WHERE id = ? AND status = 'running';

-- name: UpdateJobProgress :execrows
UPDATE jobs SET progress = ?, heartbeat_at = datetime('now'), lease_until = datetime('now', '+5 minutes')
WHERE id = ? AND status = 'running';

-- name: HeartbeatJob :execrows
UPDATE jobs SET heartbeat_at = datetime('now'), lease_until = datetime('now', '+5 minutes')
WHERE id = ? AND status = 'running';

-- name: FailJob :execrows
UPDATE jobs SET
    status = 'failed',
    error_message = ?,
    lease_until = NULL,
    heartbeat_at = NULL,
    completed_at = datetime('now')
WHERE id = ? AND status = 'running';

-- name: ListExhaustedStalledJobs :many
SELECT * FROM jobs
WHERE status = 'running'
  AND attempts >= max_attempts
  AND (lease_until IS NULL OR lease_until <= datetime('now'));

-- name: ResetStalledJobs :exec
UPDATE jobs SET
    status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
    started_at = CASE WHEN attempts >= max_attempts THEN started_at ELSE NULL END,
    progress = CASE WHEN attempts >= max_attempts THEN progress ELSE 0 END,
    lease_until = NULL,
    heartbeat_at = NULL,
    error_message = CASE WHEN attempts >= max_attempts THEN 'job exceeded retry limit' ELSE error_message END,
    completed_at = CASE WHEN attempts >= max_attempts THEN datetime('now') ELSE completed_at END
WHERE status = 'running'
  AND (lease_until IS NULL OR lease_until <= datetime('now'));
