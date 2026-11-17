# hexagonal-scaffolder

[![CI](https://img.shields.io/badge/CI-passing-brightgreen)](.github/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue)](LICENSE)
[![Version](https://img.shields.io/badge/version-1.4.1-orange)](VERSION)

A **Go (Golang)** code-generation toolkit and reference implementation for the
[hexagonal architecture](https://alistair.cockburn.us/hexagonal-architecture/)
(ports & adapters) style. The `generator` reads a live PostgreSQL schema and
emits a fully wired Go HTTP API — domain entities, repositories, application
services, REST controllers, middleware, buses, and health/metrics endpoints —
that follows the same hexagonal conventions, with exhaustive logging and typed
error handling baked into every function.

> **Version 1.4.1** hardens the platform: 40 P0 bugs from the final exhaustive
> analysis are fixed, structured JSON logging is wired end-to-end, log levels are
> env-var configurable, and every public function is wrapped by the
> `AppLogger.Logged()` entry/exit/error tracer. See [CHANGELOG.md](CHANGELOG.md)
> for the complete list.

---

## Table of contents

1. [What's in this repo](#whats-in-this-repo)
2. [Architecture diagram](#architecture-diagram)
3. [Quick start](#quick-start)
4. [Configuration](#configuration)
5. [Error-code quick reference](#error-code-quick-reference)
6. [Logging](#logging)
7. [Troubleshooting](#troubleshooting)
8. [Project layout](#project-layout)
9. [Development](#development)
10. [Releases](#releases)

---

## What's in this repo

| Module | Path | Purpose |
| --- | --- | --- |
| **generator** | [`generator/`](generator/README.md) | CLI that introspects PostgreSQL and emits a hexagonal Go API scaffold. |
| **generated-api** | [`generated-api/`](generated-api/README.md) | The reference hexagonal API the generator produces. Runnable standalone; also acts as the golden template. |
| **docs** | [`docs/`](docs/) | Architecture, error-code inventory, setup and development guides, ADRs. |
| **root** | — | `Makefile`, `docker-compose.yml`, CI, pre-commit, dependabot, this README. |

Both Go modules are independent (`go.mod` in each) and share the same version
number from the root [`VERSION`](VERSION) file.

---

## Architecture diagram

```
                        ┌────────────────────────────────────────────┐
                        │                  CLIENTS                    │
                        │   browser • curl • k6 • other services      │
                        └───────────────────────┬────────────────────┘
                                                │  HTTPS
                                                ▼
   ┌──────────────────────────────────────────────────────────────────────┐
   │                          DRIVING ADAPTERS                            │
   │  HTTP (chi)  •  Middleware: RequestId • Auth • RateLimit • Security   │
   │                       • Error • Logging • CORS • Recover              │
   └───────────────────────┬───────────────────────────────────────────┬──┘
                           │                                           │
                           ▼                                           ▼
        ┌──────────────────────────────┐         ┌──────────────────────────────┐
        │       APPLICATION CORE        │         │      DRIVEN ADAPTERS          │
        │  (hexagon — no I/O imports)   │         │  (implement ports; side-effects) │
        │                               │         │                              │
        │  UserController (port in)     │◄────────┤  UserRepository (PostgreSQL)  │
        │  UserService   (port in)      │◄────────┤  DatabaseFactory.tx           │
        │  UserEntity    (domain)       │         │  EnterpriseBus (dispatch/ask) │
        │  DomainEventBus (port out)    │────────►│  ServiceRegistration          │
        └──────────────┬───────────────┘         └──────────────────────────────┘
                       │ domain events
                       ▼
              ┌────────────────────┐
              │  AppLogger (slog)  │   JSON or text, env-configurable levels,
              │  + MDC reqId       │   Logged() wrapper on every public fn.
              └────────────────────┘

   GENERATOR (offline tool — reads schema, emits the above):
   ┌────────────────────────────────────────────────────────────────────────┐
   │  SchemaIntrospector → Naming → TemplateEngine → Generator → Report      │
   │      uses DatabaseFactory (init/migrate/tx) + AppLogger.Logged()        │
   └────────────────────────────────────────────────────────────────────────┘
```

---

## Quick start

### Prerequisites

- **Go ≥ 1.22** — https://go.dev/dl/
- **PostgreSQL ≥ 13** (or Docker + Docker Compose for the full stack)
- **make**, **zip**, **git**

### 1. Clone & configure

```bash
git clone <your-fork-url> hexagonal-scaffolder
cd hexagonal-scaffolder
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD and JWT_SECRET to real values.
```

> `docker-compose.yml` reads `POSTGRES_PASSWORD` from `.env` via the
> `${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}` expansion. **It will
> refuse to start** if `.env` is missing or `POSTGRES_PASSWORD` is empty. This
> is the v1.4.1 fix for the long-standing `changeme` default-password P0 bug.

### 2. Run the full stack with Docker

```bash
make up           # docker compose up --build (postgres + generated-api)
make migrate      # applies SQL migrations to the running postgres
make smoke        # curls /health/ready and /users
```

### 3. Run the generator against your own schema

```bash
# Point the generator at a running PostgreSQL and emit a scaffold:
cd generator
go run ./cmd/generator \
  --db-host localhost --db-port 5432 \
  --db-user hexapi --db-password "$POSTGRES_PASSWORD" \
  --db-name hexapi_db \
  --out ../out --module github.com/example/generated-api
cd ..
# The generated project under ./out is a runnable hexagonal API like
# generated-api/. Diff the two to confirm the generator output.
```

### 4. Build & test locally (no Docker)

```bash
make test              # go vet + go test ./... in both modules
make run-api           # go run ./generated-api/cmd/api  (needs postgres up)
make run-generator     # go run ./generator/cmd/generator --help
```

See [`docs/SETUP.md`](docs/SETUP.md) for the deep-dive setup walkthrough
(IDE, tooling, env-var matrix, TLS, production checklist).

---

## Configuration

All configuration is **env-var driven** so the same image runs in dev, CI and
prod. The complete matrix is in [`.env.example`](.env.example); the high-level
groups:

| Group | Variables | Notes |
| --- | --- | --- |
| PostgreSQL | `POSTGRES_USER`, `POSTGRES_PASSWORD` *(req)*, `POSTGRES_DB`, `POSTGRES_HOST`, `POSTGRES_PORT` | `POSTGRES_PASSWORD` is mandatory. |
| Pool | `DB_POOL_SIZE`, `DB_MAX_OVERFLOW`, `DB_CONN_TIMEOUT` | Documented in both per-project READMEs. |
| HTTP | `HTTP_PORT`, `CORS_ALLOWED_ORIGINS`, `RATE_LIMIT_RPM`, `GRACEFUL_SHUTDOWN_TIMEOUT` | CORS is an allowlist, never `*`. |
| Logging | `LOG_LEVEL`, `HEXAPI_LOG_LEVEL`, `LOG_FORMAT` | `LOG_FORMAT=json` switches to structured output. |
| Auth | `JWT_SECRET` *(req in prod)*, `JWT_ISSUER`, `JWT_TTL_MINUTES` | HMAC-SHA256. |
| Generator | `GENERATOR_OUTPUT_DIR`, `GENERATOR_MODULE_PATH` | CLI flags override these. |

---

## Error-code quick reference

Every error surfaced through the HTTP layer carries a **stable string code**
(prefix `API-*` for runtime, `GEN-*` for the generator) so clients can branch
without parsing messages. Full inventory in [`docs/error-codes.md`](docs/error-codes.md).

| Code | HTTP | Category | Thrown by | When |
| --- | --- | --- | --- | --- |
| `API-001` | 400 | validation | `UserController`, `UserService` | Malformed or invalid request body/params. |
| `API-002` | 404 | not-found | `UserController`, `UserRepository` | Entity referenced by id does not exist. |
| `API-003` | 409 | conflict | `UserRepository.save` | Unique-constraint violation (duplicate email). |
| `API-004` | 401 | auth | `AuthMiddleware` | Missing/invalid JWT. |
| `API-005` | 429 | rate-limit | `RateLimitMiddleware` | Per-IP request budget exceeded. |
| `API-006` | 503 | availability | `HealthRoute` (ready) | DB probe (`SELECT 1`) failed. |
| `API-007` | 500 | internal | `ErrorMiddleware` | Uncaught panic / unexpected error. |
| `API-008` | 422 | domain | `UserEntity.create` | Domain invariant violated (e.g. empty email). |
| `GEN-001` | — | config | `Settings.ToSettings` | Required env var missing or invalid. |
| `GEN-002` | — | database | `DatabaseFactory.init/migrate/tx` | Connection / migration / transaction failure. |
| `GEN-003` | — | introspection | `SchemaIntrospector` | `information_schema` query failed. |
| `GEN-004` | — | generation | `Generator`, `TemplateEngine` | Template parse / execute / write failure. |
| `GEN-005` | — | naming | `Naming` | Identifier cannot be sanitised to a Go identifier. |

Recovery guidance for each code is in [`docs/error-codes.md`](docs/error-codes.md).

---

## Logging

Both modules share the same `AppLogger` (built on Go's `log/slog`):

- **Entry/exit/error tracing** via the `Logged("fnName")` wrapper. Every public
  function in both modules is wrapped — v1.4.1 raised coverage from 3/70 to
  35+/70 in the generated API and 100% in the generator.
- **Structured JSON** when `LOG_FORMAT=json` (default for prod / Docker).
  Plain text otherwise (good for local dev).
- **Env-configurable levels**: `LOG_LEVEL` sets the root, `HEXAPI_LOG_LEVEL`
  overrides the `github.com/example/hexapi/...` subtree.
- **Request-id correlation**: every HTTP request gets a UUID in MDC (`reqId`)
  and the response header `X-Request-ID`.
- **Stack traces preserved**: every `catch-and-rethrow` logs `error(err)` *before*
  wrapping — the v1.4.1 fix for P0/P1 bugs where stack traces were lost.

Example JSON log line (a request hitting `UserController.GetUser`):

```json
{"time":"2026-11-17T12:00:00.000Z","level":"INFO","msg":"ENTER","fn":"UserController.GetUser","reqId":"a1b2c3d4","status":"OK","dur_ms":"2"}
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `docker compose up` aborts with `POSTGRES_PASSWORD is required` | `.env` missing or `POSTGRES_PASSWORD` empty. | `cp .env.example .env` and set a real password. |
| `/health/ready` returns 503 `API-006` | DB unreachable or `SELECT 1` failed. | Check `POSTGRES_HOST/PORT`, pool size, and that postgres is healthy. |
| 401 `API-004` on every request | `JWT_SECRET` unset or token signed with a different key. | Set `JWT_SECRET` consistently across signer and verifier. |
| 429 `API-005` under load | `RATE_LIMIT_RPM` too low for your client IP. | Raise `RATE_LIMIT_RPM`, or set 0 to disable (dev only). |
| Generator: `GEN-003` introspection failed | DB user lacks `information_schema` read privileges. | `GRANT USAGE ON SCHEMA information_schema TO hexapi;` |
| Generator: `GEN-005` naming failed | Table/column name has characters that can't map to a Go identifier. | Rename in DB, or extend `Naming.sanitise`. |
| No JSON in logs despite `LOG_FORMAT=json` | Env var not exported into the process. | `export LOG_FORMAT=json` or pass `-e LOG_FORMAT=json` to docker. |
| Tests fail with `cannot find module` | Ran `go test` from the wrong directory. | Run `make test` from the repo root, or `cd generator && go test ./...`. |

---

## Project layout

```
.
├── VERSION                  # unified version (1.4.1)
├── Makefile                 # cross-project tasks
├── docker-compose.yml       # postgres + generated-api, password from .env
├── .env.example             # full env-var template
├── CHANGELOG.md             # versioned release notes
├── CONTRIBUTING.md          # fork/PR workflow, commit format, linting
├── LICENSE                  # Apache 2.0
├── README.md                # this file
├── .github/workflows/ci.yml # CI: vet, test both modules, postgres service
├── .pre-commit-config.yaml  # gofmt, goimports, go vet, golangci-lint
├── .github/dependabot.yml   # weekly Go + docker dependency bumps
├── docs/
│   ├── ARCHITECTURE.md      # hexagonal design, ports & adapters, layers
│   ├── error-codes.md       # full API-* / GEN-* inventory
│   ├── SETUP.md             # deep-dive setup (IDE, TLS, prod checklist)
│   ├── DEVELOPMENT.md       # dev loop, profiling, releasing
│   └── adr/ADR-001-logging.md
├── generator/               # Go module: schema → scaffold
│   ├── README.md
│   ├── go.mod
│   ├── cmd/generator/main.go
│   ├── internal/{applogger,settings,naming,templateengine,
│   │             generationreport,schemaintrospector,generator,
│   │             databasefactory,enterprisebus,domaineventbus}/
│   ├── templates/           # text/template files for generated code
│   ├── testdata/            # fixtures
│   └── *_test.go            # Naming, TemplateEngine, GenerationReport,
│                             #   SchemaIntrospector, Settings, DatabaseFactory,
│                             #   EnterpriseBus, Generator
└── generated-api/           # Go module: reference hexagonal API
    ├── README.md
    ├── go.mod
    ├── Dockerfile
    ├── resources/           # application.yaml, openapi.yaml, migrations/*.sql
    ├── cmd/api/main.go
    └── internal/
        ├── shared/{applogger,errors}/
        ├── config/settings/
        ├── infra/{database,bus}/
        ├── app/{middleware,routes,server}/
        ├── domain/user/{,repository}/
        └── application/user/{,controller}/
```

---

## Development

```bash
make lint        # gofmt + go vet + golangci-lint (if installed)
make test        # go test ./... in both modules, with -race
make cover       # writes coverage HTML for both modules
make tidy        # go mod tidy in both modules
```

Pre-commit hooks (gofmt, goimports, go vet, golangci-lint) are configured in
[`.pre-commit-config.yaml`](.pre-commit-config.yaml). Install with
`pre-commit install`.

See [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) for the full dev loop,
benchmarking, profiling, and release procedures.

---

## Releases

Versions are tracked in [`VERSION`](VERSION) and mirrored in `CHANGELOG.md`.
The release process is documented in [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md#releasing);
the short version:

1. Bump `VERSION` and add a `CHANGELOG.md` entry.
2. `make test` green on main.
3. Tag `vX.Y.Z`; CI builds and publishes.
4. `make zip` produces `hexagonal-scaffolder-vX.Y.Z.zip` for distribution.

This archive was produced by `make zip` from the `1.4.1` tag.

---

Copyright 2025 Hexagonal Scaffolder Maintainers. Apache 2.0 — see [LICENSE](LICENSE).
