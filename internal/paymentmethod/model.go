package paymentmethod

import (
	"fmt"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

// ErrPaymentMethodNotFound is returned when a payment method does not exist or is not accessible
// by the caller. It wraps apperrors.ErrNotFound so generic not-found handling still applies.
var ErrPaymentMethodNotFound = fmt.Errorf("payment method %w", apperrors.ErrNotFound)

// Name conflicts (RN-13 of feature 006). They wrap apperrors.ErrConflict, so generic handling still applies;
// the handler maps each one to its own code.
var (
	ErrDefaultNameConflict  = fmt.Errorf("name of a default payment method: %w", apperrors.ErrConflict)
	ErrNameConflict         = fmt.Errorf("name of another payment method: %w", apperrors.ErrConflict)
	ErrArchivedNameConflict = fmt.Errorf("name of an archived payment method: %w", apperrors.ErrConflict)
)

// PaymentMethod represents an instrument used to pay purchase entries (e.g. cash, a credit card).
// Global payment methods have UserID == nil, are never archived and cannot be changed by users.
// Personal payment methods belong to a specific user (UserID != nil).
type PaymentMethod struct {
	ID         int64      `json:"id"`
	UserID     *int64     `json:"user_id,omitempty"`
	Name       string     `json:"name"`
	IsGlobal   bool       `json:"is_global"`
	ArchivedAt *time.Time `json:"archived_at"` // null = active
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AccessibleBy reports whether the payment method is global or owned by userID.
func (pm *PaymentMethod) AccessibleBy(userID int64) bool {
	return pm.IsGlobal || (pm.UserID != nil && *pm.UserID == userID)
}

// IsArchived reports whether the payment method is archived.
func (pm *PaymentMethod) IsArchived() bool { return pm.ArchivedAt != nil }

// PaymentMethodSummary is the embedded representation of a payment method inside a Purchase response.
type PaymentMethodSummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsGlobal bool   `json:"is_global"`
}

// CreatePaymentMethodRequest is the input of POST /payment-methods. Other fields sent (such as icon and
// color) are ignored by the decoder.
type CreatePaymentMethodRequest struct {
	Name string `json:"name"`
}

// UpdatePaymentMethodRequest is the input of PUT /payment-methods/{id}: it renames the payment method.
type UpdatePaymentMethodRequest struct {
	Name string `json:"name"`
}
