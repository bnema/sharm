-- name: GetUser :one
SELECT * FROM users WHERE username = ? LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: GetFirstUser :one
SELECT * FROM users LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: InsertUser :exec
INSERT INTO users (username, password_hash) VALUES (?, ?);

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ?, session_version = session_version + 1, updated_at = datetime('now') WHERE id = ?;

-- name: IncrementUserSessionVersion :exec
UPDATE users SET session_version = session_version + 1, updated_at = datetime('now') WHERE id = ?;
