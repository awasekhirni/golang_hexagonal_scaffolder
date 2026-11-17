-- 0001_init.sql — initial schema for the generated-api reference module.
-- Applied by DatabaseFactory.Migrate or `make migrate`. Idempotent.

CREATE TABLE IF NOT EXISTS users (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT         NOT NULL UNIQUE,
    name        TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
