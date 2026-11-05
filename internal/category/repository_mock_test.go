package category

import "context"

// mockCategoryRepository is a test double for Repository.
// Each method falls back gracefully when its Fn field is nil (nil-check pattern).
type mockCategoryRepository struct {
	CreateFn             func(ctx context.Context, userID int64, req *CreateCategoryRequest) (*Category, bool, error)
	GetByIDFn            func(ctx context.Context, id int64) (*Category, error)
	ListForUserFn        func(ctx context.Context, userID int64) ([]Category, error)
	UpdateFn             func(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error)
	SetArchivedFn        func(ctx context.Context, userID, id int64, archived bool) (bool, error)
	ExistsGlobalByNameFn func(ctx context.Context, name string) (bool, error)
	FindOwnByNameFn      func(ctx context.Context, userID int64, name string) (*Category, error)
}

func (m *mockCategoryRepository) Create(ctx context.Context, userID int64, req *CreateCategoryRequest) (*Category, bool, error) {
	if m.CreateFn == nil {
		return nil, false, nil
	}
	return m.CreateFn(ctx, userID, req)
}

func (m *mockCategoryRepository) GetByID(ctx context.Context, id int64) (*Category, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}

func (m *mockCategoryRepository) ListForUser(ctx context.Context, userID int64) ([]Category, error) {
	if m.ListForUserFn == nil {
		return []Category{}, nil
	}
	return m.ListForUserFn(ctx, userID)
}

func (m *mockCategoryRepository) Update(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, userID, id, req)
}

func (m *mockCategoryRepository) SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error) {
	if m.SetArchivedFn == nil {
		return false, nil
	}
	return m.SetArchivedFn(ctx, userID, id, archived)
}

func (m *mockCategoryRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	if m.ExistsGlobalByNameFn == nil {
		return false, nil
	}
	return m.ExistsGlobalByNameFn(ctx, name)
}

func (m *mockCategoryRepository) FindOwnByName(ctx context.Context, userID int64, name string) (*Category, error) {
	if m.FindOwnByNameFn == nil {
		return nil, nil
	}
	return m.FindOwnByNameFn(ctx, userID, name)
}
