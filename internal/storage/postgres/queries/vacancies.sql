-- name: CreateVacancy :one
INSERT INTO vacancies (company_id, created_by, need_id, title, description, specialization, grade_min, grade_max, must_skill_ids, nice_skill_ids, salary_from, salary_to, work_formats, city, remote_ok)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: UpdateVacancy :one
UPDATE vacancies SET title = $2, description = $3, specialization = $4, grade_min = $5, grade_max = $6, must_skill_ids = $7, nice_skill_ids = $8,
  salary_from = $9, salary_to = $10, work_formats = $11, city = $12, remote_ok = $13, need_id = $14
WHERE id = $1
RETURNING *;

-- name: SetVacancyStatus :exec
UPDATE vacancies SET status = $2,
  published_at = CASE WHEN $2::vacancy_status = 'published' THEN COALESCE(published_at, now()) ELSE published_at END,
  closed_at = CASE WHEN $2::vacancy_status = 'closed' THEN now() ELSE closed_at END
WHERE id = $1;

-- name: GetVacancy :one
SELECT * FROM vacancies WHERE id = $1;

-- name: ListVacanciesByCompany :many
SELECT * FROM vacancies WHERE company_id = $1 ORDER BY created_at DESC;

-- name: ListPublicVacancies :many
SELECT v.*, c.name AS company_name, c.industry AS company_industry, c.city AS company_city, count(*) OVER() AS total
FROM vacancies v JOIN companies c ON c.id = v.company_id
WHERE v.status = 'published'
  AND (sqlc.narg(specialization)::specialization_code IS NULL OR v.specialization = sqlc.narg(specialization)::specialization_code)
  AND (sqlc.narg(grade)::grade_code IS NULL OR (v.grade_min <= sqlc.narg(grade)::grade_code AND v.grade_max >= sqlc.narg(grade)::grade_code))
  AND (sqlc.narg(work_format)::work_format IS NULL OR sqlc.narg(work_format)::work_format = ANY(v.work_formats))
  AND (sqlc.narg(city)::text IS NULL OR v.city ILIKE sqlc.narg(city)::text OR v.remote_ok)
  AND (sqlc.narg(salary_min)::int IS NULL OR v.salary_to >= sqlc.narg(salary_min)::int)
  AND (sqlc.narg(q)::text IS NULL OR v.title ILIKE '%' || sqlc.narg(q)::text || '%' OR c.name ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY v.published_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetPublicVacancy :one
SELECT v.*, c.name AS company_name, c.industry AS company_industry, c.city AS company_city, c.description AS company_description, c.website AS company_website
FROM vacancies v JOIN companies c ON c.id = v.company_id
WHERE v.id = $1 AND v.status = 'published';

-- name: CountApplicationsForVacancies :many
SELECT vacancy_id, count(*) AS total, count(*) FILTER (WHERE status = 'sent') AS new_count
FROM applications WHERE vacancy_id = ANY(sqlc.arg(vacancy_ids)::uuid[])
GROUP BY vacancy_id;

-- name: CreateApplication :one
INSERT INTO applications (vacancy_id, candidate_user_id, cover_letter) VALUES ($1, $2, $3) RETURNING *;

-- name: GetApplication :one
SELECT * FROM applications WHERE id = $1;

-- name: GetApplicationByVacancyAndCandidate :one
SELECT * FROM applications WHERE vacancy_id = $1 AND candidate_user_id = $2;

-- name: UpdateApplicationStatus :exec
UPDATE applications SET status = $2,
  viewed_at = CASE WHEN $2::application_status = 'viewed' THEN COALESCE(viewed_at, now()) ELSE viewed_at END,
  responded_at = CASE WHEN $2::application_status IN ('invited', 'rejected') THEN now() ELSE responded_at END,
  withdrawn_at = CASE WHEN $2::application_status = 'withdrawn' THEN now() ELSE withdrawn_at END,
  reject_reason = COALESCE(sqlc.narg(reject_reason)::text, reject_reason),
  invitation_id = COALESCE(sqlc.narg(invitation_id)::uuid, invitation_id)
WHERE id = $1;

-- name: ListApplicationsByCandidate :many
SELECT a.*, v.title AS vacancy_title, v.status AS vacancy_status, v.salary_from AS vacancy_salary_from, v.salary_to AS vacancy_salary_to,
       v.specialization AS vacancy_specialization, c.id AS company_id, c.name AS company_name
FROM applications a JOIN vacancies v ON v.id = a.vacancy_id JOIN companies c ON c.id = v.company_id
WHERE a.candidate_user_id = $1 ORDER BY a.sent_at DESC LIMIT 200;

-- name: ListApplicationsByVacancy :many
SELECT a.*, cp.first_name, cp.last_name, cp.headline, cp.city, cp.salary_expected, cp.experience_years,
       COALESCE(ps.show_full_name, true) AS show_full_name, COALESCE(ps.show_city, true) AS show_city,
       COALESCE(ps.show_salary_expected, true) AS show_salary, COALESCE(ps.show_experience, true) AS show_experience,
       cc.grade, cc.status AS grade_status, cc.percentile
FROM applications a
JOIN vacancies v ON v.id = a.vacancy_id
JOIN candidate_profiles cp ON cp.user_id = a.candidate_user_id
LEFT JOIN privacy_settings ps ON ps.user_id = a.candidate_user_id
LEFT JOIN candidate_categories cc ON cc.user_id = a.candidate_user_id AND cc.specialization = v.specialization
WHERE a.vacancy_id = $1 ORDER BY a.sent_at DESC;

-- name: GetCandidateCardAny :one
-- Карточка без фильтра видимости и согласия: только для компании с активным грантом (отклик или принятое приглашение).
SELECT cc.user_id, cc.specialization, cc.grade, cc.status AS grade_status, cc.theta, cc.se, cc.percentile, cc.confirmed_at, cc.updated_at AS category_updated_at,
       cp.first_name, cp.last_name, cp.middle_name, cp.city, cp.work_formats, cp.relocation, cp.salary_expected, cp.visibility,
       cp.headline, cp.about, cp.experience_years, cp.primary_specialization,
       COALESCE(ps.show_full_name, true) AS show_full_name, COALESCE(ps.show_salary_expected, true) AS show_salary_expected,
       COALESCE(ps.show_city, true) AS show_city, COALESCE(ps.show_experience, true) AS show_experience, COALESCE(ps.show_fsp, true) AS show_fsp
FROM candidate_profiles cp
JOIN users u ON u.id = cp.user_id AND u.status = 'active'
LEFT JOIN candidate_categories cc ON cc.user_id = cp.user_id AND cc.is_primary
LEFT JOIN privacy_settings ps ON ps.user_id = cp.user_id
WHERE cp.user_id = $1;

-- name: CreateNotification :one
INSERT INTO notifications (user_id, kind, title, body, link, payload) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListNotifications :many
SELECT * FROM notifications WHERE user_id = $1 AND (NOT sqlc.arg(unread_only)::bool OR read_at IS NULL)
ORDER BY created_at DESC LIMIT sqlc.arg(lim);

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;

-- name: ListCompanyUserIDs :many
SELECT user_id FROM employer_users WHERE company_id = $1;
