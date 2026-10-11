-- +goose Up
-- Привязка ФСП ID и кэш достижений из реестра (docs/05-fsp-integration.md).

CREATE TYPE fsp_discipline AS ENUM ('algorithmic', 'product', 'security', 'robotics', 'uav');
CREATE TYPE fsp_level AS ENUM ('regional', 'national', 'international');
CREATE TYPE fsp_result_kind AS ENUM ('place', 'rank', 'rating', 'participation');

CREATE TABLE fsp_links (
  user_id      uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  fsp_id       text NOT NULL UNIQUE,
  issuer       text NOT NULL,
  subject      text NOT NULL,
  consent_id   uuid REFERENCES consents(id) ON DELETE SET NULL,
  linked_at    timestamptz NOT NULL DEFAULT now(),
  last_sync_at timestamptz,
  sync_error   text,
  UNIQUE (issuer, subject)
);

CREATE TABLE fsp_achievements (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  external_id text NOT NULL,
  discipline  fsp_discipline NOT NULL,
  level       fsp_level NOT NULL,
  event_name  text NOT NULL,
  event_date  date NOT NULL,
  result_kind fsp_result_kind NOT NULL,
  place       int CHECK (place IS NULL OR place > 0),
  rank_title  text,
  rating      int,
  role        text,
  team_name   text,
  verified    boolean NOT NULL DEFAULT true,
  fetched_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, external_id)
);
CREATE INDEX fsp_achievements_user_idx ON fsp_achievements (user_id, event_date DESC);

-- +goose Down
DROP TABLE IF EXISTS fsp_achievements;
DROP TABLE IF EXISTS fsp_links;
DROP TYPE IF EXISTS fsp_result_kind;
DROP TYPE IF EXISTS fsp_level;
DROP TYPE IF EXISTS fsp_discipline;
