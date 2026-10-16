# 001 — Payment methods management: tasks

- **Spec:** [spec.md](spec.md)
- **Plan:** [plan.md](plan.md)

Each task is small, independently verifiable and references the acceptance criteria it covers.
Run `go build ./...` and the package tests after each task.

## Database

- [x] T1 — Migration `000006_create_payment_methods.{up,down}.sql`: table, unique indexes, `user_id` index,
      seed of the six globals, column swap in `purchase_entries` with FK and index. (AC-13, AC-9)

## Payment method package

- [x] T2 — `internal/paymentmethod/model.go`: entity, `AccessibleBy`, summary, request DTOs,
      `ErrPaymentMethodNotFound`. (AC-10, AC-11)
- [x] T3 — `repository.go` + `repository_postgres.go`: `Create`, `GetByID`, `ListForUser`,
      `ExistsGlobalByName`, `Update`, `Delete`, with `23505`/`23503` → `ErrConflict`. (AC-1, AC-4, AC-9)
- [x] T4 — `service.go` + `service_test.go`: create/list/update/delete rules. (AC-1 to AC-9)
- [x] T5 — `handler.go` + `handler_test.go`: routes, error→status mapping, swag annotations. (AC-1 to AC-9)
- [x] T6 — Register routes in `internal/router` and wire dependencies in `cmd/api/main.go`. (AC-1)

## Purchase integration

- [x] T7 — `purchase/model.go`: remove `PaymentMethod` enum; add `PaymentMethodID` to request and filter,
      `PaymentMethodSummary` to the entity. (AC-10, AC-11, AC-12)
- [x] T8 — `purchase/service.go` + tests: `paymentMethodLookup`, `validatePaymentMethodAccess`,
      `payment_method_id is required`; span attribute by ID; drop the metric label. (AC-10)
- [x] T9 — `purchase/repository_postgres.go`: `payment_method_id` in insert/update, `JOIN payment_methods`
      in every read and in the count query, filter by ID, FK violation disambiguated by constraint name. (AC-10, AC-11, AC-12)
- [x] T10 — `purchase/handler.go` + tests: `payment_method_id` query param (400 if invalid),
      `ErrPaymentMethodNotFound` → 404, swag annotations. (AC-10, AC-11, AC-12)

## Docs and tooling

- [x] T11 — Grafana dashboard: remove `by (payment_method)` grouping.
- [x] T12 — `http/purchase.http`: use `payment_method_id`; replace the invalid enum example with a not-found ID.
- [x] T13 — `docs/requirements/purchase-entry.md`: new field, rules, examples and errors; link to this spec.
- [x] T14 — Regenerate Swagger and check `git diff --stat docs/swagger`.
- [x] T15 — `gofmt -l internal cmd`, `go vet ./...`, `go build ./...`, `go test ./...`.
- [x] T16 — Manual check: the user runs `docker compose up --build`; Claude runs curl scenarios against
      `:8080` and validates each response (globals listed, CRUD, purchase with personal payment method,
      another user's payment method → 404, delete in use → 409). (AC-13; SQL paths)
      Result 2026-10-15: 36/36 checks passed against a freshly recreated database.

## Notes

Decisions taken during implementation that deviate from the plan.

- T9: `JOIN payment_methods` was **not** added to the `List` count query. It reads no column from that
  table and the `NOT NULL` FK guarantees the join never changes the count; the extra join would only cost.
- T1: no expand/contract (`persistence.md`, rolling deploy). Dropping `payment_method` in the same
  migration is safe only because nothing is deployed and the database is recreated on every run.
- Payment method names have no maximum length, mirroring categories. `security.md` asks for a limit on all
  free text; this gap exists in both domains and is left for a dedicated task.
- T16: not covered end to end: a payment method deleted between the service check and the purchase
  insert (FK race mapped by `referenceNotFound`); it would need a concurrent scenario or a repository test.
