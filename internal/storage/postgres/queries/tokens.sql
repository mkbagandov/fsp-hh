-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :execrows
DELETE FROM refresh_tokens WHERE expires_at < now() - interval '30 days';

-- name: RotateRefreshToken :execrows
-- Отзыв при ротации: должна измениться ровно одна строка, иначе токен уже использован параллельным запросом.
UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL;
