package purchase

import "context"

// mockPurchaseRepository is a test double for PurchaseRepository.
// Each method can be overridden by setting the corresponding Fn field.
type mockPurchaseRepository struct {
	CreateFn  func(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByIDFn func(ctx context.Context, id int64) (*Purchase, error)
	ListFn    func(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	UpdateFn  func(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	DeleteFn  func(ctx context.Context, id int64) (bool, error)
}

func (m *mockPurchaseRepository) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	return m.CreateFn(ctx, req)
}

func (m *mockPurchaseRepository) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	return m.GetByIDFn(ctx, id)
}

func (m *mockPurchaseRepository) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	return m.ListFn(ctx, filter)
}

func (m *mockPurchaseRepository) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	return m.UpdateFn(ctx, id, req)
}

func (m *mockPurchaseRepository) Delete(ctx context.Context, id int64) (bool, error) {
	return m.DeleteFn(ctx, id)
}
