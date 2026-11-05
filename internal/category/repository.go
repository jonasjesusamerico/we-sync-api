package category

import "context"

// Repository is the outbound port for category persistence.
//
// Contracts:
//   - GetByID returns (nil, nil) when the category does not exist.
//   - FindOwnByName returns (nil, nil) when the user has no category with that name key.
//   - Update returns (nil, nil) when no row with the given ID and userID exists.
//   - SetArchived returns (false, nil) when no row with the given ID and userID exists.
//   - Create and Update return apperrors.ErrConflict on name uniqueness violation.
//
// Names are compared by catalog_name_key (trimmed by the service, then case- and accent-insensitive).
type Repository interface {
	// Create inserts a personal category owned by userID, or reactivates the user's archived category
	// with the same name key: the stored name is kept and icon, color and archived_at = NULL are written.
	// inserted is true for a new row and false for a reactivation. It returns (nil, false, nil) when the
	// user already has an active category with the same name key.
	Create(ctx context.Context, userID int64, req *CreateCategoryRequest) (c *Category, inserted bool, err error)

	// GetByID fetches a category by ID regardless of ownership.
	// Used internally by the purchase service to validate accessibility.
	// Callers are responsible for checking ownership.
	GetByID(ctx context.Context, id int64) (*Category, error)

	// ListForUser returns all categories visible to the user, archived ones included:
	// global categories (user_id IS NULL) plus the user's own categories.
	// No limit: these are hand-made lists of one person, tens of items (RN-20 of feature 006).
	ListForUser(ctx context.Context, userID int64) ([]Category, error)

	// ExistsGlobalByName reports whether a global category has the same name key.
	// Used to block personal names that shadow a global.
	ExistsGlobalByName(ctx context.Context, name string) (bool, error)

	// FindOwnByName returns the user's own category (active or archived) with the same name key.
	// It reads from the write pool: it runs right after a unique violation.
	FindOwnByName(ctx context.Context, userID int64, name string) (*Category, error)

	// Update replaces the name, icon and color of a personal category. Scoped to userID to prevent
	// cross-tenant writes. Returns (nil, nil) when the category does not exist or belongs to another user.
	Update(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error)

	// SetArchived archives (archived == true) or reactivates a personal category. Scoped to userID.
	// Archiving an archived category keeps its original archived_at. Returns false when the category
	// does not exist or belongs to another user.
	SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error)
}
