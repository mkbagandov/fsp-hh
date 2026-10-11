-- +goose Up
-- Этап 6: опрос, банк заданий (метаданные и калибровка), тест-сессии, категории, история грейдов.

CREATE TYPE test_session_status AS ENUM ('created', 'in_progress', 'completed', 'abandoned', 'expired');

CREATE TABLE surveys (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  specialization   specialization_code NOT NULL,
  industry         text,
  primary_stack    text[] NOT NULL DEFAULT '{}',
  claimed_grade    grade_code NOT NULL,
  experience_years numeric(4,1),
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX surveys_user_idx ON surveys (user_id, created_at DESC);

-- Метаданные семейств: код семейства живёт в pkg/itembank, здесь — калибровка и статистика.
CREATE TABLE item_families (
  id         text PRIMARY KEY,
  spec       specialization_code NOT NULL,
  topic      text NOT NULL,
  grade      grade_code NOT NULL,
  format     text NOT NULL,
  a          numeric NOT NULL,
  b          numeric NOT NULL,
  c          numeric NOT NULL,
  a_prior    numeric NOT NULL,
  b_prior    numeric NOT NULL,
  exposures  int NOT NULL DEFAULT 0,
  corrects   int NOT NULL DEFAULT 0,
  enabled    boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX item_families_spec_idx ON item_families (spec, enabled);

CREATE TABLE test_sessions (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  survey_id      uuid REFERENCES surveys(id) ON DELETE SET NULL,
  specialization specialization_code NOT NULL,
  target_grade   grade_code NOT NULL,
  status         test_session_status NOT NULL DEFAULT 'created',
  seed           bigint NOT NULL,
  started_at     timestamptz,
  finished_at    timestamptz,
  expires_at     timestamptz NOT NULL,
  theta          numeric,
  se             numeric,
  items_count    int NOT NULL DEFAULT 0,
  correct_count  int NOT NULL DEFAULT 0,
  decision       grade_status,
  percentile     numeric,
  flags          jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX test_sessions_active_idx ON test_sessions (user_id) WHERE status IN ('created', 'in_progress');
CREATE INDEX test_sessions_user_idx ON test_sessions (user_id, created_at DESC);
CREATE INDEX test_sessions_spec_idx ON test_sessions (specialization, created_at DESC);

-- Выданные задания: ответ хранится только здесь и не отдаётся в API до завершения сессии.
CREATE TABLE test_items (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id    uuid NOT NULL REFERENCES test_sessions(id) ON DELETE CASCADE,
  position      int NOT NULL,
  family_id     text NOT NULL REFERENCES item_families(id),
  seed          bigint NOT NULL,
  instance      jsonb NOT NULL,
  served_at     timestamptz NOT NULL DEFAULT now(),
  deadline_at   timestamptz NOT NULL,
  answered_at   timestamptz,
  submission    jsonb,
  is_correct    boolean,
  time_spent_ms int,
  theta_before  numeric,
  theta_after   numeric,
  se_after      numeric,
  UNIQUE (session_id, position),
  UNIQUE (session_id, family_id)
);
CREATE INDEX test_items_family_idx ON test_items (family_id, served_at DESC);

CREATE TABLE candidate_categories (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  specialization       specialization_code NOT NULL,
  grade                grade_code NOT NULL,
  status               grade_status NOT NULL,
  is_primary           boolean NOT NULL DEFAULT false,
  theta                numeric,
  se                   numeric,
  percentile           numeric,
  last_session_id      uuid,
  confirmed_at         timestamptz,
  next_test_allowed_at timestamptz,
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, specialization)
);
CREATE INDEX candidate_categories_cat_idx ON candidate_categories (specialization, grade, status);
CREATE TRIGGER candidate_categories_updated_at BEFORE UPDATE ON candidate_categories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE grade_history (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  specialization specialization_code NOT NULL,
  claimed_grade  grade_code NOT NULL,
  decision       grade_status NOT NULL,
  result_grade   grade_code,
  theta          numeric,
  se             numeric,
  session_id     uuid,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX grade_history_user_idx ON grade_history (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS grade_history;
DROP TABLE IF EXISTS candidate_categories;
DROP TABLE IF EXISTS test_items;
DROP TABLE IF EXISTS test_sessions;
DROP TABLE IF EXISTS item_families;
DROP TABLE IF EXISTS surveys;
DROP TYPE IF EXISTS test_session_status;
