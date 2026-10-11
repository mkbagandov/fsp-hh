-- +goose Up
-- Этап 8: компании, потребности, подборки с версиями, гранты на контакты, приглашения.

CREATE TYPE need_status AS ENUM ('draft', 'active', 'archived');
CREATE TYPE invitation_status AS ENUM ('sent', 'viewed', 'accepted', 'declined', 'withdrawn', 'expired');
CREATE TYPE decline_reason AS ENUM ('salary_below', 'not_looking', 'role_mismatch', 'format_mismatch', 'company', 'other');
CREATE TYPE contact_grant_source AS ENUM ('invitation_accepted', 'application_sent');

CREATE TABLE companies (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name            text NOT NULL,
  inn             text,
  description     text,
  industry        text,
  website         text,
  city            text,
  logo_path       text,
  public_contacts jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER companies_updated_at BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE employer_users (
  user_id      uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  company_id   uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  company_role text NOT NULL DEFAULT 'owner',
  position     text,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX employer_users_company_idx ON employer_users (company_id);

CREATE TABLE needs (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id       uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  created_by       uuid NOT NULL REFERENCES users(id),
  title            text NOT NULL,
  description      text,
  team_description text,
  specialization   specialization_code NOT NULL,
  grade_min        grade_code NOT NULL,
  grade_max        grade_code NOT NULL,
  must_skill_ids   int[] NOT NULL DEFAULT '{}',
  nice_skill_ids   int[] NOT NULL DEFAULT '{}',
  salary_from      integer NOT NULL,
  salary_to        integer NOT NULL,
  work_formats     work_format[] NOT NULL DEFAULT '{}',
  city             text,
  remote_ok        boolean NOT NULL DEFAULT true,
  status           need_status NOT NULL DEFAULT 'draft',
  extraction       jsonb,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (salary_from > 0 AND salary_from <= salary_to)
);
CREATE INDEX needs_company_idx ON needs (company_id, created_at DESC);
CREATE TRIGGER needs_updated_at BEFORE UPDATE ON needs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE selections (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  need_id    uuid NOT NULL UNIQUE REFERENCES needs(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE selection_versions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  selection_id uuid NOT NULL REFERENCES selections(id) ON DELETE CASCADE,
  version_no   int NOT NULL,
  filters      jsonb NOT NULL DEFAULT '{}'::jsonb,
  weights      jsonb NOT NULL,
  category_map jsonb NOT NULL,
  items_count  int NOT NULL DEFAULT 0,
  created_by   uuid NOT NULL REFERENCES users(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (selection_id, version_no)
);

CREATE TABLE selection_items (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  version_id        uuid NOT NULL REFERENCES selection_versions(id) ON DELETE CASCADE,
  candidate_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  rank              int NOT NULL,
  score             numeric NOT NULL,
  relevance         numeric NOT NULL,
  p_accept          numeric NOT NULL,
  components        jsonb NOT NULL,
  explanation       jsonb NOT NULL,
  grade_status      grade_status NOT NULL,
  UNIQUE (version_id, candidate_user_id)
);
CREATE INDEX selection_items_rank_idx ON selection_items (version_id, rank);

-- Единственный ключ к контактам кандидата для компании (docs/adr/0008).
CREATE TABLE contact_grants (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  company_id        uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  source            contact_grant_source NOT NULL,
  source_id         uuid NOT NULL,
  granted_at        timestamptz NOT NULL DEFAULT now(),
  revoked_at        timestamptz
);
CREATE UNIQUE INDEX contact_grants_active_idx ON contact_grants (candidate_user_id, company_id) WHERE revoked_at IS NULL;

CREATE TABLE invitations (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id        uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  need_id           uuid REFERENCES needs(id) ON DELETE SET NULL,
  vacancy_id        uuid,
  candidate_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by        uuid NOT NULL REFERENCES users(id),
  message           text NOT NULL,
  salary_from       integer NOT NULL,
  salary_to         integer NOT NULL,
  contact_method    jsonb NOT NULL,
  status            invitation_status NOT NULL DEFAULT 'sent',
  sent_at           timestamptz NOT NULL DEFAULT now(),
  viewed_at         timestamptz,
  responded_at      timestamptz,
  decline_reason    decline_reason,
  decline_comment   text,
  withdrawn_at      timestamptz,
  expires_at        timestamptz NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  CHECK (salary_from > 0 AND salary_from <= salary_to)
);
CREATE UNIQUE INDEX invitations_active_idx ON invitations (company_id, candidate_user_id) WHERE status IN ('sent', 'viewed');
CREATE INDEX invitations_candidate_idx ON invitations (candidate_user_id, created_at DESC);
CREATE INDEX invitations_company_idx ON invitations (company_id, created_at DESC);
CREATE INDEX invitations_need_idx ON invitations (need_id);

-- +goose Down
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS contact_grants;
DROP TABLE IF EXISTS selection_items;
DROP TABLE IF EXISTS selection_versions;
DROP TABLE IF EXISTS selections;
DROP TABLE IF EXISTS needs;
DROP TABLE IF EXISTS employer_users;
DROP TABLE IF EXISTS companies;
DROP TYPE IF EXISTS contact_grant_source;
DROP TYPE IF EXISTS decline_reason;
DROP TYPE IF EXISTS invitation_status;
DROP TYPE IF EXISTS need_status;
