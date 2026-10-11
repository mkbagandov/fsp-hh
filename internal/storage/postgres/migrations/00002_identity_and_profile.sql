-- +goose Up
-- Схема этапов 2–4: пользователи и доступ, согласия, аудит, справочник навыков, профиль кандидата.
-- Описание полей — docs/02-data-model.md.

CREATE TYPE user_role AS ENUM ('candidate', 'employer', 'admin');
CREATE TYPE user_status AS ENUM ('active', 'blocked', 'deleted');
CREATE TYPE specialization_code AS ENUM ('backend', 'frontend', 'data_ml', 'devops', 'qa', 'security', 'common');
CREATE TYPE grade_code AS ENUM ('junior', 'middle', 'senior');
CREATE TYPE grade_status AS ENUM ('claimed', 'confirmed', 'not_confirmed', 'upgrade_suggested');
CREATE TYPE candidate_visibility AS ENUM ('open', 'passive', 'hidden');
CREATE TYPE work_format AS ENUM ('office', 'hybrid', 'remote');
CREATE TYPE skill_source AS ENUM ('declared', 'resume', 'test_confirmed');
CREATE TYPE consent_kind AS ENUM ('processing', 'publication', 'fsp_data');
CREATE TYPE verification_purpose AS ENUM ('verify_email', 'reset_password');

-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE users (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email             citext NOT NULL UNIQUE,
  email_verified_at timestamptz,
  password_hash     text,
  role              user_role NOT NULL,
  status            user_status NOT NULL DEFAULT 'active',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  last_login_at     timestamptz
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE email_verifications (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose     verification_purpose NOT NULL,
  code_hash   text NOT NULL,
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz,
  attempts    int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_verifications_user_idx ON email_verifications (user_id, purpose, created_at DESC);

CREATE TABLE refresh_tokens (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  text NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  replaced_by uuid,
  user_agent  text,
  ip          inet,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id, created_at DESC);

CREATE TABLE consents (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind         consent_kind NOT NULL,
  version      text NOT NULL,
  accepted_at  timestamptz NOT NULL DEFAULT now(),
  ip           inet,
  user_agent   text,
  withdrawn_at timestamptz
);
CREATE INDEX consents_user_kind_idx ON consents (user_id, kind, accepted_at DESC);

CREATE TABLE audit_log (
  id            bigserial PRIMARY KEY,
  actor_user_id uuid,
  action        text NOT NULL,
  object_type   text NOT NULL,
  object_id     uuid,
  details       jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip            inet,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_object_idx ON audit_log (object_type, object_id);
CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_user_id, created_at DESC);

CREATE TABLE skills (
  id         serial PRIMARY KEY,
  code       text NOT NULL UNIQUE,
  name       text NOT NULL UNIQUE,
  group_code text NOT NULL,
  specs      specialization_code[] NOT NULL DEFAULT '{}',
  topic      text,
  synonyms   text[] NOT NULL DEFAULT '{}'
);
CREATE INDEX skills_synonyms_idx ON skills USING gin (synonyms);

CREATE TABLE candidate_profiles (
  user_id                uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  first_name             text NOT NULL DEFAULT '',
  last_name              text NOT NULL DEFAULT '',
  middle_name            text,
  city                   text,
  work_formats           work_format[] NOT NULL DEFAULT '{}',
  relocation             boolean NOT NULL DEFAULT false,
  salary_min             integer CHECK (salary_min IS NULL OR salary_min > 0),
  salary_expected        integer CHECK (salary_expected IS NULL OR salary_expected > 0),
  visibility             candidate_visibility NOT NULL DEFAULT 'open',
  headline               text,
  about                  text,
  experience_years       numeric(4,1) CHECK (experience_years IS NULL OR experience_years >= 0),
  primary_specialization specialization_code,
  links                  jsonb NOT NULL DEFAULT '{}'::jsonb,
  resume_pdf_path        text,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER candidate_profiles_updated_at BEFORE UPDATE ON candidate_profiles FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX candidate_profiles_visibility_idx ON candidate_profiles (visibility);

-- Контакты хранятся отдельно и выдаются только по гранту (docs/adr/0008).
CREATE TABLE candidate_contacts (
  user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  phone      text,
  telegram   text,
  email      text,
  other      jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER candidate_contacts_updated_at BEFORE UPDATE ON candidate_contacts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE privacy_settings (
  user_id              uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  show_full_name       boolean NOT NULL DEFAULT true,
  show_experience      boolean NOT NULL DEFAULT true,
  show_fsp             boolean NOT NULL DEFAULT true,
  show_salary_expected boolean NOT NULL DEFAULT true,
  show_city            boolean NOT NULL DEFAULT true,
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER privacy_settings_updated_at BEFORE UPDATE ON privacy_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE candidate_experiences (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  company_name   text NOT NULL,
  position       text NOT NULL,
  specialization specialization_code,
  started_on     date NOT NULL,
  ended_on       date CHECK (ended_on IS NULL OR ended_on >= started_on),
  description    text,
  stack          text[] NOT NULL DEFAULT '{}',
  sort_order     int NOT NULL DEFAULT 0
);
CREATE INDEX candidate_experiences_user_idx ON candidate_experiences (user_id, sort_order, started_on DESC);

CREATE TABLE candidate_education (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  institution    text NOT NULL,
  degree         text,
  field          text,
  graduated_year int CHECK (graduated_year IS NULL OR graduated_year BETWEEN 1950 AND 2100)
);
CREATE INDEX candidate_education_user_idx ON candidate_education (user_id);

CREATE TABLE candidate_skills (
  user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  skill_id             int NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
  level                smallint CHECK (level IS NULL OR level BETWEEN 1 AND 5),
  source               skill_source NOT NULL,
  confirmed_session_id uuid,
  created_at           timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, skill_id)
);
CREATE INDEX candidate_skills_skill_idx ON candidate_skills (skill_id);

CREATE TABLE resume_uploads (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  file_name    text NOT NULL,
  mime         text NOT NULL,
  size_bytes   int NOT NULL,
  storage_path text NOT NULL,
  parsed_at    timestamptz,
  parse_result jsonb,
  parser       text,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX resume_uploads_user_idx ON resume_uploads (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS resume_uploads;
DROP TABLE IF EXISTS candidate_skills;
DROP TABLE IF EXISTS candidate_education;
DROP TABLE IF EXISTS candidate_experiences;
DROP TABLE IF EXISTS privacy_settings;
DROP TABLE IF EXISTS candidate_contacts;
DROP TABLE IF EXISTS candidate_profiles;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS consents;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS set_updated_at();
DROP TYPE IF EXISTS verification_purpose;
DROP TYPE IF EXISTS consent_kind;
DROP TYPE IF EXISTS skill_source;
DROP TYPE IF EXISTS work_format;
DROP TYPE IF EXISTS candidate_visibility;
DROP TYPE IF EXISTS grade_status;
DROP TYPE IF EXISTS grade_code;
DROP TYPE IF EXISTS specialization_code;
DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;
