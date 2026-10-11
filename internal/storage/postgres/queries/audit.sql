-- name: InsertAudit :exec
INSERT INTO audit_log (actor_user_id, action, object_type, object_id, details, ip)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAudit :many
SELECT * FROM audit_log
WHERE (sqlc.narg(object_type)::text IS NULL OR object_type = sqlc.narg(object_type)::text)
  AND (sqlc.narg(object_id)::uuid IS NULL OR object_id = sqlc.narg(object_id)::uuid)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
