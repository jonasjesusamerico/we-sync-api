# Task: Create Purchase Entry

## Description

Implement a new feature to create a purchase entry record.

The purchase entry must contain the following fields:

| Field | Type | Required | Description |
|---------|------|----------|-------------|
| purchase_date | date | Yes | Date when the purchase was made. |
| description | string | Yes | Description of the purchase. |
| sector | string | Yes | Business sector or department related to the purchase. |
| payment_method | string | Yes | Payment method used for the purchase. |
| amount | decimal | Yes | Purchase amount. Must be greater than zero. |
| status | string | Yes | Current status of the purchase. |

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

## Acceptance Criteria

1. The system must allow creating a purchase entry.
2. All fields are mandatory.
3. `amount` must be greater than `0`.
4. `purchase_date` must be a valid date.
5. `status` must contain a valid value from the allowed statuses.
6. `payment_method` must contain a valid value from the allowed payment methods.
7. The purchase entry must be persisted in the database.
8. The API must return the created purchase entry after successful creation.

## Example Request

```json
{
  "purchase_date": "2026-06-20",
  "description": "Office supplies",
  "sector": "Administration",
  "payment_method": "PIX",
  "amount": 150.75,
  "status": "PAID"
}
```

## Example Response

```json
{
  "id": 1,
  "purchase_date": "2026-06-20",
  "description": "Office supplies",
  "sector": "Administration",
  "payment_method": "PIX",
  "amount": 150.75,
  "status": "PAID",
  "created_at": "2026-06-20T10:00:00Z",
  "updated_at": "2026-06-20T10:00:00Z"
}
```

## Example Go Struct

```go
type Purchase struct {
    ID            uint      `json:"id"`
    PurchaseDate  time.Time `json:"purchase_date"`
    Description   string    `json:"description"`
    Sector        string    `json:"sector"`
    PaymentMethod string    `json:"payment_method"`
    Amount        float64   `json:"amount"`
    Status        string    `json:"status"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```