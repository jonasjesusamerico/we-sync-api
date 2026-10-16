# 001 — Payment methods management: plan

- **Spec:** [spec.md](spec.md)
- **Status:** Approved

## Approach

Create a new feature package `internal/paymentmethod` that mirrors `internal/category` file by file
(model, repository, Postgres repository, service, handler, tests). Purchase entries switch from the string
enum to a `payment_method_id` foreign key, validated in the purchase service through a consumer-side lookup
interface, exactly like `categoryLookup`.

No shared/generic "lookup table" abstraction: two similar packages are cheaper to read and change than a
generic one (rule of three, see `.claude/rules/solid-and-code-quality.md`).

## Decisions (approved 2026-10-15)

1. New migration `000006`; `000005` is not edited (rule from `CLAUDE.md`).
2. Drop the `payment_method` label from purchase metrics; the Grafana panel becomes ungrouped.
3. Disambiguate foreign key violations in the purchase repository by constraint name.
4. Update `docs/requirements/purchase-entry.md` to the new contract, pointing to this spec.

## Affected code

| Path | Change |
|------|--------|
| `migrations/000006_create_payment_methods.{up,down}.sql` | New: table, seed, `purchase_entries` column swap (see Database). |
| `internal/paymentmethod/model.go` | New: `PaymentMethod`, `AccessibleBy`, `PaymentMethodSummary`, `Create/UpdatePaymentMethodRequest`, `ErrPaymentMethodNotFound`. |
| `internal/paymentmethod/repository.go` | New: `Repository` interface with the same contracts as `category.Repository`. |
| `internal/paymentmethod/repository_postgres.go` | New: SQL implementation; `23505` → `ErrConflict`, `23503` on delete → `ErrConflict`. |
| `internal/paymentmethod/service.go` | New: Create / List / Update / Delete with the same rules as categories (trim, empty → 422, global collision → 409, global → 403, other user → 404). |
| `internal/paymentmethod/handler.go` | New: `/payment-methods` routes and swag annotations. |
| `internal/purchase/model.go` | Remove the `PaymentMethod` enum type and `IsValidPaymentMethod`; `PurchaseRequest.PaymentMethodID int64`; `Purchase.PaymentMethod paymentmethod.PaymentMethodSummary`; `ListFilter.PaymentMethodID int64`. |
| `internal/purchase/service.go` | Add `paymentMethodLookup` interface + `validatePaymentMethodAccess`; replace enum validation with `payment_method_id is required` (422); update span/metric attributes to use the ID. |
| `internal/purchase/repository_postgres.go` | Insert/update `payment_method_id`; `JOIN payment_methods` in Create, GetByID, List, Update and in the count query; filter by `pe.payment_method_id`. |
| `internal/purchase/handler.go` | Parse `payment_method_id` query param (positive integer, else 400); map `ErrPaymentMethodNotFound` → 404 `payment method not found`; update swag annotations. |
| `internal/router/router.go` | Register `PaymentMethod` handler in the authenticated group. |
| `cmd/api/main.go` | Wire `paymentmethod` repository → service → handler; pass the repository to `purchase.NewService`. |
| `http/purchase.http` | Replace `payment_method` with `payment_method_id`; replace the invalid enum example with a not-found ID example. |
| `docs/requirements/purchase-entry.md` | Update the `payment_method` field, rules and examples; point to this spec. |
| `observability/grafana/provisioning/dashboards/we-sync-api.json` | Remove `by (payment_method)` grouping (see Observability). |
| `docs/swagger/` | Regenerate. |

## Database

New migration `000006_create_payment_methods` (applied migrations are not edited, per `CLAUDE.md`):

- `payment_methods` with the same shape as `categories`: `id`, nullable `user_id` FK to `users` with
  `ON DELETE CASCADE`, `name`, timestamps; partial unique indexes on `lower(name)` for globals and
  `(user_id, lower(name))` for personal entries; index on `user_id`.
- Seed the six global payment methods (pt-BR names from the spec).
- `purchase_entries`: drop `chk_payment_method` and `payment_method`; add
  `payment_method_id BIGINT NOT NULL REFERENCES payment_methods(id)` (default `RESTRICT`, which makes
  deleting a payment method in use fail with `23503`); add an index on it.

No backfill: the spec states there is no persisted data. Adding the `NOT NULL` column fails loudly if rows
exist, which is the desired behavior here.

`down` reverses the steps (drop index/column, re-add `payment_method` + `chk_payment_method`, drop table).

## API contract changes

- New: `GET/POST /api/v1/payment-methods`, `PUT/DELETE /api/v1/payment-methods/{id}`.
- Changed: purchase request `payment_method` → `payment_method_id`; response `payment_method` becomes an
  object; list filter `payment_method` → `payment_method_id`.
- Swagger must be regenerated.

## Security and data isolation

- Every personal read/write query is scoped by `user_id`; other users' entries return not found.
- The purchase service checks `AccessibleBy(userID)` before insert/update and returns
  `ErrPaymentMethodNotFound` otherwise, so the existence of another user's payment method is never revealed.
- Global entries are read-only through the API (403 on update/delete).

## Observability

- Spans `paymentMethodService.*` / `paymentMethodRepository.*` following the category naming.
- Purchase span attributes: `purchase.payment_method` (string) → `purchase.payment_method_id` (int).
- Purchase metrics (`purchase_created_total` / `purchase_updated_total`): **drop** the `payment_method`
  label. Personal IDs are unbounded per user and would explode Prometheus cardinality; names are user data.
- `observability/grafana/provisioning/dashboards/we-sync-api.json` (panel grouping `by (payment_method)`):
  change to an ungrouped `sum(rate(...))`.

## Test strategy

Unit tests only (no database), with manual mocks, mirroring `internal/category/*_test.go`.

| AC | Level | Test |
|----|-------|------|
| AC-1 | service, handler | List returns what the repository returns for the caller; handler 200 |
| AC-2 | service, handler | Create success, trimmed name; handler 201 |
| AC-3 | service | Empty / whitespace name → validation error (create and update) |
| AC-4 | service | Global name collision → conflict; repository conflict propagated |
| AC-5 | service, handler | Update success; handler 200 |
| AC-6 | service | Update/delete of global → forbidden |
| AC-7 | service | Not found and other user's entry → not found |
| AC-8 | service, handler | Delete success; handler 204 |
| AC-9 | service, handler | Repository conflict on delete → 409 |
| AC-10 | service, handler | Missing ID → 422; inaccessible / unknown ID → 404 `payment method not found` |
| AC-11 | handler | Response body contains `payment_method` object |
| AC-12 | handler | `payment_method_id` query param parsed; invalid value → 400 |
| AC-13 | manual | `docker compose up --build` and `GET /api/v1/payment-methods` lists the six globals |

SQL in `repository_postgres.go` is not covered by unit tests (same as today); it is verified manually
through `http/purchase.http` against `docker compose`.

## Risks and alternatives considered

- **Two FKs in the purchase insert/update:** today any foreign key violation in
  `purchase/repository_postgres.go` (`isForeignKeyViolation`) is mapped to `ErrCategoryNotFound`. With a second FK the error must be
  disambiguated by `pgErr.ConstraintName`, otherwise a deleted payment method would be reported as
  "category not found".
- **Editing `000005` instead of a new migration:** cleaner history, but `CLAUDE.md` forbids editing applied
  migrations without an explicit request. Not chosen.
- **Generic lookup package shared with categories:** rejected (premature abstraction).
- **Keeping the enum with an optional custom text field:** rejected; it does not give ownership,
  uniqueness or filtering by ID.
