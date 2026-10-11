-- name: CreateUser :one
INSERT INTO users (email, password_hash, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: SetEmailVerified :exec
UPDATE users SET email_verified_at = now() WHERE id = $1 AND email_verified_at IS NULL;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1;

-- name: CountUsersByRole :one
SELECT count(*) FROM users WHERE role = $1 AND status = 'active';
