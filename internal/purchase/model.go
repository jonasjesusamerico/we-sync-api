package purchase

import "time"

// Status constants
const (
	StatusPending  = "PENDING"
	StatusPaid     = "PAID"
	StatusCanceled = "CANCELED"
)

// Payment method constants
const (
	PaymentMethodCash         = "CASH"
	PaymentMethodCreditCard   = "CREDIT_CARD"
	PaymentMethodDebitCard    = "DEBIT_CARD"
	PaymentMethodPix          = "PIX"
	PaymentMethodBankTransfer = "BANK_TRANSFER"
	PaymentMethodBoleto       = "BOLETO"
)

// IsValidStatus returns true if the given status is one of the allowed values.
func IsValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusPaid, StatusCanceled:
		return true
	}
	return false
}

// IsValidPaymentMethod returns true if the given payment method is one of the allowed values.
func IsValidPaymentMethod(s string) bool {
	switch s {
	case PaymentMethodCash, PaymentMethodCreditCard, PaymentMethodDebitCard,
		PaymentMethodPix, PaymentMethodBankTransfer, PaymentMethodBoleto:
		return true
	}
	return false
}

// Purchase represents a purchase entry record.
type Purchase struct {
	ID            int64     `json:"id"`
	PurchaseDate  time.Time `json:"purchase_date"`
	Description   string    `json:"description"`
	Sector        string    `json:"sector"`
	PaymentMethod string    `json:"payment_method"`
	Amount        float64   `json:"amount"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PurchaseRequest holds the input data for creating or updating a purchase entry.
type PurchaseRequest struct {
	PurchaseDate  time.Time `json:"purchase_date"`
	Description   string    `json:"description"`
	Sector        string    `json:"sector"`
	PaymentMethod string    `json:"payment_method"`
	Amount        float64   `json:"amount"`
	Status        string    `json:"status"`
}

// CreatePurchaseRequest is the input type for the Create operation.
type CreatePurchaseRequest = PurchaseRequest

// UpdatePurchaseRequest is the input type for the Update operation.
type UpdatePurchaseRequest = PurchaseRequest

// ListFilter holds the query parameters for filtering and paginating purchase entries.
type ListFilter struct {
	Month         int
	Year          int
	Status        string
	Sector        string
	PaymentMethod string
	Page          int
	PageSize      int
}

// PaginatedResult wraps a page of purchase entries with pagination metadata.
type PaginatedResult struct {
	Items      []Purchase `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}
