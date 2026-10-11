-- name: AdminUserCounts :many
SELECT role, status, count(*) AS total FROM users GROUP BY role, status;

-- name: AdminCategoryCounts :many
SELECT specialization, grade, status, count(*) AS total FROM candidate_categories GROUP BY specialization, grade, status;

-- name: AdminSessionStats :many
SELECT status, count(*) AS total,
       COALESCE(avg(items_count), 0)::double precision AS avg_items,
       COALESCE(avg(EXTRACT(EPOCH FROM (finished_at - started_at))), 0)::double precision AS avg_seconds,
       count(*) FILTER (WHERE flags <> '{}'::jsonb) AS flagged
FROM test_sessions GROUP BY status;

-- name: AdminSessionsBySpec :many
SELECT specialization, count(*) AS total FROM test_sessions WHERE status = 'completed' GROUP BY specialization;

-- name: AdminInvitationCounts :many
SELECT status, count(*) AS total FROM invitations GROUP BY status;

-- name: AdminApplicationCounts :many
SELECT status, count(*) AS total FROM applications GROUP BY status;

-- name: AdminVacancyCounts :many
SELECT status, count(*) AS total FROM vacancies GROUP BY status;

-- name: AdminNeedCounts :many
SELECT status, count(*) AS total FROM needs GROUP BY status;

-- name: AdminFunnel :one
SELECT
  (SELECT count(*) FROM users WHERE role = 'candidate') AS candidates,
  (SELECT count(DISTINCT user_id) FROM surveys) AS surveyed,
  (SELECT count(DISTINCT user_id) FROM test_sessions WHERE status = 'completed') AS tested,
  (SELECT count(DISTINCT user_id) FROM candidate_categories WHERE status IN ('confirmed', 'upgrade_suggested')) AS confirmed,
  (SELECT count(DISTINCT candidate_user_id) FROM invitations) AS invited,
  (SELECT count(DISTINCT candidate_user_id) FROM invitations WHERE status = 'accepted') AS accepted,
  (SELECT count(*) FROM companies) AS companies,
  (SELECT count(*) FROM users WHERE created_at >= now() - interval '7 days') AS new_users_7d,
  (SELECT count(*) FROM test_sessions WHERE created_at >= now() - interval '7 days') AS sessions_7d,
  (SELECT count(*) FROM invitations WHERE created_at >= now() - interval '7 days') AS invitations_7d;

-- name: AdminDailyActivity :many
SELECT d::date AS day,
       (SELECT count(*) FROM users u WHERE u.created_at::date = d::date) AS registrations,
       (SELECT count(*) FROM test_sessions t WHERE t.created_at::date = d::date) AS sessions,
       (SELECT count(*) FROM invitations i WHERE i.created_at::date = d::date) AS invitations,
       (SELECT count(*) FROM applications a WHERE a.sent_at::date = d::date) AS applications
FROM generate_series(now()::date - (sqlc.arg(days)::int - 1), now()::date, interval '1 day') AS d
ORDER BY day;

-- name: AdminListUsers :many
SELECT id, email, role, status, created_at, last_login_at, email_verified_at, count(*) OVER() AS total
FROM users
WHERE (sqlc.narg(q)::text IS NULL OR email ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(role)::user_role IS NULL OR role = sqlc.narg(role)::user_role)
  AND (sqlc.narg(status)::user_status IS NULL OR status = sqlc.narg(status)::user_status)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: AdminPromote :many
UPDATE users SET role = 'admin' WHERE email = ANY(sqlc.arg(emails)::text[]) AND role <> 'admin' RETURNING id, email;

-- name: AdminItemFamilyStats :many
SELECT f.id, f.spec, f.topic, f.grade, f.format, f.a, f.b, f.c, f.b_prior, f.exposures, f.corrects, f.enabled,
       count(ti.id) AS served,
       count(ti.id) FILTER (WHERE ti.answered_at IS NOT NULL) AS answered,
       count(ti.id) FILTER (WHERE ti.is_correct) AS correct,
       COALESCE(avg(ti.time_spent_ms) FILTER (WHERE ti.answered_at IS NOT NULL), 0)::double precision AS avg_time_ms,
       COALESCE(avg(ti.theta_before) FILTER (WHERE ti.is_correct), 0)::double precision AS theta_correct,
       COALESCE(avg(ti.theta_before) FILTER (WHERE ti.is_correct = false), 0)::double precision AS theta_incorrect,
       count(ti.id) FILTER (WHERE ti.is_correct) > 0 AND count(ti.id) FILTER (WHERE ti.is_correct = false) > 0 AS has_both
FROM item_families f LEFT JOIN test_items ti ON ti.family_id = f.id
GROUP BY f.id
ORDER BY f.spec, f.topic, f.grade, f.id;

-- name: AdminSetFamilyEnabled :exec
UPDATE item_families SET enabled = $2, updated_at = now() WHERE id = $1;

-- name: AdminFlaggedSessions :many
SELECT s.id, s.user_id, u.email, s.specialization, s.target_grade, s.status, s.items_count, s.correct_count, s.flags, s.started_at, s.finished_at, s.decision
FROM test_sessions s JOIN users u ON u.id = s.user_id
WHERE s.flags <> '{}'::jsonb
ORDER BY s.created_at DESC LIMIT sqlc.arg(lim);

-- name: AdminEmployers :many
SELECT c.id, c.name, c.industry, c.city, c.created_at,
       (SELECT count(*) FROM employer_users eu WHERE eu.company_id = c.id) AS users,
       (SELECT count(*) FROM needs n WHERE n.company_id = c.id) AS needs,
       (SELECT count(*) FROM vacancies v WHERE v.company_id = c.id) AS vacancies,
       (SELECT count(*) FROM invitations i WHERE i.company_id = c.id) AS invitations,
       (SELECT count(*) FROM invitations i WHERE i.company_id = c.id AND i.status = 'accepted') AS accepted,
       (SELECT count(*) FROM invitations i WHERE i.company_id = c.id AND i.status = 'declined') AS declined,
       (SELECT count(*) FROM invitations i WHERE i.company_id = c.id AND i.status = 'withdrawn') AS withdrawn
FROM companies c ORDER BY c.created_at DESC LIMIT 500;

-- name: AdminVacancies :many
SELECT v.id, v.title, v.status, v.specialization, v.grade_min, v.grade_max, v.salary_from, v.salary_to, v.published_at, v.created_at, c.name AS company_name, c.id AS company_id,
       (SELECT count(*) FROM applications a WHERE a.vacancy_id = v.id) AS applications
FROM vacancies v JOIN companies c ON c.id = v.company_id
ORDER BY v.created_at DESC LIMIT 500;

-- name: AdminAudit :many
SELECT a.id, a.actor_user_id, u.email AS actor_email, a.action, a.object_type, a.object_id, a.details, a.created_at, count(*) OVER() AS total
FROM audit_log a LEFT JOIN users u ON u.id = a.actor_user_id
WHERE (sqlc.narg(action)::text IS NULL OR a.action LIKE sqlc.narg(action)::text || '%')
  AND (sqlc.narg(actor)::text IS NULL OR u.email ILIKE '%' || sqlc.narg(actor)::text || '%')
ORDER BY a.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
