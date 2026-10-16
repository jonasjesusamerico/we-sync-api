package purchase

import (
	"context"

	"github.com/jonasjesusamerico/we-sync-api/internal/category"
)

// mockCategoryLookup is a test double for categoryLookup used in purchase
// service tests. Follows the nil-check pattern.
type mockCategoryLookup struct {
	GetByIDFn func(ctx context.Context, id int64) (*category.Category, error)
}

func (m *mockCategoryLookup) GetByID(ctx context.Context, id int64) (*category.Category, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}
