-- name: CreateConsent :one
INSERT INTO consents (user_id, kind, version, ip, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActiveConsent :one
SELECT * FROM consents
WHERE user_id = $1 AND kind = $2 AND withdrawn_at IS NULL
ORDER BY accepted_at DESC
LIMIT 1;

-- name: ListUserConsents :many
SELECT * FROM consents WHERE user_id = $1 ORDER BY accepted_at DESC;

-- name: WithdrawConsent :execrows
UPDATE consents SET withdrawn_at = now()
WHERE user_id = $1 AND kind = $2 AND withdrawn_at IS NULL;
