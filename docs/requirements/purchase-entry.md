# Task: Create Purchase Entry

## Description

Implement a feature to create a purchase entry record for the authenticated user.

The purchase entry must contain the following fields:

| Field | Type | Required | Description |
|---------|------|----------|-------------|
| purchase_date | date (`YYYY-MM-DD`) | Yes | Date when the purchase was made. |
| description | string | Yes | Description of the purchase. |
| category_id | integer | Yes | ID of a category visible to the user: a global category or one of the user's own personal categories. |
| payment_method | string | Yes | Payment method used for the purchase. |
| amount | decimal (sent as string) | Yes | Purchase amount. Must be greater than zero. |
| status | string | Yes | Current status of the purchase. |

> History: the first version of this requirement used a free-text `sector` field.
> It was replaced by `category_id`, backed by the category management feature
> (global categories seeded by the system + personal categories per user).

## Business Rules

### Allowed Status Values

- PENDING
- PAID
- CANCELED

### Allowed Payment Methods

- CASH
- CREDIT_CARD
- DEBIT_CARD
- PIX
- BANK_TRANSFER
- BOLETO

### Category Ownership

- `category_id` must reference an existing category that is either global or owned by the authenticated user.
- A category owned by another user is treated as not found, so its existence is never revealed.

## Acceptance Criteria

1. The system must allow an authenticated user to create a purchase entry.
2. All fields are mandatory.
3. `amount` must be greater than `0`.
4. `purchase_date` must be a valid date in `YYYY-MM-DD` format.
5. `status` must contain a valid value from the allowed statuses.
6. `payment_method` must contain a valid value from the allowed payment methods.
7. `category_id` must reference a category accessible to the user; otherwise the API returns `404` with `"category not found"`.
8. The purchase entry must be persisted in the database, owned by the authenticated user.
9. The API must return the created purchase entry, including a summary of its category, after successful creation.

## Example Request

`POST /api/v1/purchases` with `Authorization: Bearer <access_token>`

```json
{
  "purchase_date": "2026-06-20",
  "description": "Office supplies",
  "category_id": 3,
  "payment_method": "PIX",
  "amount": "150.75",
  "status": "PAID"
}
```

## Example Response — `201 Created`

```json
{
  "id": 1,
  "user_id": 42,
  "purchase_date": "2026-06-20",
  "description": "Office supplies",
  "category": {
    "id": 3,
    "name": "Moradia",
    "is_global": true
  },
  "payment_method": "PIX",
  "amount": "150.75",
  "status": "PAID",
  "created_at": "2026-06-20T10:00:00Z",
  "updated_at": "2026-06-20T10:00:00Z"
}
```

## Error Responses

| Status | Code | When |
|---|---|---|
| 400 | `BAD_REQUEST` | Malformed JSON or wrong `purchase_date` format |
| 401 | `UNAUTHORIZED` | Missing or invalid access token |
| 404 | `NOT_FOUND` | `category_id` does not exist or belongs to another user |
| 422 | `VALIDATION_ERROR` | A business rule above is violated |

The source of truth for the data model is `internal/purchase/model.go`
(amount uses `decimal.Decimal`, the date uses the date-only `purchase.Date` type).
