# Setup guide

This is the deep-dive setup walkthrough for `hexagonal-scaffolder` v1.4.0. The
root `README.md` has the short version; this file covers every environment,
the full env-var matrix, IDE setup, TLS, and the production checklist.

## Table of contents

1. [Prerequisites](#1-prerequisites)
2. [Quick local setup (no Docker)](#2-quick-local-setup-no-docker)
3. [Docker setup (recommended)](#3-docker-setup-recommended)
4. [Environment variable reference](#4-environment-variable-reference)
5. [IDE & tooling](#5-ide--tooling)
6. [TLS / HTTPS](#6-tls--https)
7. [Running the generator](#7-running-the-generator)
8. [Production checklist](#8-production-checklist)
9. [Uninstall](#9-uninstall)

---

## 1. Prerequisites

| Tool | Min version | Why | Install |
| --- | --- | --- | --- |
| Go | 1.22 | Both modules use `log/slog` (1.21+) and standard library features. | https://go.dev/dl/ |
| PostgreSQL | 13 | `information_schema` queries and `RETURNING` clauses. | https://www.postgresql.org/download/ |
| Docker + Compose | 24+ / v2 | Full stack via `make up`. | https://docs.docker.com/get-docker/ |
| make | any | Cross-project tasks. | `apt install make` / `brew install make` |
| git | any | Version control, pre-commit hooks. | https://git-scm.com/ |
| pre-commit (optional) | any | Local hooks before push. | `pip install pre-commit` |

Verify:

```bash
go version           # go1.22.x
psql --version       # 13+
docker --version     # 24+
make --version
```

## 2. Quick local setup (no Docker)

Use this when you already have a PostgreSQL running somewhere and you want to
run the API and tests directly.

```bash
# 1. Get the code
git clone <your-fork> hexagonal-scaffolder
cd hexagonal-scaffolder

# 2. Configure (root .env is read by docker-compose only; for local runs
#    export the vars into your shell).
cp .env.example .env
# Edit .env — set POSTGRES_PASSWORD, JWT_SECRET, POSTGRES_HOST=127.0.0.1
export $(grep -v '^#' .env | xargs)

# 3. Create the DB and apply migrations (one-off)
createdb -h 127.0.0.1 -U "$POSTGRES_USER" "$POSTGRES_DB" || true
psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -f generated-api/resources/migrations/0001_init.sql

# 4. Test both modules
make test

# 5. Run the API
make run-api
# → curl http://localhost:8080/health/ready

# 6. Run the generator against your schema
make run-generator
```

## 3. Docker setup (recommended)

```bash
cp .env.example .env
# Edit .env — at minimum set:
#   POSTGRES_PASSWORD=<a real secret>
#   JWT_SECRET=<a real secret>

make up        # docker compose up --build
# In another shell:
make migrate   # apply migrations to the running postgres
make smoke     # curl /health/ready and /users
```

`docker-compose.yml` will **refuse to start** if `POSTGRES_PASSWORD` or
`JWT_SECRET` are missing or empty. This is the v1.4.0 hardening for P0-7.13 —
no more `changeme` defaults.

### Bringing it down

```bash
make down      # docker compose down -v (wipes the postgres volume)
```

### Logs

The API logs to stdout in JSON by default (`LOG_FORMAT=json`). Tail with:

```bash
docker compose logs -f api
```

## 4. Environment variable reference

All configuration is env-var driven. `.env.example` is the canonical template;
this table is the reference.

| Variable | Default | Required | Applies to | Description |
| --- | --- | --- | --- | --- |
| `POSTGRES_USER` | `hexapi` | no | both | DB user. |
| `POSTGRES_PASSWORD` | — | **yes** | both | DB password. compose fails fast if unset. |
| `POSTGRES_DB` | `hexapi_db` | no | both | DB name. |
| `POSTGRES_HOST` | `postgres` | no | both | Hostname the app uses to reach postgres. Use `127.0.0.1` for local non-compose runs. |
| `POSTGRES_PORT` | `5432` | no | both | DB port. |
| `DB_POOL_SIZE` | `10` | no | api | Max cached connections. |
| `DB_MAX_OVERFLOW` | `20` | no | api | Extra connections allowed under burst. |
| `DB_CONN_TIMEOUT` | `5` | no | api | Seconds to wait for an idle conn. |
| `HTTP_PORT` | `8080` | no | api | HTTP listen port. |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:8080` | no | api | Comma-separated allowlist. Never `*`. |
| `RATE_LIMIT_RPM` | `100` | no | api | Per-IP requests/minute. `0` disables. |
| `GRACEFUL_SHUTDOWN_TIMEOUT` | `5` | no | api | Seconds to drain on SIGTERM. |
| `LOG_LEVEL` | `INFO` | no | both | Root log level: DEBUG/INFO/WARN/ERROR. |
| `HEXAPI_LOG_LEVEL` | `DEBUG` | no | both | Overrides the hexapi package subtree. |
| `LOG_FORMAT` | `text` (dev), `json` (docker) | no | both | `json` for structured output. |
| `JWT_SECRET` | — | **yes in prod** | api | HMAC-SHA256 signing key. |
| `JWT_ISSUER` | `hexapi` | no | api | `iss` claim. |
| `JWT_TTL_MINUTES` | `60` | no | api | Token lifetime. |
| `GENERATOR_OUTPUT_DIR` | `./out` | no | generator | Where the scaffold is written. |
| `GENERATOR_MODULE_PATH` | `github.com/example/generated-api` | no | generator | Module path stamped into generated `go.mod`. |

## 5. IDE & tooling

### VS Code (recommended)

Install the **Go** extension (`golang.go`). Recommended `settings.json`:

```json
{
  "go.useLanguageServer": true,
  "gopls": { "ui.diagnostic.staticcheck": true },
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "workspace",
  "[go]": { "editor.formatOnSave": true, "editor.defaultFormatter": "golang.go" }
}
```

### GoLand / IntelliJ

Enable the Go plugin. Set `Project GOPATH` to your `$GOPATH`. Point the
formatter at `gofmt -s`.

### Command-line tooling

```bash
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.1
pip install pre-commit
pre-commit install
```

### Database GUI

Any GUI that speaks Postgres works: `psql`, `pgAdmin`, `DBeaver`, TablePlus.
Connection details come from `.env`.

## 6. TLS / HTTPS

The server speaks plain HTTP by default. Terminate TLS at a reverse proxy
(`nginx`, `Caddy`, an ALB, etc.) in production. A minimal `Caddyfile`:

```caddy
api.example.com {
  reverse_proxy localhost:8080
}
```

Set `CORS_ALLOWED_ORIGINS` to the HTTPS origin and `Cache-Control: no-store`
on auth responses is already applied by `SecurityHeadersMiddleware`.

If you must run TLS in-process (not recommended), wrap `server.ListenAndServe`
with `ListenAndServeTLS(certFile, keyFile)` in `cmd/api/main.go` and ensure
the cert is rotated out of band.

## 7. Running the generator

```bash
cd generator
go run ./cmd/generator \
  --db-host 127.0.0.1 \
  --db-port 5432 \
  --db-user "$POSTGRES_USER" \
  --db-password "$POSTGRES_PASSWORD" \
  --db-name "$POSTGRES_DB" \
  --out ../out \
  --module github.com/example/generated-api \
  --force
```

Flags:

| Flag | Env equivalent | Default | Description |
| --- | --- | --- | --- |
| `--db-host` | `POSTGRES_HOST` | `127.0.0.1` | DB host. |
| `--db-port` | `POSTGRES_PORT` | `5432` | DB port. |
| `--db-user` | `POSTGRES_USER` | `hexapi` | DB user. |
| `--db-password` | `POSTGRES_PASSWORD` | — | DB password (required). |
| `--db-name` | `POSTGRES_DB` | `hexapi_db` | DB name. |
| `--out` | `GENERATOR_OUTPUT_DIR` | `./out` | Output directory. |
| `--module` | `GENERATOR_MODULE_PATH` | `github.com/example/generated-api` | Module path for generated `go.mod`. |
| `--force` | — | `false` | Overwrite a non-empty `--out`. |

The generator writes a runnable hexagonal API under `--out`. Diff it against
`generated-api/` to confirm the generator output matches the golden template.

## 8. Production checklist

Before promoting to production:

- [ ] `.env` is provisioned by your secret manager (Vault, AWS SSM, etc.), not
      checked into git.
- [ ] `POSTGRES_PASSWORD` and `JWT_SECRET` are strong (≥ 32 random bytes).
- [ ] `CORS_ALLOWED_ORIGINS` is the production origin only — no wildcards.
- [ ] `RATE_LIMIT_RPM` is tuned to your legitimate peak traffic.
- [ ] `LOG_FORMAT=json` and `LOG_LEVEL=INFO` (drop to `DEBUG` only for
      incident triage).
- [ ] TLS terminates at the load balancer; `HSTS` header is honoured.
- [ ] Health checks: LB uses `/health/ready`; k8s uses `/health/live` for
      liveness and `/health/ready` for readiness.
- [ ] Metrics: scrape `/metrics` into Prometheus; alert on `hexapi_*`.
- [ ] Backups: postgres `pgdata` volume is snapshotted on a schedule.
- [ ] Graceful shutdown: `GRACEFUL_SHUTDOWN_TIMEOUT` ≥ the LB's drain timeout.
- [ ] Migrations are applied via a controlled job, not at container boot.

## 9. Uninstall

```bash
make down            # stop and remove containers + volume
rm -rf out bin coverage out-integration
# Remove the cloned directory.
```
