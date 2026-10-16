package purchase

import (
	"context"

	"github.com/jonasjesusamerico/we-sync-api/internal/category"
)

// mockCategoryRepository is a test double for category.CategoryRepository
// used in purchase service tests. Follows the nil-check pattern.
type mockCategoryRepository struct {
	GetByIDFn            func(ctx context.Context, id int64) (*category.Category, error)
	ListForUserFn        func(ctx context.Context, userID int64) ([]category.Category, error)
	CreateFn             func(ctx context.Context, userID int64, req *category.CreateCategoryRequest) (*category.Category, error)
	UpdateFn             func(ctx context.Context, userID, id int64, req *category.UpdateCategoryRequest) (*category.Category, error)
	DeleteFn             func(ctx context.Context, userID, id int64) (bool, error)
	ExistsGlobalByNameFn func(ctx context.Context, name string) (bool, error)
}

func (m *mockCategoryRepository) GetByID(ctx context.Context, id int64) (*category.Category, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}

func (m *mockCategoryRepository) ListForUser(ctx context.Context, userID int64) ([]category.Category, error) {
	if m.ListForUserFn == nil {
		return []category.Category{}, nil
	}
	return m.ListForUserFn(ctx, userID)
}

func (m *mockCategoryRepository) Create(ctx context.Context, userID int64, req *category.CreateCategoryRequest) (*category.Category, error) {
	if m.CreateFn == nil {
		return nil, nil
	}
	return m.CreateFn(ctx, userID, req)
}

func (m *mockCategoryRepository) Update(ctx context.Context, userID, id int64, req *category.UpdateCategoryRequest) (*category.Category, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, userID, id, req)
}

func (m *mockCategoryRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
	if m.DeleteFn == nil {
		return false, nil
	}
	return m.DeleteFn(ctx, userID, id)
}

func (m *mockCategoryRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	if m.ExistsGlobalByNameFn == nil {
		return false, nil
	}
	return m.ExistsGlobalByNameFn(ctx, name)
}
