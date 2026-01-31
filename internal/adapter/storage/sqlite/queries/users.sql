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
UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?;
