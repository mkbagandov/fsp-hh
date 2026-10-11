-- +goose Up
-- Вакансии, отклики и уведомления (docs/02-data-model.md). Классический путь поверх обратной механики:
-- вакансия с обязательной вилкой, отклик кандидата сразу раскрывает его контакты компании (правило ТЗ 2.2).
CREATE TYPE vacancy_status AS ENUM ('draft', 'published', 'closed');
CREATE TYPE application_status AS ENUM ('sent', 'viewed', 'invited', 'rejected', 'withdrawn');
CREATE TYPE notification_kind AS ENUM ('invitation_received', 'invitation_status', 'application_received', 'application_status', 'test_result', 'micro_task_offered', 'system');

CREATE TABLE vacancies (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id      uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  created_by      uuid NOT NULL REFERENCES users(id),
  need_id         uuid REFERENCES needs(id) ON DELETE SET NULL,
  title           text NOT NULL,
  description     text NOT NULL,
  specialization  specialization_code NOT NULL,
  grade_min       grade_code NOT NULL,
  grade_max       grade_code NOT NULL,
  must_skill_ids  integer[] NOT NULL DEFAULT '{}',
  nice_skill_ids  integer[] NOT NULL DEFAULT '{}',
  salary_from     integer NOT NULL,
  salary_to       integer NOT NULL,
  work_formats    work_format[] NOT NULL DEFAULT '{}',
  city            text,
  remote_ok       boolean NOT NULL DEFAULT true,
  status          vacancy_status NOT NULL DEFAULT 'draft',
  published_at    timestamptz,
  closed_at       timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  CHECK (salary_from > 0 AND salary_from <= salary_to)
);
CREATE TRIGGER vacancies_updated_at BEFORE UPDATE ON vacancies FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX vacancies_public_idx ON vacancies (status, specialization, published_at DESC);
CREATE INDEX vacancies_company_idx ON vacancies (company_id, created_at DESC);

ALTER TABLE invitations ADD CONSTRAINT invitations_vacancy_fkey FOREIGN KEY (vacancy_id) REFERENCES vacancies(id) ON DELETE SET NULL;

CREATE TABLE applications (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  vacancy_id        uuid NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
  candidate_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  cover_letter      text,
  status            application_status NOT NULL DEFAULT 'sent',
  sent_at           timestamptz NOT NULL DEFAULT now(),
  viewed_at         timestamptz,
  responded_at      timestamptz,
  reject_reason     text,
  withdrawn_at      timestamptz,
  invitation_id     uuid REFERENCES invitations(id) ON DELETE SET NULL,
  UNIQUE (vacancy_id, candidate_user_id)
);
CREATE INDEX applications_candidate_idx ON applications (candidate_user_id, sent_at DESC);
CREATE INDEX applications_vacancy_idx ON applications (vacancy_id, status, sent_at DESC);

CREATE TABLE notifications (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       notification_kind NOT NULL,
  title      text NOT NULL,
  body       text,
  link       text,
  payload    jsonb NOT NULL DEFAULT '{}',
  read_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, read_at, created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE applications;
ALTER TABLE invitations DROP CONSTRAINT invitations_vacancy_fkey;
DROP TABLE vacancies;
DROP TYPE notification_kind;
DROP TYPE application_status;
DROP TYPE vacancy_status;
