-- +goose Up
-- Базовые расширения. citext — регистронезависимые e-mail; pgcrypto — gen_random_uuid()
-- (в PostgreSQL 13+ оба расширения "trusted", владельцу БД права суперпользователя не нужны).
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- +goose Down
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS citext;
