package paymentmethod

import "context"

// Repository is the outbound port for payment method persistence.
//
// Contracts:
//   - GetByID returns (nil, nil) when the payment method does not exist.
//   - FindOwnByName returns (nil, nil) when the user has no payment method with that name key.
//   - Update returns (nil, nil) when no row with the given ID and userID exists.
//   - SetArchived returns (false, nil) when no row with the given ID and userID exists.
//   - Create and Update return apperrors.ErrConflict on name uniqueness violation.
//
// Names are compared by catalog_name_key (trimmed by the service, then case- and accent-insensitive).
type Repository interface {
	// Create inserts a personal payment method owned by userID, or reactivates the user's archived payment
	// method with the same name key: the stored name is kept and archived_at = NULL is written.
	// inserted is true for a new row and false for a reactivation. It returns (nil, false, nil) when the
	// user already has an active payment method with the same name key.
	Create(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (pm *PaymentMethod, inserted bool, err error)

	// GetByID fetches a payment method by ID regardless of ownership.
	// Used internally by the purchase service to validate accessibility.
	// Callers are responsible for checking ownership.
	GetByID(ctx context.Context, id int64) (*PaymentMethod, error)

	// ListForUser returns all payment methods visible to the user, archived ones included:
	// global payment methods (user_id IS NULL) plus the user's own payment methods.
	// No limit: these are hand-made lists of one person, tens of items (RN-20 of feature 006).
	ListForUser(ctx context.Context, userID int64) ([]PaymentMethod, error)

	// ExistsGlobalByName reports whether a global payment method has the same name key.
	// Used to block personal names that shadow a global.
	ExistsGlobalByName(ctx context.Context, name string) (bool, error)

	// FindOwnByName returns the user's own payment method (active or archived) with the same name key.
	// It reads from the write pool: it runs right after a unique violation.
	FindOwnByName(ctx context.Context, userID int64, name string) (*PaymentMethod, error)

	// Update renames a personal payment method. Scoped to userID to prevent cross-tenant writes. Returns (nil, nil) when the payment method does not exist or belongs to another user.
	Update(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error)

	// SetArchived archives (archived == true) or reactivates a personal payment method. Scoped to userID.
	// Archiving an archived payment method keeps its original archived_at. Returns false when the payment
	// method does not exist or belongs to another user.
	SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error)
}
