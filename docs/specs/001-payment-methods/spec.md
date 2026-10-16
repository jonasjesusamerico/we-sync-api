# 001 — Payment methods management

- **Status:** Done
- **Author:** jonasjesusamerico
- **Date:** 2026-10-15

## Context

Purchase entries store `payment_method` as a fixed enum (`CASH`, `CREDIT_CARD`, `DEBIT_CARD`, `PIX`,
`BANK_TRANSFER`, `BOLETO`), validated in code and by a `CHECK` constraint. Users cannot express the
instruments they actually use (e.g. a specific credit card or a meal voucher), so they cannot separate
spending per instrument.

Categories already solve the same problem: system-defined global entries plus personal entries per user.
Payment methods will follow the same model.

## Goals

- Provide global payment methods available to every user.
- Let each user create, rename, and delete their own personal payment methods.
- Purchase entries reference a payment method by ID, restricted to global or the user's own entries.

## Non-goals

- Payment method types/behavior (credit card statements, installments, closing dates). See open questions.
- Changing `status`, which remains a fixed enum.
- Backwards compatibility with the string `payment_method` field: the project has no deployed clients
  and no persisted data.

## Functional requirements

### Data

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | integer | — | Generated identifier. |
| user_id | integer \| null | — | `null` for global payment methods; owner ID for personal ones. |
| name | string | Yes | Display name. Trimmed; must not be empty. |
| is_global | boolean | — | `true` when `user_id` is `null`. |
| created_at / updated_at | timestamp | — | Audit timestamps. |

### Global payment methods (seeded)

Names are user-facing data, in pt-BR:

- Dinheiro
- Cartão de crédito
- Cartão de débito
- Pix
- Transferência bancária
- Boleto

### Business rules

- Global payment methods cannot be updated or deleted through the API.
- Names are unique case-insensitively among globals, and among each user's personal entries.
- A personal payment method cannot have the same name (case-insensitive) as a global one.
- A payment method referenced by any purchase entry cannot be deleted.

### Ownership and authorization

- A user sees global payment methods and only their own personal ones.
- A personal payment method owned by another user behaves as not found (`404`) in every operation,
  including when referenced by `payment_method_id` in a purchase entry.

## Acceptance criteria

- **AC-1:** `GET /api/v1/payment-methods` returns global payment methods plus the caller's personal ones, and never another user's.
- **AC-2:** `POST /api/v1/payment-methods` creates a personal payment method and returns `201` with the created resource.
- **AC-3:** Creating or renaming with an empty or whitespace-only name returns `422`.
- **AC-4:** Creating or renaming to a name that already exists among globals or the caller's own entries (case-insensitive) returns `409`.
- **AC-5:** `PUT /api/v1/payment-methods/{id}` renames a personal payment method owned by the caller and returns `200`.
- **AC-6:** Updating or deleting a global payment method returns `403`.
- **AC-7:** Updating or deleting a payment method that does not exist or belongs to another user returns `404`.
- **AC-8:** `DELETE /api/v1/payment-methods/{id}` removes an unused personal payment method and returns `204`.
- **AC-9:** Deleting a payment method referenced by purchase entries returns `409`.
- **AC-10:** Purchase create/update accept `payment_method_id` (required) instead of `payment_method`; an ID that does not exist or belongs to another user returns `404` with `"payment method not found"`.
- **AC-11:** Purchase responses include `payment_method` as a summary object `{id, name, is_global}`.
- **AC-12:** The purchase list filter `payment_method` (enum) is replaced by `payment_method_id`.
- **AC-13:** The six global payment methods listed above exist after migrations run.

## API contract (draft)

### Payment methods

Mirrors `/api/v1/categories`.

`POST /api/v1/payment-methods`

```json
{ "name": "Nubank" }
```

Response — `201 Created`:

```json
{
  "id": 7,
  "user_id": 42,
  "name": "Nubank",
  "is_global": false,
  "created_at": "2026-10-15T10:00:00Z",
  "updated_at": "2026-10-15T10:00:00Z"
}
```

### Purchase entries (changed fields only)

Request:

```json
{ "payment_method_id": 7 }
```

Response:

```json
{
  "payment_method": { "id": 7, "name": "Nubank", "is_global": false }
}
```

List filter: `GET /api/v1/purchases?payment_method_id=7`

### Errors

| Status | When | Message |
|--------|------|---------|
| 400 | Malformed JSON or invalid ID in path/query | |
| 401 | Missing or invalid access token | |
| 403 | Update/delete of a global payment method | |
| 404 | Payment method not found or owned by another user | `payment method not found` |
| 409 | Duplicate name, or delete of a payment method in use | |
| 422 | Empty name, or missing `payment_method_id` in a purchase | |

## Open questions

- [x] Existing data to migrate? — No; the database is recreated on every run.
- [x] Clients depending on the string enum? — No.
- [x] Global names in pt-BR? — Yes.
- [x] Personal payment methods carry only a name, or also a type (credit, debit, pix...)? — Name only;
      a type will be added when a feature needs type-specific behavior (e.g. credit card statements).

## Changelog

- 2026-10-15 — Created.
- 2026-10-15 — Approved; payment methods carry only a name.
- 2026-10-15 — Done; all acceptance criteria validated (unit tests + manual run, see tasks.md T16).
