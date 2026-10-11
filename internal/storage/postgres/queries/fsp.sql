-- name: CreateFSPLink :one
INSERT INTO fsp_links (user_id, fsp_id, issuer, subject, consent_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFSPLinkByUser :one
SELECT * FROM fsp_links WHERE user_id = $1;

-- name: GetFSPLinkBySubject :one
SELECT * FROM fsp_links WHERE issuer = $1 AND subject = $2;

-- name: GetFSPLinkByFSPID :one
SELECT * FROM fsp_links WHERE fsp_id = $1;

-- name: DeleteFSPLink :execrows
DELETE FROM fsp_links WHERE user_id = $1;

-- name: SetFSPLinkSynced :exec
UPDATE fsp_links SET last_sync_at = now(), sync_error = $2 WHERE user_id = $1;

-- name: UpsertFSPAchievement :exec
INSERT INTO fsp_achievements (user_id, external_id, discipline, level, event_name, event_date, result_kind, place, rank_title, rating, role, team_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (user_id, external_id) DO UPDATE SET
  discipline = EXCLUDED.discipline, level = EXCLUDED.level, event_name = EXCLUDED.event_name,
  event_date = EXCLUDED.event_date, result_kind = EXCLUDED.result_kind, place = EXCLUDED.place,
  rank_title = EXCLUDED.rank_title, rating = EXCLUDED.rating, role = EXCLUDED.role,
  team_name = EXCLUDED.team_name, fetched_at = now();

-- name: ListFSPAchievements :many
SELECT * FROM fsp_achievements WHERE user_id = $1 ORDER BY event_date DESC;

-- name: DeleteFSPAchievements :exec
DELETE FROM fsp_achievements WHERE user_id = $1;

-- name: DeleteFSPAchievementsNotIn :exec
DELETE FROM fsp_achievements WHERE user_id = $1 AND NOT (external_id = ANY(sqlc.arg(keep)::text[]));
