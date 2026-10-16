package purchase

import "context"

// Repository is the outbound port for purchase entry persistence.
// Implementations must use (nil, nil) to signal "not found" on GetByID and Update.
// Delete returns (false, nil) when no row with the given ID exists.
type Repository interface {
	Create(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, userID, id int64) (*Purchase, error)
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	Update(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, userID, id int64) (bool, error)
}
