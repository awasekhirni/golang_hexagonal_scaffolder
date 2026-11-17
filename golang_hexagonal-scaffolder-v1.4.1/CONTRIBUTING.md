# Contributing to hexagonal-scaffolder

Thanks for your interest in improving hexagonal-scaffolder. This document is
the contract between contributors and maintainers — please read it before
opening a pull request.

## Code of conduct

Be respectful, assume good intent, and focus on the technical merits of the
change. Personal attacks, harassment and discrimination are not tolerated.

## Project layout (orientation)

- `generator/` — Go module that reads a PostgreSQL schema and emits a
  hexagonal Go API scaffold.
- `generated-api/` — Go module that is the **reference implementation** the
  generator produces. It is runnable on its own and doubles as the golden
  template.
- `docs/` — architecture, error-code inventory, setup, dev guide, ADRs.
- Root — `Makefile`, `docker-compose.yml`, CI, pre-commit, dependabot,
  `VERSION`, `CHANGELOG.md`, this file.

Each Go module has its own `go.mod`; there is no parent module. Always run
`go` commands from inside the module you mean to affect, or use the
`make` targets which cd for you.

## Fork & PR workflow

1. **Fork** the repository on GitHub and clone your fork.
2. Add the upstream remote:
   ```bash
   git remote add upstream https://github.com/<org>/hexagonal-scaffolder.git
   ```
3. Create a feature branch off the latest `main`:
   ```bash
   git checkout -b feat/<short-description>
   ```
4. Make your changes. Keep commits small and focused.
5. Run the local checks (see below). Do not push until they pass.
6. Push to your fork and open a pull request against `main`.
7. Fill in the PR template: **what**, **why**, **how to test**, **risk**.
8. Address review feedback by pushing more commits (do not force-push during
   review unless the reviewer asks for a squash).

## Local checks

```bash
make fmt      # gofmt + goimports (writes in place)
make vet      # go vet both modules
make test     # go test -race both modules
make lint     # vet + golangci-lint (if installed)
make cover    # write coverage HTML for both modules
```

CI runs the same set plus the integration target, so a green local run is a
strong signal that CI will pass. **A red `make test` is a blocker — do not
push.**

## Commit message format

We follow [Conventional Commits](https://www.conventionalcommits.org/) so the
changelog can be generated mechanically:

```
<type>(<scope>): <imperative summary>

<optional body — what and why, wrapped at 72 columns>

<optional footer — BREAKING CHANGE:, Closes #123, Since 1.4.0>
```

| type | use for |
| --- | --- |
| `feat` | a new user-facing capability |
| `fix` | a bug fix (reference the P0/P1 id when applicable, e.g. `fix(db): P0-3.1`) |
| `refactor` | internal change with no behaviour shift |
| `perf` | performance improvement |
| `docs` | documentation only |
| `test` | test-only change |
| `chore` | tooling, build, deps |
| `ci` | CI / pipeline changes |

**Examples**

```
fix(generator): use parameter-bound query in QueryEnumValues (P1-3.9)

The previous implementation interpolated the enum type name into the SQL
string, opening a SQL-injection vector if a malicious value reached the
introspector. Use $1 and pass the type name via the driver.
```

```
feat(api): wire AppLogger.Logged into UserController handlers

Wraps Get/List/Create/Update/Delete with entry/exit/error tracing. Raises
generated-api Logged coverage from 3/70 to 35+/70. Since 1.4.0.
```

## Coding standards

- **Go style**: `gofmt -s` + `goimports`. No tabs/spaces mixups.
- **Error handling**: every public function returns a typed error from
  `internal/shared/errors` (runtime) or `internal/errors` (generator). Never
  return a bare `errors.New`. Every `catch-and-rethrow` site logs
  `log.Error(err)` **before** wrapping, so the original stack is preserved.
- **Logging**: every public function is wrapped by `AppLogger.Logged("fn")`.
  If you add a public function, wire it. The `Logged` test in each module
  enforces this for the packages it covers.
- **`// Since X.Y.Z.`**: add this doc comment to every new exported function.
- **Tests**: every new function needs at least one happy-path and one
  error-path test. Table-driven where it reads better. No `time.Sleep`-based
  sync — use channels or `testing.T.Deadline()`.
- **No panics in libraries**: only `main` and `must`-style helpers in
  `cmd/` may `panic`. Everywhere else returns an error.
- **Dependencies**: justify any new module dependency in the PR description.
  Prefer the standard library; `database/sql` + `lib/pq` for Postgres,
  `chi` for routing, `prometheus/client_golang` for metrics.

## Pre-commit hooks

Install once:

```bash
pip install pre-commit
pre-commit install
```

The hooks (`.pre-commit-config.yaml`) run `gofmt`, `goimports`, `go vet`, and
`golangci-lint` on staged files. They run on every `git commit`. If a hook
fails, fix and re-stage — do not bypass with `--no-verify`.

## CI

`.github/workflows/ci.yml` runs on every push and PR:

1. Checkout.
2. Install Go 1.22.
3. Start a PostgreSQL 15 service container.
4. `go vet` + `go test -race` in both modules, against the live Postgres.
5. Upload coverage to the PR as an artifact.

A red CI step blocks merge. Required reviewers can re-run failed jobs but
cannot override a failing test.

## Releasing

1. Bump `VERSION` (single source of truth).
2. Add a `[X.Y.Z] — YYYY-MM-DD` section to `CHANGELOG.md` following Keep a
   Changelog.
3. `make test` green on `main`.
4. Tag `vX.Y.Z`; CI builds the artifacts and publishes.
5. `make zip` produces the distributable archive.
6. Draft the GitHub release from the tag, pasting the changelog section.

## Adding a new error code

1. Append a row to `docs/error-codes.md` with code, HTTP status, category,
   thrown-by, when, recovery.
2. Add the constant in `internal/shared/errors/codes.go` (runtime) or
   `internal/errors/codes.go` (generator).
3. Add a test that asserts the code is returned for the documented condition.
4. Reference the code in the changelog entry.

## Questions

Open a discussion on GitHub for design questions; open an issue for bugs.
Do not email maintainers directly — keep the conversation where others can
benefit from it.
