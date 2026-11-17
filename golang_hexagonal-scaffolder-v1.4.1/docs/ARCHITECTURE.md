# Architecture

> **Scope**: This document describes the runtime and design architecture of
> `hexagonal-scaffolder` v1.4.0 — both the `generator` (the offline tool that
> emits a scaffold) and the `generated-api` (the reference hexagonal API the
> generator produces). It is the canonical reference for where code lives and
> why.

## 1. Hexagonal architecture (ports & adapters)

The codebase follows Alistair Cockburn's hexagonal architecture. The core
insight: business logic depends on **ports** (interfaces) and is exercised
through **adapters** that translate between the outside world and the ports.

```
                    ┌────────────── driving adapters ──────────────┐
                    │   HTTP / CLI / test harness / message bus     │
                    └───────────────────────┬───────────────────────┘
                                            │ calls
                                            ▼
   ┌────────────────────────────────────────────────────────────────────┐
   │                          APPLICATION CORE                           │
   │   (no imports of net/http, database/sql, chi, pq — only ports)      │
   │                                                                     │
   │   UserEntity   domain model + invariants                            │
   │   UserService  application service (use cases)                      │
   │   UserController  HTTP handler (port-in adapter — thin)             │
   │   UserRepository interface  (port-out — implemented by adapter)     │
   │   DomainEventBus interface (port-out — implemented by adapter)      │
   │   EnterpriseBus interface  (port-out — implemented by adapter)      │
   └────────────────────────┬───────────────────────┬───────────────────┘
                            │ implements            │ implements
                            ▼                       ▼
   ┌──────────────────────────────────┐  ┌──────────────────────────────┐
   │       DRIVEN ADAPTERS             │  │     DRIVEN ADAPTERS           │
   │  UserRepositoryPostgres           │  │  EnterpriseBusInProcess       │
   │   (database/sql + lib/pq)         │  │  DomainEventBusInProcess      │
   │  DatabaseFactory (Init/Migrate/Tx)│  │  ServiceRegistration          │
   └──────────────────────────────────┘  └──────────────────────────────┘
```

**Rules enforced by layer:**

- **Core** (`internal/domain`, `internal/application`) imports **only** the
  standard library, `internal/shared/errors`, and `internal/shared/applogger`.
  It must never import `database/sql`, `net/http`, or any third-party driver.
  This is the rule that keeps the core testable in isolation.
- **Driving adapters** (`internal/app/routes`, `internal/app/middleware`)
  translate HTTP into core calls. They never contain business rules; a
  controller method should be ~10 lines: parse, call the service, render.
- **Driven adapters** (`internal/infra/database`, `internal/infra/bus`)
  implement the port-out interfaces against concrete technologies.

## 2. Module map

### generator

| Package | Responsibility |
| --- | --- |
| `cmd/generator` | CLI entry point, flag parsing, wires the pipeline. |
| `internal/applogger` | `AppLogger` with `Logged("fn")` wrapper. JSON/text output, env levels. |
| `internal/settings` | Env → typed `Settings`; validates required vars; typed errors. |
| `internal/naming` | snake_case ↔ PascalCase ↔ camelCase ↔ Go-identifier sanitisation. |
| `internal/templateengine` | Wraps `text/template` with helpers and a parse-execute API. |
| `internal/generationreport` | Builds the report of generated files + metrics. |
| `internal/schemaintrospector` | Reads `information_schema` (tables, columns, enums, views). **P1-3.9: parameter-bound query for enum values.** |
| `internal/databasefactory` | `Init/Migrate/Tx` against the introspection DB. **P0-3.1: AppError pass-through in Tx.** |
| `internal/enterprisebus` | `DispatchResolved/AskResolved` (command/query bus). |
| `internal/domaineventbus` | `Publish` for domain events. |
| `internal/generator` | Orchestrates introspect → name → render → write → report. |
| `templates` | `text/template` files for every generated artefact. |
| `testdata` | Schema fixtures for tests. |

### generated-api

| Package | Responsibility |
| --- | --- |
| `cmd/api` | HTTP server bootstrap, wiring, graceful shutdown. |
| `internal/shared/applogger` | Same `AppLogger` as the generator. |
| `internal/shared/errors` | Typed errors with stable `API-*` codes. |
| `internal/config/settings` | Env → typed `Settings`. **Logs error before rethrow as `ConfigError`.** |
| `internal/infra/database` | `DatabaseFactory.Init/Migrate/Tx`. **AppError pass-through in Tx (P0-3.1).** |
| `internal/infra/bus` | `EnterpriseBus`, `DomainEventBus`, `ServiceRegistration`. |
| `internal/app/middleware` | RequestId, Auth, Error, Logging, Security, RateLimit, CORS, Recover. |
| `internal/app/routes` | Health (`/health`, `/health/live`, `/health/ready`), Metrics (`/metrics`), User (`/users`). |
| `internal/app/server` | `chi` router wiring, middleware ordering, `ListenAndServe`. |
| `internal/domain/user` | `UserEntity` with `Create` enforcing invariants (P0-3.2 typed error). |
| `internal/domain/user/repository` | `UserRepository` interface + Postgres adapter. **Duplicate-key mapping (P0-3.1).** |
| `internal/application/user` | `UserService` use cases. |
| `internal/application/user/controller` | `UserController` HTTP handlers. **Typed errors for 404/400 (P0-3.6).** |

## 3. Request lifecycle

A `GET /users/{id}` request flows as follows:

1. **`net/http`** accepts the connection; `chi` matches the route.
2. **`RecoverMiddleware`** wraps the call to convert panics to `API-007`.
3. **`RequestIdMiddleware`** generates a UUID, stores it in context + slog
   attributes, sets `X-Request-ID` on the response.
4. **`LoggingMiddleware`** logs the request line and the eventual status code
   + duration.
5. **`SecurityHeadersMiddleware`** sets CSP, HSTS, X-Content-Type-Options,
   X-Frame-Options, Referrer-Policy, Cache-Control.
6. **`RateLimitMiddleware`** checks the per-IP token bucket; on exhaustion
   returns `API-005` (429).
7. **`AuthMiddleware`** (on protected routes) verifies the JWT; on failure
   returns `API-004` (401).
8. **`CORSMiddleware`** enforces the allowlist.
9. **`UserController.GetUser`** runs under `log.Logged("UserController.GetUser")`:
   - ENTER logged with `reqId`.
   - Parse `{id}` from the URL (validate UUID; bad → `API-001`).
   - Call `UserService.Get(ctx, id)`, which itself is `Logged`-wrapped and
     delegates to `UserRepository.FindByID`.
   - `UserRepository.FindByID` runs inside `DatabaseFactory.Tx`, also
     `Logged`-wrapped. If the row is missing → `API-002`.
   - Return the entity; render JSON.
   - OK logged with `dur_ms`.
10. On any returned `AppError`, **`ErrorMiddleware`** maps it to the
    documented HTTP status + JSON body `{"code":"API-xxx","message":...,"reqId":...}`.

## 4. Error model

Every error returned by the core or adapters implements `AppError`:

```go
type AppError interface {
    error
    Code() string       // stable string, e.g. "API-002"
    HTTPStatus() int    // 400, 404, 409, 429, 500, 503 ...
    Category() string   // "validation", "not-found", "conflict", ...
}
```

`ErrorMiddleware` is the single place that translates `AppError` → HTTP. This
is the rule that makes the error surface stable across releases. Bare errors
that escape the middleware become `API-007` (500) and are logged at ERROR
with the original stack.

## 5. Logging model

`AppLogger` (built on `log/slog`) is shared by both modules. Key properties:

- **`Logged("fn")` wrapper**: returns a `func() error` decorator that emits
  `ENTER` on entry, `OK` on success, `ERROR` (with the error) on failure.
  Applied to **every** public function in both modules (v1.4.0 raised
  generated-api coverage from 3/70 to 35+/70).
- **`log.Error(err)` before rethrow**: `Settings.ToSettings` and
  `DatabaseFactory.Init/Migrate/Tx` all log the underlying cause with full
  stack trace *before* wrapping in a typed `ConfigError` / `DatabaseError`.
  This is the v1.4.0 fix for the lost-stack-trace P0.
- **JSON output** when `LOG_FORMAT=json`; text otherwise. The handler is
  chosen at logger construction; switching is a one-env-var change.
- **Env levels**: `LOG_LEVEL` sets the root, `HEXAPI_LOG_LEVEL` overrides the
  `hexapi` subtree.
- **Request-id correlation**: every log line within a request carries the
  `reqId` slog attribute set by `RequestIdMiddleware`.

## 6. Database access

- All DB access goes through `DatabaseFactory.Tx(ctx, fn)` — never a raw
  `db.Query` from a handler. `Tx` opens a transaction, calls `fn(tx)`, commits
  on nil error, rolls back on non-nil.
- **P0-3.1 (v1.3.0) + v1.4.0 hardening**: `Tx` has an `AppError` pass-through
  catch — if `fn` returns an `AppError`, `Tx` re-returns it unchanged so the
  caller's typed-error mapping (e.g. `UserRepository.Save` mapping a
  duplicate-key to `API-003`) is not swallowed by a generic catch.
- Migrations live in `generated-api/resources/migrations/*.sql` and are
  applied by `DatabaseFactory.Migrate` (idempotent, ordered by filename).

## 7. Concurrency & shutdown

- `DatabaseFactory` returns a `*sql.DB` whose pool is tuned by
  `DB_POOL_SIZE` / `DB_MAX_OVERFLOW` / `DB_CONN_TIMEOUT`.
- `cmd/api` installs a `signal.NotifyContext` for `SIGINT`/`SIGTERM`.
  On signal, the HTTP server's `Shutdown(ctx)` is called with a deadline of
  `GRACEFUL_SHUTDOWN_TIMEOUT` seconds (default 5), draining in-flight
  requests before the process exits.
- `EnterpriseBus` and `DomainEventBus` are safe for concurrent use; each
  handler invocation runs in the caller's goroutine, so back-pressure is the
  caller's responsibility.

## 8. Generator pipeline

```
CLI flags → Settings.ToSettings (GEN-001 on bad config)
              │
              ▼
   DatabaseFactory.Init  (GEN-002 on connect failure)
              │
              ▼
   DatabaseFactory.Migrate (introspection DB; idempotent)
              │
              ▼
   SchemaIntrospector.Introspect        ← uses parameter-bound queries (P1-3.9)
     ├─ tables (name, isView)
     ├─ columns (name, type, nullable, enum labels)
     └─ enums (queryEnumValues: $1 placeholder)
              │
              ▼
   Naming.apply (snake_case → Go identifiers)   ← GEN-005 on unsanitisable
              │
              ▼
   TemplateEngine.executeAll(templates, model)
              │
              ▼
   Generator.writeFiles(outputDir)
              │
              ▼
   GenerationReport  (files written, LOC, duration)
```

Every step runs under `AppLogger.Logged("...")`; every error is logged at
ERROR with stack before being re-wrapped as a typed `GEN-*` error.

## 9. Security posture

- No default passwords (P0-7.13). `docker-compose.yml` fails fast without
  `.env`.
- CORS allowlist (P0-7.9), never `*`.
- Rate limiting per IP (default 100 rpm).
- Security headers on every response.
- JWT auth on protected routes; `JWT_SECRET` required.
- SQL injection mitigated: every introspection query uses placeholders
  (P1-3.9, hardened in v1.4.0).
- No secrets in logs: `AppLogger` redacts a configured list of attribute
  names (`password`, `secret`, `token`).

## 10. Testing strategy

- **Unit**: every public function has at least one happy-path and one
  error-path test. Table-driven where it reads better.
- **Logged enforcement**: a `TestAllPublicFunctionsAreLogged` test in each
  module uses `go/ast` to walk every exported function and assert that the
  first statement is a `log.Logged(...)` call (or the function is in an
  allowlist of pure helpers). This is how we keep the 35+/70 promise from
  regressing.
- **Integration**: the CI workflow brings up a real PostgreSQL service
  container and runs `make run-integration`, which generates a scaffold from
  the live schema and smoke-tests the reference API.
- **Race**: `go test -race` is the default in `make test` and CI.
