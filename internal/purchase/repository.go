package purchase

import "context"

// PurchaseRepository is the outbound port for purchase entry persistence.
// Implementations must use (nil, nil) to signal "not found" on GetByID.
// Delete returns (false, nil) when no row with the given ID exists.
type PurchaseRepository interface {
	Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, id int64) (*Purchase, error)
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, id int64) (bool, error)
}
