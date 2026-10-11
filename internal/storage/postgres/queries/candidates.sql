-- name: EnsureCandidateProfile :exec
INSERT INTO candidate_profiles (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING;

-- name: EnsureCandidateContacts :exec
INSERT INTO candidate_contacts (user_id, email) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING;

-- name: EnsurePrivacySettings :exec
INSERT INTO privacy_settings (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING;

-- name: GetCandidateProfile :one
SELECT * FROM candidate_profiles WHERE user_id = $1;

-- name: UpdateCandidateProfile :one
UPDATE candidate_profiles SET
  first_name = $2,
  last_name = $3,
  middle_name = $4,
  city = $5,
  work_formats = $6,
  relocation = $7,
  salary_min = $8,
  salary_expected = $9,
  visibility = $10,
  headline = $11,
  about = $12,
  experience_years = $13,
  primary_specialization = $14,
  links = $15
WHERE user_id = $1
RETURNING *;

-- name: GetCandidateContacts :one
SELECT * FROM candidate_contacts WHERE user_id = $1;

-- name: UpdateCandidateContacts :one
UPDATE candidate_contacts SET phone = $2, telegram = $3, email = $4, other = $5
WHERE user_id = $1
RETURNING *;

-- name: GetPrivacySettings :one
SELECT * FROM privacy_settings WHERE user_id = $1;

-- name: UpdatePrivacySettings :one
UPDATE privacy_settings SET
  show_full_name = $2, show_experience = $3, show_fsp = $4, show_salary_expected = $5, show_city = $6
WHERE user_id = $1
RETURNING *;

-- name: ListCandidateExperiences :many
SELECT * FROM candidate_experiences WHERE user_id = $1 ORDER BY sort_order, started_on DESC;

-- name: ReplaceCandidateExperiencesDelete :exec
DELETE FROM candidate_experiences WHERE user_id = $1;

-- name: InsertCandidateExperience :one
INSERT INTO candidate_experiences (user_id, company_name, position, specialization, started_on, ended_on, description, stack, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListCandidateEducation :many
SELECT * FROM candidate_education WHERE user_id = $1 ORDER BY graduated_year DESC NULLS LAST;

-- name: DeleteCandidateEducation :exec
DELETE FROM candidate_education WHERE user_id = $1;

-- name: InsertCandidateEducation :one
INSERT INTO candidate_education (user_id, institution, degree, field, graduated_year)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListCandidateSkills :many
SELECT cs.user_id, cs.skill_id, cs.level, cs.source, cs.confirmed_session_id, s.code, s.name, s.group_code
FROM candidate_skills cs JOIN skills s ON s.id = cs.skill_id
WHERE cs.user_id = $1
ORDER BY s.group_code, s.name;

-- name: UpsertCandidateSkill :exec
INSERT INTO candidate_skills (user_id, skill_id, level, source, confirmed_session_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, skill_id) DO UPDATE
  SET level = COALESCE(EXCLUDED.level, candidate_skills.level),
      -- подтверждение тестом не понижается до «заявлен»
      source = CASE WHEN candidate_skills.source = 'test_confirmed' THEN candidate_skills.source ELSE EXCLUDED.source END,
      confirmed_session_id = COALESCE(EXCLUDED.confirmed_session_id, candidate_skills.confirmed_session_id);

-- name: DeleteCandidateSkill :exec
DELETE FROM candidate_skills WHERE user_id = $1 AND skill_id = $2;

-- name: DeleteCandidateSkillsNotIn :exec
DELETE FROM candidate_skills
WHERE user_id = $1 AND source <> 'test_confirmed' AND NOT (skill_id = ANY(sqlc.arg(keep_ids)::int[]));

-- name: CreateResumeUpload :one
INSERT INTO resume_uploads (user_id, file_name, mime, size_bytes, storage_path)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: SetResumeParsed :exec
UPDATE resume_uploads SET parsed_at = now(), parse_result = $2, parser = $3 WHERE id = $1;

-- name: GetResumeUpload :one
SELECT * FROM resume_uploads WHERE id = $1 AND user_id = $2;
