# ADR-001: Structured logging with `log/slog` and the `Logged` wrapper

- **Status**: Accepted
- **Date**: 2026-11-17
- **Deciders**: Hexagonal Scaffolder Maintainers
- **Supersedes**: ad-hoc `log.Printf` calls (v1.0.0–v1.2.0)

## Context

The v1.2 audit found three structural problems with logging:

1. **No entry/exit tracing**: only 3 of ~70 public functions in the
   generated API logged anything, and those that did logged inconsistent
   formats. Diagnosing a failed request meant grepping for scattered,
   differently-formatted lines.
2. **Lost stack traces**: every `catch-and-rethrow` site wrapped the original
   error into a typed `AppError` *without* first logging the cause, so the
   stack trace of the underlying `database/sql` or `net/http` error was gone
   by the time it reached `ErrorMiddleware`.
3. **No structured output**: lines were free-form text, so log aggregators
   could not reliably index `reqId`, `fn`, `status`, or `dur_ms`.

The v1.3.0 release added an `AppLogger` and a `Logged` wrapper but only
partially wired it; the v1.4.0 exhaustive analysis found the wiring still
covered only 3/70 functions and that JSON output was never actually enabled.

## Decision

1. **Use Go's `log/slog`** (standard library, 1.21+) as the logging backend.
   It is structured by design, has a JSON handler built in, and needs no
   third-party dependency. The previous Kotlin version used
   `logstash-logback-encoder`; the Go equivalent is `slog.NewJSONHandler`.
2. **Wrap every public function with `AppLogger.Logged("fn")`**. The wrapper
   emits `ENTER` on entry, `OK` on success, `ERROR` (with the error) on
   failure, and carries the `reqId` from context. This is enforced by an AST
   test (`TestAllPublicFunctionsAreLogged`) so the 35+/70 promise does not
   regress.
3. **Log the cause before rethrow**. Every `catch-and-rethrow` site must call
   `log.Error("cause", "err", err)` *before* wrapping in a typed `AppError`.
   This is the fix for the lost-stack-trace P0.
4. **Env-var configurable levels and format**. `LOG_LEVEL` sets the root,
   `HEXAPI_LOG_LEVEL` overrides the hexapi subtree, and `LOG_FORMAT=json`
   switches the handler to `slog.NewJSONHandler`. Defaults: `INFO` / `DEBUG` /
   `text` for local dev, `json` for Docker.
5. **Redact secrets**. `AppLogger` drops any attribute whose key is in
   `{password, secret, token, jwt}`. This is a defence-in-depth measure; it
   does not excuse logging secrets in the first place.

## Consequences

- **Positive**: every request leaves a consistent, machine-parseable trail
  keyed by `reqId`. Lost-stack-trace bugs are gone by construction.
  Aggregators (Loki, Datadog, ELK) can index `fn`, `status`, `dur_ms`
  directly.
- **Positive**: the AST test makes the "every public function is logged"
  invariant self-enforcing; new code cannot silently regress it.
- **Negative**: ~5–10% runtime overhead from the extra log calls at DEBUG.
  Mitigated by running at `INFO` in production (the `ENTER`/`OK` lines are
  DEBUG-level; `ERROR` is always emitted).
- **Negative**: the `Logged` wrapper adds one closure allocation per call.
  Acceptable for an HTTP API; would be reconsidered for a hot inner loop.

## Alternatives considered

- **`zap` / `zerolog`**: faster, but a third-party dependency for a benefit
  that does not matter at our request rates. `slog` keeps the dependency
  tree small.
- **OpenTelemetry logs**: promising but the Go logs SDK was not stable at
  decision time. Revisit in a future ADR if/when we adopt OTel traces end to
  end.
- **A code generator for the `Logged` wrapper**: would remove the boilerplate
  but adds a build step and a learning curve. The AST test already enforces
  the contract; the boilerplate is the price of explicitness.

## References

- `internal/shared/applogger/applogger.go` (generated-api)
- `internal/applogger/applogger.go` (generator)
- `docs/ARCHITECTURE.md` §5 Logging model
- Go blog: [Structured Logging with slog](https://go.dev/blog/slog)
