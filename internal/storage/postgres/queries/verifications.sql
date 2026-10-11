-- name: CreateVerification :one
INSERT INTO email_verifications (user_id, purpose, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetActiveVerification :one
SELECT * FROM email_verifications
WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementVerificationAttempts :one
UPDATE email_verifications SET attempts = attempts + 1
WHERE id = $1
RETURNING attempts;

-- name: ConsumeVerification :execrows
UPDATE email_verifications SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL;

-- name: CountRecentVerifications :one
SELECT count(*) FROM email_verifications
WHERE user_id = $1 AND purpose = $2 AND created_at > sqlc.arg(since)::timestamptz;

-- name: InvalidateVerifications :exec
UPDATE email_verifications SET consumed_at = now()
WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL;

-- name: AttemptVerification :one
-- Атомарно засчитывает попытку ввода кода: строка возвращается, только если код активен и лимит не исчерпан.
-- Параллельные запросы сериализуются блокировкой строки, поэтому лимит попыток нельзя обойти гонкой.
UPDATE email_verifications SET attempts = attempts + 1
WHERE id = (
  SELECT ev.id FROM email_verifications ev
  WHERE ev.user_id = $1 AND ev.purpose = $2 AND ev.consumed_at IS NULL AND ev.expires_at > now()
  ORDER BY ev.created_at DESC
  LIMIT 1
)
  AND consumed_at IS NULL
  AND attempts < sqlc.arg(max_attempts)::int
RETURNING id, code_hash, attempts;

-- name: LockUserCodes :exec
-- Сериализует выдачу кодов одному пользователю: подсчёт лимита и вставка идут под одной блокировкой.
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(user_id)::uuid::text, 7301));
