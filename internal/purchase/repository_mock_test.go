package purchase

import "context"

// mockPurchaseRepository is a test double for PurchaseRepository.
// Each method falls back gracefully when its Fn field is nil (nil-check pattern).
type mockPurchaseRepository struct {
	CreateFn  func(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error)
	GetByIDFn func(ctx context.Context, userID, id int64) (*Purchase, error)
	ListFn    func(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	UpdateFn  func(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	DeleteFn  func(ctx context.Context, userID, id int64) (bool, error)
}

func (m *mockPurchaseRepository) Create(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error) {
	if m.CreateFn == nil {
		return nil, nil
	}
	return m.CreateFn(ctx, userID, req)
}

func (m *mockPurchaseRepository) GetByID(ctx context.Context, userID, id int64) (*Purchase, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, userID, id)
}

func (m *mockPurchaseRepository) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	if m.ListFn == nil {
		return &PaginatedResult{Items: []Purchase{}}, nil
	}
	return m.ListFn(ctx, filter)
}

func (m *mockPurchaseRepository) Update(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, userID, id, req)
}

func (m *mockPurchaseRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
	if m.DeleteFn == nil {
		return false, nil
	}
	return m.DeleteFn(ctx, userID, id)
}
