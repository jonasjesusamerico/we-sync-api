package purchase

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

// Status represents the lifecycle state of a purchase entry.
type Status string

const (
	StatusPending  Status = "PENDING"
	StatusPaid     Status = "PAID"
	StatusCanceled Status = "CANCELED"
)

// IsValid returns true if the status is one of the allowed values.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusPaid, StatusCanceled:
		return true
	}
	return false
}

// IsValidStatus is a convenience wrapper kept for backwards compatibility with tests.
func IsValidStatus(s string) bool { return Status(s).IsValid() }

// ---------------------------------------------------------------------------
// Date — a date-only type that serialises as "YYYY-MM-DD" in JSON.
// ---------------------------------------------------------------------------

// Date wraps time.Time and marshals/unmarshals only the date portion (no time, no timezone).
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

// MarshalJSON encodes the date as "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Time.Format(dateLayout))
}

// UnmarshalJSON decodes a "YYYY-MM-DD" string into Date.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("date must be in YYYY-MM-DD format: %w", err)
	}
	d.Time = t
	return nil
}

// ---------------------------------------------------------------------------
// Core entities
// ---------------------------------------------------------------------------

// Purchase represents a purchase entry record.
type Purchase struct {
	ID            int64                              `json:"id"`
	UserID        int64                              `json:"user_id"`
	PurchaseDate  Date                               `json:"purchase_date"`
	Description   string                             `json:"description"`
	Category      category.CategorySummary           `json:"category"`
	PaymentMethod paymentmethod.PaymentMethodSummary `json:"payment_method"`
	Amount        decimal.Decimal                    `json:"amount"`
	Status        Status                             `json:"status"`
	CreatedAt     time.Time                          `json:"created_at"`
	UpdatedAt     time.Time                          `json:"updated_at"`
}

// PurchaseRequest holds the input data for creating or updating a purchase entry.
type PurchaseRequest struct {
	PurchaseDate    Date            `json:"purchase_date"`
	Description     string          `json:"description"`
	CategoryID      int64           `json:"category_id"`
	PaymentMethodID int64           `json:"payment_method_id"`
	Amount          decimal.Decimal `json:"amount"`
	Status          Status          `json:"status"`
}

// CreatePurchaseRequest is the input type for the Create operation.
type CreatePurchaseRequest struct{ PurchaseRequest }

// UpdatePurchaseRequest is the input type for the Update operation.
type UpdatePurchaseRequest struct{ PurchaseRequest }

// ListFilter holds the query parameters for filtering and paginating purchase entries.
type ListFilter struct {
	UserID          int64
	Month           int
	Year            int
	Status          Status
	CategoryID      int64
	PaymentMethodID int64
	Page            int
	PageSize        int
}

// MaxPageSize is the upper bound accepted by the service for PageSize.
const MaxPageSize = 100

// PaginatedResult wraps a page of purchase entries with pagination metadata.
type PaginatedResult struct {
	Items      []Purchase `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}
