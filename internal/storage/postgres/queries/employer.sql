-- name: CreateCompany :one
INSERT INTO companies (name, inn, description, industry, website, city, public_contacts)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: UpdateCompany :one
UPDATE companies SET name = $2, inn = $3, description = $4, industry = $5, website = $6, city = $7, public_contacts = $8
WHERE id = $1
RETURNING *;

-- name: CreateEmployerUser :exec
INSERT INTO employer_users (user_id, company_id, company_role, position) VALUES ($1, $2, $3, $4);

-- name: GetEmployerUser :one
SELECT * FROM employer_users WHERE user_id = $1;

-- name: CreateNeed :one
INSERT INTO needs (company_id, created_by, title, description, team_description, specialization, grade_min, grade_max,
                   must_skill_ids, nice_skill_ids, salary_from, salary_to, work_formats, city, remote_ok, status, extraction)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: GetNeed :one
SELECT * FROM needs WHERE id = $1;

-- name: ListNeeds :many
SELECT * FROM needs WHERE company_id = $1 ORDER BY created_at DESC;

-- name: UpdateNeed :one
UPDATE needs SET title = $2, description = $3, team_description = $4, specialization = $5, grade_min = $6, grade_max = $7,
  must_skill_ids = $8, nice_skill_ids = $9, salary_from = $10, salary_to = $11, work_formats = $12, city = $13, remote_ok = $14
WHERE id = $1
RETURNING *;

-- name: SetNeedStatus :exec
UPDATE needs SET status = $2 WHERE id = $1;

-- name: GetOrCreateSelection :one
INSERT INTO selections (need_id) VALUES ($1)
ON CONFLICT (need_id) DO UPDATE SET need_id = EXCLUDED.need_id
RETURNING *;

-- name: NextSelectionVersionNo :one
SELECT COALESCE(max(version_no), 0) + 1 AS next_no FROM selection_versions WHERE selection_id = $1;

-- name: CreateSelectionVersion :one
INSERT INTO selection_versions (selection_id, version_no, filters, weights, category_map, items_count, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListSelectionVersions :many
SELECT * FROM selection_versions WHERE selection_id = $1 ORDER BY version_no DESC;

-- name: GetSelectionVersion :one
SELECT * FROM selection_versions WHERE selection_id = $1 AND version_no = $2;

-- name: GetLatestSelectionVersion :one
SELECT * FROM selection_versions WHERE selection_id = $1 ORDER BY version_no DESC LIMIT 1;

-- name: InsertSelectionItems :copyfrom
INSERT INTO selection_items (version_id, candidate_user_id, rank, score, relevance, p_accept, components, explanation, grade_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListSelectionItems :many
SELECT * FROM selection_items WHERE version_id = $1 ORDER BY rank;

-- name: ListMatchingCandidates :many
-- Кандидаты категорий специализации, доступные для подборки: не скрыты, с действующим согласием на публикацию.
SELECT cc.user_id, cc.grade, cc.status AS grade_status, cc.theta, cc.se, cc.percentile, cc.updated_at AS category_updated_at, cc.confirmed_at,
       cp.first_name, cp.last_name, cp.city, cp.work_formats, cp.relocation, cp.salary_min, cp.salary_expected, cp.visibility,
       cp.headline, cp.experience_years,
       COALESCE(ps.show_full_name, true) AS show_full_name, COALESCE(ps.show_salary_expected, true) AS show_salary_expected,
       COALESCE(ps.show_city, true) AS show_city, COALESCE(ps.show_experience, true) AS show_experience, COALESCE(ps.show_fsp, true) AS show_fsp,
       EXISTS (SELECT 1 FROM fsp_links fl WHERE fl.user_id = cc.user_id) AS fsp_linked
FROM candidate_categories cc
JOIN candidate_profiles cp ON cp.user_id = cc.user_id
JOIN users u ON u.id = cc.user_id AND u.status = 'active'
LEFT JOIN privacy_settings ps ON ps.user_id = cc.user_id
WHERE cc.specialization = $1
  AND cc.grade = ANY(sqlc.arg(grades)::grade_code[])
  AND cp.visibility <> 'hidden'
  AND EXISTS (SELECT 1 FROM consents c WHERE c.user_id = cc.user_id AND c.kind = 'publication' AND c.withdrawn_at IS NULL);

-- name: ListCandidateSkillsForUsers :many
SELECT cs.user_id, cs.skill_id, cs.source, s.code, s.name
FROM candidate_skills cs JOIN skills s ON s.id = cs.skill_id
WHERE cs.user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: ListFSPAchievementsForUsers :many
SELECT user_id, discipline, level, result_kind, place, event_date, event_name
FROM fsp_achievements WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: ListInvitationHistoryForCompany :many
SELECT candidate_user_id, status, decline_reason, salary_to
FROM invitations WHERE company_id = $1 AND candidate_user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: GetCandidateCard :one
SELECT cc.user_id, cc.specialization, cc.grade, cc.status AS grade_status, cc.theta, cc.se, cc.percentile, cc.confirmed_at, cc.updated_at AS category_updated_at,
       cp.first_name, cp.last_name, cp.middle_name, cp.city, cp.work_formats, cp.relocation, cp.salary_expected, cp.visibility,
       cp.headline, cp.about, cp.experience_years, cp.primary_specialization,
       COALESCE(ps.show_full_name, true) AS show_full_name, COALESCE(ps.show_salary_expected, true) AS show_salary_expected,
       COALESCE(ps.show_city, true) AS show_city, COALESCE(ps.show_experience, true) AS show_experience, COALESCE(ps.show_fsp, true) AS show_fsp
FROM candidate_profiles cp
JOIN users u ON u.id = cp.user_id AND u.status = 'active'
LEFT JOIN candidate_categories cc ON cc.user_id = cp.user_id AND cc.is_primary
LEFT JOIN privacy_settings ps ON ps.user_id = cp.user_id
WHERE cp.user_id = $1 AND cp.visibility <> 'hidden'
  AND EXISTS (SELECT 1 FROM consents c WHERE c.user_id = cp.user_id AND c.kind = 'publication' AND c.withdrawn_at IS NULL);

-- name: CreateContactGrant :one
INSERT INTO contact_grants (candidate_user_id, company_id, source, source_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (candidate_user_id, company_id) WHERE revoked_at IS NULL DO UPDATE SET source = EXCLUDED.source
RETURNING *;

-- name: GetActiveContactGrant :one
SELECT * FROM contact_grants WHERE candidate_user_id = $1 AND company_id = $2 AND revoked_at IS NULL;

-- name: ContactsForCompany :one
-- Контакты кандидата выдаются только при активном гранте компании (docs/adr/0008).
SELECT cc.phone, cc.telegram, cc.email, cc.other
FROM candidate_contacts cc
WHERE cc.user_id = $1
  AND EXISTS (SELECT 1 FROM contact_grants g WHERE g.candidate_user_id = $1 AND g.company_id = $2 AND g.revoked_at IS NULL)
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = $1 AND u.status = 'active');

-- name: CreateInvitation :one
INSERT INTO invitations (company_id, need_id, vacancy_id, candidate_user_id, created_by, message, salary_from, salary_to, contact_method, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetInvitation :one
SELECT * FROM invitations WHERE id = $1;

-- name: ListInvitationsByCompany :many
SELECT * FROM invitations WHERE company_id = $1 AND (sqlc.narg(need_id)::uuid IS NULL OR need_id = sqlc.narg(need_id)::uuid)
ORDER BY created_at DESC LIMIT 200;

-- name: ListInvitationsByCandidate :many
SELECT i.*, c.name AS company_name, c.industry AS company_industry, c.city AS company_city
FROM invitations i JOIN companies c ON c.id = i.company_id
WHERE i.candidate_user_id = $1 ORDER BY i.created_at DESC LIMIT 200;

-- name: UpdateInvitationStatus :exec
UPDATE invitations SET status = $2,
  viewed_at = CASE WHEN $2::invitation_status = 'viewed' THEN COALESCE(viewed_at, now()) ELSE viewed_at END,
  responded_at = CASE WHEN $2::invitation_status IN ('accepted', 'declined') THEN now() ELSE responded_at END,
  withdrawn_at = CASE WHEN $2::invitation_status = 'withdrawn' THEN now() ELSE withdrawn_at END,
  decline_reason = $3, decline_comment = $4
WHERE id = $1;

-- name: ExpireInvitations :execrows
UPDATE invitations SET status = 'expired' WHERE status IN ('sent', 'viewed') AND expires_at < now();

-- name: CountActiveInvitationsForNeed :one
SELECT count(*) FROM invitations WHERE need_id = $1 AND status IN ('sent', 'viewed');

-- name: CountInvitationsSentToday :one
SELECT count(*) FROM invitations WHERE company_id = $1 AND created_at > now() - interval '24 hours';

-- name: CompanyInvitationStats :one
SELECT count(*) AS total,
       count(*) FILTER (WHERE status = 'accepted') AS accepted,
       count(*) FILTER (WHERE status = 'declined') AS declined,
       count(*) FILTER (WHERE created_at > now() - interval '30 days') AS last_30d,
       count(*) FILTER (WHERE status = 'accepted' AND created_at > now() - interval '30 days') AS accepted_30d
FROM invitations WHERE company_id = $1;

-- name: DeclineReasonsForNeed :many
SELECT decline_reason, count(*) AS cnt FROM invitations
WHERE need_id = $1 AND status = 'declined' AND decline_reason IS NOT NULL
GROUP BY decline_reason ORDER BY cnt DESC;

-- name: GetCompanyPublicContacts :one
SELECT name, public_contacts FROM companies WHERE id = $1;
