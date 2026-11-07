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
	return json.Marshal(d.Format(dateLayout))
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
	// Search is matched anywhere in the description, ignoring case and accents.
	// The service trims it; empty means no search.
	Search   string
	Page     int
	PageSize int
}

// maxSearchLength is counted in characters (runes), after trimming, like the description.
const maxSearchLength = 255

// MaxPageSize is the upper bound accepted by the service for PageSize.
const MaxPageSize = 100

// PaginatedResult wraps a page of purchase entries with pagination metadata.
// Total counts every status; TotalAmount sums the same filter without CANCELED.
type PaginatedResult struct {
	Items       []Purchase      `json:"items"`
	Total       int             `json:"total"`
	TotalAmount decimal.Decimal `json:"total_amount"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
	TotalPages  int             `json:"total_pages"`
}

// ---------------------------------------------------------------------------
// Month summary (GET /purchases/summary)
// ---------------------------------------------------------------------------

// ComparisonPeriod tells which days of both months the comparison covers.
type ComparisonPeriod string

const (
	// PeriodUntilDay compares day 1 up to today's day (the requested month is today's month).
	PeriodUntilDay ComparisonPeriod = "UNTIL_DAY"
	// PeriodFullMonth compares both whole months.
	PeriodFullMonth ComparisonPeriod = "FULL_MONTH"
)

// ComparisonResult is the outcome of comparing the current amount with the previous one.
type ComparisonResult string

const (
	ResultMore   ComparisonResult = "MORE"
	ResultLess   ComparisonResult = "LESS"
	ResultEqual  ComparisonResult = "EQUAL"   // rounded percent is 0
	ResultNoBase ComparisonResult = "NO_BASE" // previous amount is 0: no percent
)

// Comparison compares the month with the previous one over the same period.
type Comparison struct {
	Period         ComparisonPeriod `json:"period"`
	UntilDay       *int             `json:"until_day"` // null in FULL_MONTH
	PreviousYear   int              `json:"previous_year"`
	PreviousMonth  int              `json:"previous_month"`
	CurrentAmount  decimal.Decimal  `json:"current_amount"`
	PreviousAmount decimal.Decimal  `json:"previous_amount"`
	Result         ComparisonResult `json:"result"`
	Percent        *int64           `json:"percent"` // null only in NO_BASE
}

// MonthSummary is the response of GET /purchases/summary.
type MonthSummary struct {
	Year          int             `json:"year"`
	Month         int             `json:"month"`
	PurchaseCount int             `json:"purchase_count"` // every status
	Spent         decimal.Decimal `json:"spent"`          // PAID + PENDING of the whole month
	Comparison    Comparison      `json:"comparison"`
}

// SummaryRanges are the inclusive date bounds (UTC midnight) that Summarize sums over.
// PreviousStart and PreviousEnd are only meaningful when HasPrevious is true: January of
// year 1 has no previous month.
type SummaryRanges struct {
	MonthStart    time.Time // first day of the month
	MonthEnd      time.Time // last day of the month
	CurrentEnd    time.Time // last day of the compared current period
	HasPrevious   bool
	PreviousStart time.Time // first day of the previous month
	PreviousEnd   time.Time // last day of the compared previous period
}

// SummarySums are the sums Summarize reads; amounts exclude CANCELED.
type SummarySums struct {
	MonthCount     int             // purchases in the month, every status
	MonthSpent     decimal.Decimal // whole month
	CurrentAmount  decimal.Decimal // MonthStart..CurrentEnd
	PreviousAmount decimal.Decimal // PreviousStart..PreviousEnd; zero without a previous month
}

// Bounds of GET /purchases/summary; time.Date would silently normalize values out of range.
const (
	minSummaryYear = 1
	maxSummaryYear = 9999
)
