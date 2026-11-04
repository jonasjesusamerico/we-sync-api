package purchase

import "context"

// Repository is the outbound port for purchase entry persistence.
// Implementations must use (nil, nil) to signal "not found" on GetByID and Update.
// Delete returns (false, nil) when no row with the given ID exists.
type Repository interface {
	Create(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, userID, id int64) (*Purchase, error)
	// List returns one page of the user's purchases matching filter, newest first, plus
	// Total (count of every status) and TotalAmount (sum without CANCELED) for the whole
	// filter. filter.Search must already be trimmed; it is matched literally with strpos (no
	// wildcards), ignoring case and accents.
	// It reads from the read pool: right after a write it may lag if Read becomes a replica.
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	// Summarize returns the user's purchase sums over ranges in one query. It never
	// returns (nil, nil). Amounts exclude CANCELED; MonthCount counts every status.
	// It reads from the read pool (same lag note as List).
	Summarize(ctx context.Context, userID int64, ranges SummaryRanges) (*SummarySums, error)
	Update(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, userID, id int64) (bool, error)
}
