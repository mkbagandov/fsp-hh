-- name: UpsertSkill :one
INSERT INTO skills (code, name, group_code, specs, topic, synonyms)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (code) DO UPDATE
  SET name = EXCLUDED.name,
      group_code = EXCLUDED.group_code,
      specs = EXCLUDED.specs,
      topic = EXCLUDED.topic,
      synonyms = EXCLUDED.synonyms
RETURNING id;

-- name: ListSkills :many
SELECT * FROM skills ORDER BY group_code, name;

-- name: GetSkillByCode :one
SELECT * FROM skills WHERE code = $1;

-- name: GetSkillsByIDs :many
SELECT * FROM skills WHERE id = ANY(sqlc.arg(ids)::int[]);

-- name: GetSkillsByCodes :many
SELECT * FROM skills WHERE code = ANY(sqlc.arg(codes)::text[]);

-- name: CountSkills :one
SELECT count(*) FROM skills;
