package paymentmethod

import (
	"fmt"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

// ErrPaymentMethodNotFound is returned when a payment method does not exist or is not accessible
// by the caller. It wraps apperrors.ErrNotFound so generic not-found handling still applies.
var ErrPaymentMethodNotFound = fmt.Errorf("payment method %w", apperrors.ErrNotFound)

// PaymentMethod represents an instrument used to pay purchase entries (e.g. cash, a credit card).
// Global payment methods have UserID == nil and cannot be modified or deleted by users.
// Personal payment methods belong to a specific user (UserID != nil).
type PaymentMethod struct {
	ID        int64     `json:"id"`
	UserID    *int64    `json:"user_id,omitempty"`
	Name      string    `json:"name"`
	IsGlobal  bool      `json:"is_global"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccessibleBy reports whether the payment method is global or owned by userID.
func (pm *PaymentMethod) AccessibleBy(userID int64) bool {
	return pm.IsGlobal || (pm.UserID != nil && *pm.UserID == userID)
}

// PaymentMethodSummary is the embedded representation of a payment method inside a Purchase response.
type PaymentMethodSummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsGlobal bool   `json:"is_global"`
}

// CreatePaymentMethodRequest is the input type for creating a personal payment method.
type CreatePaymentMethodRequest struct {
	Name string `json:"name"`
}

// UpdatePaymentMethodRequest is the input type for renaming a personal payment method.
type UpdatePaymentMethodRequest struct {
	Name string `json:"name"`
}
