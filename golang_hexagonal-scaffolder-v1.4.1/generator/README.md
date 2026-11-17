# generator

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.4.0-orange)](../VERSION)

The **generator** is the offline CLI that introspects a live PostgreSQL schema
and emits a complete hexagonal Go HTTP API scaffold — domain entities,
repositories, application services, REST controllers, middleware, buses, and
health/metrics endpoints — following the same conventions as the
[`generated-api`](../generated-api/README.md) reference module.

Every public function is wrapped by `AppLogger.Logged` (entry/exit/error
tracing), every error is typed (`GEN-*` codes from
[`docs/error-codes.md`](../docs/error-codes.md)), and every catch-and-rethrow
logs the cause with stack trace before wrapping.

## Layout

```
generator/
├── README.md                ← this file
├── go.mod
├── application.yaml         ← default config (env vars override)
├── .env.example             ← generator-only env template
├── cmd/generator/main.go    ← CLI entry point
├── internal/
│   ├── applogger/           ← AppLogger + Logged wrapper (slog JSON)
│   ├── settings/            ← Env → typed Settings (GEN-001)
│   ├── naming/              ← snake/Pascal/Camel/Go-identifier (GEN-005)
│   ├── templateengine/      ← text/template wrapper + helpers
│   ├── generationreport/    ← files-written report + metrics
│   ├── schemaintrospector/  ← information_schema reader (P1-3.9: bound params)
│   ├── generator/           ← orchestrates the pipeline
│   ├── databasefactory/     ← Init/Migrate/Tx (P0-3.1: AppError pass-through)
│   ├── enterprisebus/       ← DispatchResolved / AskResolved
│   └── domaineventbus/      ← Publish
├── templates/               ← text/template files for generated code
└── testdata/                ← fixtures + golden output
```

## Quick start

```bash
cd generator
go run ./cmd/generator \
  --db-host 127.0.0.1 --db-port 5432 \
  --db-user hexapi --db-password "$POSTGRES_PASSWORD" \
  --db-name hexapi_db \
  --out ../out --module github.com/example/generated-api --force
```

The generated project under `../out` is runnable: `cd ../out && go run ./cmd/api`.

## Flags

| Flag | Env | Default | Description |
| --- | --- | --- | --- |
| `--db-host` | `POSTGRES_HOST` | `127.0.0.1` | DB host. |
| `--db-port` | `POSTGRES_PORT` | `5432` | DB port. |
| `--db-user` | `POSTGRES_USER` | `hexapi` | DB user. |
| `--db-password` | `POSTGRES_PASSWORD` | — | DB password (required). |
| `--db-name` | `POSTGRES_DB` | `hexapi_db` | DB name. |
| `--out` | `GENERATOR_OUTPUT_DIR` | `./out` | Output directory. |
| `--module` | `GENERATOR_MODULE_PATH` | `github.com/example/generated-api` | Module path for generated go.mod. |
| `--log-level` | `LOG_LEVEL` | `INFO` | Root log level. |
| `--hexapi-log-level` | `HEXAPI_LOG_LEVEL` | `DEBUG` | hexapi subtree level. |
| `--log-format` | `LOG_FORMAT` | `text` | `json` or `text`. |
| `--force` | — | `false` | Overwrite non-empty `--out`. |

## Configuration

The generator reads env vars for everything; `application.yaml` holds defaults
that env vars override. See [`.env.example`](.env.example) for the full set.
The variables documented here mirror the root README:

- `DB_POOL_SIZE`, `DB_MAX_OVERFLOW` — passed through to the generated project's
  defaults.
- `LOG_LEVEL`, `HEXAPI_LOG_LEVEL`, `LOG_FORMAT` — control the generator's own
  logging (same semantics as the generated API; see
  [ADR-001](../docs/adr/ADR-001-logging.md)).

## Testing

```bash
cd generator
go test -race ./...            # all packages
go test -race -run Naming      # just naming
UPDATE_GOLDEN=1 go test ./internal/generator/...   # refresh golden files
```

Test inventory:

| File | Covers | Cases |
| --- | --- | --- |
| `internal/naming/naming_test.go` | case conversion, sanitisation | 9 |
| `internal/templateengine/templateengine_test.go` | parse, execute, helpers, errors | 5 |
| `internal/generationreport/generationreport_test.go` | add files, metrics, render | 5 |
| `internal/schemaintrospector/schemaintrospector_test.go` | tables, columns, enums, isView, SQL-injection regression | 8 |
| `internal/settings/settings_test.go` | env parsing, required vars, defaults | 6 |
| `internal/databasefactory/databasefactory_test.go` | Init/Migrate/Tx, AppError pass-through, rollback | 6 |
| `internal/enterprisebus/enterprisebus_test.go` | dispatch, ask, timeout | 5 |
| `internal/generator/generator_test.go` | end-to-end with fake introspector | 5 |

## Error codes

See [`docs/error-codes.md`](../docs/error-codes.md) § "Generator error codes".
Summary: `GEN-001` config, `GEN-002` database, `GEN-003` introspection,
`GEN-004` generation, `GEN-005` naming, `GEN-006` output dir non-empty,
`GEN-007` template.

## Logging

`AppLogger` is built on `log/slog`. `LOG_FORMAT=json` switches the handler to
`slog.NewJSONHandler`; otherwise text. `LOG_LEVEL` sets the root;
`HEXAPI_LOG_LEVEL` overrides the `github.com/example/hexapi` subtree. Every
public function is wrapped by `log.Logged("fn")` — enforced by
`TestAllPublicFunctionsAreLogged` in `internal/applogger`.
