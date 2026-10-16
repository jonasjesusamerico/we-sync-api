package paymentmethod

import "context"

// Repository is the outbound port for payment method persistence.
//
// Contracts:
//   - GetByID returns (nil, nil) when the payment method does not exist.
//   - Update returns (nil, nil) when no row with the given ID and userID exists.
//   - Delete returns (false, nil) when no row with the given ID and userID exists.
//   - Create returns apperrors.ErrConflict on name uniqueness violation.
type Repository interface {
	// Create inserts a new personal payment method owned by userID.
	Create(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, error)

	// GetByID fetches a payment method by ID regardless of ownership.
	// Used internally by the purchase service to validate accessibility.
	// Callers are responsible for checking ownership.
	GetByID(ctx context.Context, id int64) (*PaymentMethod, error)

	// ListForUser returns all payment methods visible to the user:
	// global payment methods (user_id IS NULL) plus the user's own payment methods.
	ListForUser(ctx context.Context, userID int64) ([]PaymentMethod, error)

	// ExistsGlobalByName reports whether a global payment method with the given name
	// exists (case-insensitive). Used to block personal names that shadow a global.
	ExistsGlobalByName(ctx context.Context, name string) (bool, error)

	// Update renames a personal payment method. Scoped to userID to prevent cross-tenant writes.
	// Returns (nil, nil) when the payment method does not exist or belongs to another user.
	Update(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error)

	// Delete removes a personal payment method. Scoped to userID.
	// Returns (false, nil) when the payment method does not exist or belongs to another user.
	// Returns apperrors.ErrConflict when the payment method is referenced by purchase entries.
	Delete(ctx context.Context, userID, id int64) (bool, error)
}
