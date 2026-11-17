# Development guide

Day-to-day operations for contributors to `hexagonal-scaffolder`. The root
`README.md` covers running; this file covers the dev loop, profiling,
benchmarking, debugging, and releasing.

## Table of contents

1. [Dev loop](#1-dev-loop)
2. [Module layout recap](#2-module-layout-recap)
3. [Adding a function (the Logged contract)](#3-adding-a-function-the-logged-contract)
4. [Testing conventions](#4-testing-conventions)
5. [Profiling & benchmarking](#5-profiling--benchmarking)
6. [Debugging](#6-debugging)
7. [Database migrations](#7-database-migrations)
8. [Templates (generator)](#8-templates-generator)
9. [Releasing](#9-releasing)

---

## 1. Dev loop

The fastest feedback loop:

```bash
# In one terminal — postgres + api up:
make up

# In another terminal — watch tests on save:
cd generator     && go test -race ./...         # or
cd generated-api && go test -race ./...
```

For file-watch use `entr` or `air`:

```bash
ls generator/**/*.go | entr -c sh -c 'cd generator && go test -race ./...'
```

Format on save is enforced by pre-commit; do not commit unformatted code.

## 2. Module layout recap

```
generator/      ← Go module #1: schema → scaffold
generated-api/  ← Go module #2: reference hexagonal API
docs/           ← architecture, error codes, setup, this file, ADRs
```

There is **no parent go.mod**. Run `go` commands from inside the module you
mean to affect. `make` targets cd for you.

## 3. Adding a function (the Logged contract)

Every public function in both modules is wrapped by `AppLogger.Logged`. The
contract is enforced by `TestAllPublicFunctionsAreLogged` (an AST walk). When
you add a public function:

```go
// MyFunc does X.
// Since 1.4.0.
func (s *Service) MyFunc(ctx context.Context, in In) (Out, error) {
    return Out{}, s.log.Logged("Service.MyFunc", func() error {
        // ... body ...
        return nil
    }).(func() (Out, error)) // see the AppLogger docs for the typed helper
}
```

Use the typed helper `Logged2[In, Out]` (in `applogger`) to avoid the cast in
the common 2-return case. The wrapper emits `ENTER` on entry, `OK` on success
and `ERROR` (with the error) on failure, and is the single place that holds
the "log every entry/exit" policy.

**Error handling rule**: if you catch an error and re-throw a wrapped one,
log the original **first**:

```go
if err != nil {
    s.log.Error("cause", "err", err)   // ← before the wrap
    return ConfigError{Code: "GEN-001", Cause: err}
}
```

This is the v1.4.0 fix for the lost-stack-trace P0.

## 4. Testing conventions

- **Table-driven** for functions with several input shapes.
- **Happy + error path** for every public function.
- **No `time.Sleep`** for sync; use channels, `testing.T.Deadline()`, or
  `eventually`-style helpers.
- **`-race`** is the default in `make test` and CI.
- **Test names**: `TestSubject_Variant_expectedResult`, e.g.
  `TestNaming_SnakeToPascal_camelCasesCorrectly`.
- **Fixtures** live under `testdata/`; never read from the repo root.
- **Golden files**: when a test compares generated output, store the expected
  output under `testdata/golden/*.golden` and refresh with
  `UPDATE_GOLDEN=1 go test ./...`.

## 5. Profiling & benchmarking

CPU and heap profiles are first-class:

```bash
# CPU profile for the generator:
cd generator
go test -run=^$ -bench=. -cpuprofile=cpu.out ./internal/naming/...
go tool pprof -http=:8080 cpu.out

# Heap profile for the API under load:
cd generated-api
go test -run=^$ -bench=. -benchmem -memprofile=mem.out ./...
go tool pprof -http=:8080 mem.out
```

In production, `/debug/pprof/*` is **not** mounted by default. Enable it only
behind auth + allowlist for incident triage.

## 6. Debugging

- **Logs**: every request carries a `reqId`. Grep for it:
  ```bash
  docker compose logs api | jq 'select(.reqId=="a1b2c3d4")'
  ```
- **`delve`**: `dlv debug ./cmd/api` from `generated-api/`. Set breakpoints
  inside `Logged` to see every entry/exit.
- **DB**: `docker compose exec postgres psql -U hexapi -d hexapi_db`.
- **Strace a slow query**: enable `log_min_duration_statement=200` in postgres.

## 7. Database migrations

Migrations live in `generated-api/resources/migrations/*.sql`, named
`NNNN_description.sql` (zero-padded, ordered). `DatabaseFactory.Migrate`
applies them in order inside a transaction and tracks applied versions in a
`schema_migrations` table.

To add a migration:

1. Create `generated-api/resources/migrations/0002_add_audit_table.sql`.
2. Make it idempotent (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`).
3. Add a test in `databasefactory_test.go` that asserts the migration applies
   cleanly on a fresh DB and is a no-op on a second run.
4. Update `docs/SETUP.md` if it changes the env-var matrix.

Never edit an applied migration — write a new one instead.

## 8. Templates (generator)

Templates live in `generator/templates/*.tmpl`. They are plain `text/template`
files with custom helpers registered by `TemplateEngine`:

| Helper | Behaviour |
| --- | --- |
| `Pascal`, `Camel`, `Snake` | Case conversion via `Naming`. |
| `Lower`, `Upper` | String case. |
| `TypeMap` | SQL type → Go type. |
| `Comment` | Wraps a string as a Go doc comment. |
| `Since` | Stamps a `// Since 1.4.0.` doc line. |

When you change a template:

1. Regenerate the golden output:
   ```bash
   cd generator && UPDATE_GOLDEN=1 go test ./internal/generator/...
   ```
2. Diff the golden files to confirm the change is intentional.
3. Update `generated-api/` to match if the template is the source of truth
   for that file.

## 9. Releasing

1. **Bump version**: edit `VERSION` (single source of truth) and add a
   `[X.Y.Z] — YYYY-MM-DD` section to `CHANGELOG.md` following Keep a Changelog.
2. **Green build**: `make test` passes on `main`; CI is green.
3. **Tag**:
   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
4. **CI** builds and publishes artifacts on tag push.
5. **Archive**:
   ```bash
   make zip   # produces ../hexagonal-scaffolder-vX.Y.Z.zip
   ```
6. **GitHub release**: draft from the tag, paste the changelog section,
   attach the zip.
7. **Announce**: link the release in your team channel; mention any breaking
   changes and the migration path.

### Versioning policy

We follow [SemVer](https://semver.org/):

- **MAJOR**: incompatible API or error-code changes.
- **MINOR**: new features, new error codes, backward-compatible.
- **PATCH**: bug fixes only.

Error codes are additive within a major version: a code never changes its
meaning; new conditions get a new code.
