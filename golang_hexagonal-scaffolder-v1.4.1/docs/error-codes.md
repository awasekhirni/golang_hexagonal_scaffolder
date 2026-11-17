# Error-code inventory

Every error surfaced by `hexagonal-scaffolder` carries a **stable string
code**. Clients branch on the code, never on the message text (which may
change between releases). This file is the authoritative inventory — every
code, its HTTP status, category, where it is thrown, when, and how to recover.

Codes are versioned: a code never changes its meaning within a major version.
New conditions get a new code. Codes are retired only across major versions,
and even then only with a migration note in `CHANGELOG.md`.

## Categories

| Category | Meaning |
| --- | --- |
| `validation` | Client sent malformed or semantically invalid input. |
| `not-found` | Referenced entity does not exist. |
| `conflict` | Mutation violates a uniqueness or state constraint. |
| `auth` | Authentication or authorisation failure. |
| `rate-limit` | Caller exceeded the request budget. |
| `availability` | A required dependency (DB, bus) is unhealthy. |
| `internal` | Uncaught panic or unexpected error. |
| `domain` | Domain invariant violated by otherwise-valid input. |
| `config` | Required configuration missing or invalid. |
| `database` | Connection, migration, or transaction failure. |
| `introspection` | Generator cannot read the schema. |
| `generation` | Generator cannot render or write a file. |
| `naming` | Identifier cannot be sanitised to a Go identifier. |

## Runtime error codes (`API-*`)

| Code | HTTP | Category | Thrown by | When | Recovery |
| --- | --- | --- | --- | --- | --- |
| `API-001` | 400 | validation | `UserController`, `UserService`, route param parsing | Request body or path/query param is malformed, wrong type, or fails a validation rule. | Fix the request payload; see the `message` for the offending field. |
| `API-002` | 404 | not-found | `UserController`, `UserRepository.FindByID` | An entity referenced by id does not exist. | Check the id; if the client believes it exists, re-fetch the list. |
| `API-003` | 409 | conflict | `UserRepository.Save` (P0-3.1) | A unique constraint was violated (e.g. duplicate email on create). | Use a different unique value, or `Update` instead of `Create`. |
| `API-004` | 401 | auth | `AuthMiddleware` | JWT missing, malformed, expired, or signature mismatch. | Obtain a fresh token and retry with `Authorization: Bearer <token>`. |
| `API-005` | 429 | rate-limit | `RateLimitMiddleware` | Per-IP request budget exceeded (default 100 req/min). | Back off; respect `Retry-After`. Raise `RATE_LIMIT_RPM` if legitimate. |
| `API-006` | 503 | availability | `HealthRoute` (`/health/ready`) | DB probe `SELECT 1` failed within the readiness timeout. | Wait and retry; investigate the DB or pool config. |
| `API-007` | 500 | internal | `ErrorMiddleware` | Uncaught panic, or a non-`AppError` escaped the handler. | Look up the `reqId` in logs; file an issue with the stack trace. |
| `API-008` | 422 | domain | `UserEntity.Create` (P0-3.2) | Input is structurally valid but violates a domain invariant (empty email, negative id, etc.). | Correct the offending field; the `message` names it. |
| `API-009` | 405 | method-not-allowed | `chi` router | HTTP method not allowed on this route. | Use one of the documented methods for the resource. |
| `API-010` | 415 | unsupported-media-type | `UserController` | `Content-Type` is not `application/json` on a body-bearing request. | Set `Content-Type: application/json`. |
| `API-011` | 504 | gateway-timeout | `EnterpriseBus.AskResolved` | A synchronous ask timed out waiting for a handler response. | Retry with idempotency; investigate the downstream handler. |
| `API-012` | 502 | bad-gateway | `DomainEventBus.Publish` | A downstream event subscriber returned an error. | Retry; inspect subscriber logs. |

## Generator error codes (`GEN-*`)

| Code | Category | Thrown by | When | Recovery |
| --- | --- | --- | --- | --- |
| `GEN-001` | config | `Settings.ToSettings` | A required env var or flag is missing, empty, or fails to parse. | Set the variable listed in the error `message`; see `.env.example`. |
| `GEN-002` | database | `DatabaseFactory.Init/Migrate/Tx` | Cannot connect, ping fails, migration file missing or SQL fails, transaction fails. | Verify host/port/credentials; check migration files; ensure the DB is reachable. |
| `GEN-003` | introspection | `SchemaIntrospector` | `information_schema` query fails (permissions, malformed result). | Grant read on `information_schema`; check DB version. |
| `GEN-004` | generation | `Generator`, `TemplateEngine` | Template parse or execute failure, or file write failure (disk full, permission). | Inspect the template; check output dir permissions. |
| `GEN-005` | naming | `Naming` | An identifier cannot be sanitised to a valid Go identifier (e.g. starts with a digit after stripping). | Rename in the DB, or extend `Naming.sanitise`. |
| `GEN-006` | config | `Generator` | Output directory exists and is non-empty and `--force` was not passed. | Pass `--force` or choose a different `--out`. |
| `GEN-007` | template | `TemplateEngine` | A template references a helper that does not exist, or returns an error. | Fix the template; check helper registration. |

## Wire format

Errors returned by the HTTP layer are JSON:

```json
{
  "code": "API-002",
  "message": "user not found: id=123e4567-e89b-12d3-a456-426614174000",
  "reqId": "a1b2c3d4-..."
}
```

- `code` is one of the codes above.
- `message` is human-readable and may change between releases; do not branch
  on it.
- `reqId` correlates with the `X-Request-ID` response header and the
  structured logs.

## Adding a new code

1. Append a row to the appropriate table above.
2. Add the constant in `internal/shared/errors/codes.go` (runtime) or
   `internal/errors/codes.go` (generator).
3. Add a test asserting the code is returned for the documented condition.
4. Reference the new code in the changelog entry for the release that
   introduces it.
