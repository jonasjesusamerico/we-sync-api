# NNN — Feature name: plan

- **Spec:** [spec.md](spec.md)
- **Status:** Draft

## Approach

Short description of the chosen solution and why it is the simplest one that satisfies the spec.

## Affected code

| Path | Change |
|------|--------|
| `internal/<domain>/model.go` | |
| `internal/<domain>/repository*.go` | |
| `internal/<domain>/service.go` | |
| `internal/<domain>/handler.go` | |
| `internal/router` | |
| `cmd/api/main.go` | |

## Database

New migration(s) `migrations/NNNNNN_name.{up,down}.sql`: tables, columns, constraints, indexes.
Impact on existing data and rollback.

## API contract changes

Endpoints added/changed, status codes, Swagger regeneration needed (yes/no).

## Security and data isolation

How ownership is enforced (queries filtered by `user_id`, not-found for other users' resources).

## Observability

Logs, traces, metrics added or changed.

## Test strategy

| AC | Level | Test |
|----|-------|------|
| AC-1 | service / handler / repository | |

## Risks and alternatives considered

-
