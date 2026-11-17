# generated-api

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.4.0-orange)](../VERSION)

The **reference hexagonal Go API** that the [`generator`](../generator/README.md)
emits. It is runnable on its own and doubles as the golden template the
generator's output is diffed against.

Every public function is wrapped by `AppLogger.Logged` (entry/exit/error
tracing), every error is typed (`API-*` codes from
[`docs/error-codes.md`](../docs/error-codes.md)), every catch-and-rethrow
logs the cause with stack trace before wrapping, and every SQL query uses
parameter-bound placeholders.

## Layout

```
generated-api/
├── README.md
├── go.mod
├── Dockerfile
├── application.yaml          ← default config (env vars override)
├── .env.example
├── resources/
│   ├── migrations/0001_init.sql
│   └── openapi.yaml
├── cmd/api/main.go
└── internal/
    ├── shared/
    │   ├── applogger/        ← AppLogger + Logged wrapper (slog JSON)
    │   └── errors/           ← typed AppError with API-* codes
    ├── config/settings/      ← env → typed Settings (GEN-001/API-001)
    ├── infra/
    │   ├── database/         ← DatabaseFactory Init/Migrate/Tx (P0-3.1)
    │   └── bus/              ← EnterpriseBus, DomainEventBus, ServiceRegistration
    ├── app/
    │   ├── middleware/       ← RequestId, Auth, Error, Logging, Security,
    │   │                       RateLimit, CORS, Recover
    │   ├── routes/           ← Health (live/ready/combined), Metrics, User
    │   └── server/           ← chi router wiring, graceful shutdown
    ├── domain/user/          ← UserEntity + UserDomainError (P0-3.2)
    │   └── repository/       ← UserRepository + Postgres adapter (P0-3.1)
    └── application/user/
        ├── service.go        ← UserService use cases
        └── controller/       ← UserController HTTP handlers (P0-3.6)
```

## Quick start

```bash
# From the repo root:
cp .env.example .env          # set POSTGRES_PASSWORD + JWT_SECRET
make up                       # docker compose up --build
make migrate                  # apply SQL migrations
make smoke                    # curl /health/ready and /users
```

Or locally (needs postgres running on 127.0.0.1:5432):

```bash
cd generated-api
export $(grep -v '^#' ../.env | xargs)
psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -f resources/migrations/0001_init.sql
go run ./cmd/api
```

## Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/health` | none | Combined health (runtime + DB). |
| GET | `/health/live` | none | Liveness — Go runtime only. |
| GET | `/health/ready` | none | Readiness — probes DB with `SELECT 1`. |
| GET | `/metrics` | none | Prometheus metrics. |
| POST | `/auth/token` | none | Issue a JWT (dev: any email/password). |
| GET | `/users` | JWT | List users. |
| POST | `/users` | JWT | Create a user. |
| GET | `/users/{id}` | JWT | Get a user by id. |
| PUT | `/users/{id}` | JWT | Update a user. |
| DELETE | `/users/{id}` | JWT | Delete a user. |

## Configuration

Env vars documented in the root [`.env.example`](../.env.example). The ones
most relevant to this module:

| Variable | Default | Notes |
| --- | --- | --- |
| `DB_POOL_SIZE` | 10 | Max cached connections. |
| `DB_MAX_OVERFLOW` | 20 | Extra connections under burst. |
| `DB_CONN_TIMEOUT` | 5 | Seconds to wait for an idle conn. |
| `LOG_LEVEL` | INFO | Root log level. |
| `HEXAPI_LOG_LEVEL` | DEBUG | hexapi subtree override. |
| `LOG_FORMAT` | text (dev) / json (docker) | Structured output. |
| `CORS_ALLOWED_ORIGINS` | http://localhost:8080 | Allowlist, never `*`. |
| `RATE_LIMIT_RPM` | 100 | Per-IP requests/minute. |
| `JWT_SECRET` | — | HMAC-SHA256 key (required in prod). |

## Testing

```bash
cd generated-api
go test -race ./...            # all packages
```

Test inventory: `applogger_test`, `errors_test`, `settings_test`,
`databasefactory_test`, `bus_test`, `middleware_test` (RequestId, Auth,
RateLimit, Security, Error), `health_test`, `user_test` (entity invariants),
`repository_test`, `service_test`, `controller_test`, `applogcheck`
(AST enforcement of the Logged contract).

## Error model

Every error returned through the HTTP layer carries a stable `API-*` code.
See [`docs/error-codes.md`](../docs/error-codes.md) for the full inventory.
The wire format is:

```json
{"code":"API-002","message":"user not found: id=...","reqId":"..."}
```
