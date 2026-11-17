// server helpers: env access + embedded migrations. Since 1.4.0.

package server

import (
	"os"

	"github.com/example/generated-api/internal/infra/database"
)

// _osGetenv wraps os.Getenv (swappable for tests). Since 1.4.0.
func _osGetenv(k string) string { return os.Getenv(k) }

// migrations returns the init migration baked into the binary. The same SQL
// lives in resources/migrations/0001_init.sql for `make migrate`; keeping it
// embedded means the server self-applies it on boot if the table is missing.
// Since 1.4.0.
func migrations() []database.Migration {
	return []database.Migration{
		{Version: "0001_init", SQL: migrationSQL},
	}
}

// migrationSQL is the initial schema, idempotent. Since 1.4.0.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS users (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT         NOT NULL UNIQUE,
    name        TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
`
