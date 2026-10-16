package category

import "context"

// Repository is the outbound port for category persistence.
//
// Contracts:
//   - GetByID returns (nil, nil) when the category does not exist.
//   - Update returns (nil, nil) when no row with the given ID and userID exists.
//   - Delete returns (false, nil) when no row with the given ID and userID exists.
//   - Create returns apperrors.ErrConflict on name uniqueness violation.
type Repository interface {
	// Create inserts a new personal category owned by userID.
	Create(ctx context.Context, userID int64, req *CreateCategoryRequest) (*Category, error)

	// GetByID fetches a category by ID regardless of ownership.
	// Used internally by the purchase service to validate accessibility.
	// Callers are responsible for checking ownership.
	GetByID(ctx context.Context, id int64) (*Category, error)

	// ListForUser returns all categories visible to the user:
	// global categories (user_id IS NULL) plus the user's own categories.
	ListForUser(ctx context.Context, userID int64) ([]Category, error)

	// ExistsGlobalByName reports whether a global category with the given name
	// exists (case-insensitive). Used to block personal names that shadow a global.
	ExistsGlobalByName(ctx context.Context, name string) (bool, error)

	// Update renames a personal category. Scoped to userID to prevent cross-tenant writes.
	// Returns (nil, nil) when the category does not exist or belongs to another user.
	Update(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error)

	// Delete removes a personal category. Scoped to userID.
	// Returns (false, nil) when the category does not exist or belongs to another user.
	// Returns apperrors.ErrConflict when the category is referenced by purchase entries.
	Delete(ctx context.Context, userID, id int64) (bool, error)
}
