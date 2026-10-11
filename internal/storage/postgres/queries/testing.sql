-- name: CreateSurvey :one
INSERT INTO surveys (user_id, specialization, industry, primary_stack, claimed_grade, experience_years)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetLatestSurvey :one
SELECT * FROM surveys WHERE user_id = $1 AND specialization = $2 ORDER BY created_at DESC LIMIT 1;

-- name: ListSurveys :many
SELECT * FROM surveys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: UpsertItemFamily :exec
INSERT INTO item_families (id, spec, topic, grade, format, a, b, c, a_prior, b_prior)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $6, $7)
ON CONFLICT (id) DO UPDATE SET
  spec = EXCLUDED.spec, topic = EXCLUDED.topic, grade = EXCLUDED.grade, format = EXCLUDED.format,
  a_prior = EXCLUDED.a_prior, b_prior = EXCLUDED.b_prior,
  -- калиброванные значения сохраняются, пока автор не изменил априорные параметры в коде;
  -- новые априорные — калибровка начинается заново от них
  a = CASE WHEN item_families.a_prior = EXCLUDED.a_prior THEN item_families.a ELSE EXCLUDED.a END,
  b = CASE WHEN item_families.b_prior = EXCLUDED.b_prior THEN item_families.b ELSE EXCLUDED.b END,
  c = EXCLUDED.c,
  updated_at = now();

-- name: ListItemFamilies :many
SELECT * FROM item_families ORDER BY id;

-- name: ListItemFamiliesBySpec :many
SELECT * FROM item_families WHERE enabled AND (spec = $1 OR spec = 'common') ORDER BY id;

-- name: IncrementFamilyStats :exec
UPDATE item_families SET exposures = exposures + 1, corrects = corrects + CASE WHEN sqlc.arg(correct)::boolean THEN 1 ELSE 0 END
WHERE id = $1;

-- name: UpdateFamilyParams :exec
UPDATE item_families SET a = $2, b = $3, updated_at = now() WHERE id = $1;

-- name: SetFamilyEnabled :exec
UPDATE item_families SET enabled = $2, updated_at = now() WHERE id = $1;

-- name: GetExposureCounts :many
-- Число сессий специализации за 30 дней, в которых показывалось семейство (доля считается в Go).
SELECT ti.family_id, count(DISTINCT ti.session_id) AS sessions_with_family
FROM test_items ti
JOIN test_sessions ts ON ts.id = ti.session_id
WHERE ts.specialization = $1 AND ts.created_at > now() - interval '30 days'
GROUP BY ti.family_id;

-- name: CountRecentSessions :one
SELECT count(*) FROM test_sessions WHERE specialization = $1 AND created_at > now() - interval '30 days';

-- name: CreateTestSession :one
INSERT INTO test_sessions (user_id, survey_id, specialization, target_grade, seed, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTestSession :one
SELECT * FROM test_sessions WHERE id = $1;

-- name: GetActiveTestSession :one
SELECT * FROM test_sessions WHERE user_id = $1 AND status IN ('created', 'in_progress') LIMIT 1;

-- name: ListTestSessions :many
SELECT * FROM test_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50;

-- name: StartTestSession :exec
UPDATE test_sessions SET status = 'in_progress', started_at = COALESCE(started_at, now()) WHERE id = $1;

-- name: UpdateTestSessionProgress :exec
UPDATE test_sessions SET theta = $2, se = $3, items_count = $4, correct_count = $5 WHERE id = $1;

-- name: FinishTestSession :exec
UPDATE test_sessions SET status = $2, theta = $3, se = $4, decision = $5, percentile = $6, finished_at = now()
WHERE id = $1;

-- name: UpdateTestSessionFlags :exec
UPDATE test_sessions SET flags = $2 WHERE id = $1;

-- name: CreateTestItem :one
INSERT INTO test_items (session_id, position, family_id, seed, instance, deadline_at, theta_before)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListTestItems :many
SELECT * FROM test_items WHERE session_id = $1 ORDER BY position;

-- name: GetTestItem :one
SELECT * FROM test_items WHERE id = $1 AND session_id = $2;

-- name: AnswerTestItem :exec
UPDATE test_items SET answered_at = now(), submission = $3, is_correct = $4, time_spent_ms = $5, theta_after = $6, se_after = $7
WHERE id = $1 AND session_id = $2 AND answered_at IS NULL;

-- name: UpsertCandidateCategory :one
INSERT INTO candidate_categories (user_id, specialization, grade, status, is_primary, theta, se, percentile, last_session_id, confirmed_at, next_test_allowed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (user_id, specialization) DO UPDATE SET
  grade = EXCLUDED.grade, status = EXCLUDED.status, is_primary = candidate_categories.is_primary OR EXCLUDED.is_primary,
  theta = EXCLUDED.theta, se = EXCLUDED.se, percentile = EXCLUDED.percentile, last_session_id = EXCLUDED.last_session_id,
  confirmed_at = COALESCE(EXCLUDED.confirmed_at, candidate_categories.confirmed_at),
  next_test_allowed_at = EXCLUDED.next_test_allowed_at
RETURNING *;

-- name: GetCandidateCategory :one
SELECT * FROM candidate_categories WHERE user_id = $1 AND specialization = $2;

-- name: ListCandidateCategories :many
SELECT * FROM candidate_categories WHERE user_id = $1 ORDER BY is_primary DESC, updated_at DESC;

-- name: CountPrimaryCategories :one
SELECT count(*) FROM candidate_categories WHERE user_id = $1 AND is_primary;

-- name: CategoryPercentile :one
-- Доля кандидатов специализации с θ ниже заданной (среди имеющих оценку).
SELECT CAST(CASE WHEN count(*) = 0 THEN 0.5 ELSE CAST(count(*) FILTER (WHERE theta < sqlc.arg(theta)::numeric) AS double precision) / CAST(count(*) AS double precision) END AS double precision) AS percentile
FROM candidate_categories WHERE specialization = $1 AND theta IS NOT NULL;

-- name: InsertGradeHistory :exec
INSERT INTO grade_history (user_id, specialization, claimed_grade, decision, result_grade, theta, se, session_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListGradeHistory :many
SELECT * FROM grade_history WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100;

-- name: ConfirmSkillsByTopics :execrows
UPDATE candidate_skills cs SET source = 'test_confirmed', confirmed_session_id = sqlc.arg(session_id)::uuid
FROM skills s
WHERE cs.skill_id = s.id AND cs.user_id = $1 AND s.topic = ANY(sqlc.arg(topics)::text[]) AND cs.source <> 'test_confirmed';

-- name: SetProfileSpecializationIfEmpty :exec
UPDATE candidate_profiles SET primary_specialization = $2 WHERE user_id = $1 AND primary_specialization IS NULL;

-- name: ListCalibrationObservations :many
-- Ответы завершённых сессий с итоговой оценкой θ̂ отвечавшего — данные для калибровки сложности.
SELECT ti.family_id, ts.theta::float8 AS theta, ti.is_correct::boolean AS correct
FROM test_items ti
JOIN test_sessions ts ON ts.id = ti.session_id
WHERE ts.status = 'completed' AND ts.theta IS NOT NULL AND ti.is_correct IS NOT NULL;
