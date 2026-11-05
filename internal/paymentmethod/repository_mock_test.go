package paymentmethod

import "context"

// mockPaymentMethodRepository is a test double for Repository.
// Each method falls back gracefully when its Fn field is nil (nil-check pattern).
type mockPaymentMethodRepository struct {
	CreateFn             func(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error)
	GetByIDFn            func(ctx context.Context, id int64) (*PaymentMethod, error)
	ListForUserFn        func(ctx context.Context, userID int64) ([]PaymentMethod, error)
	UpdateFn             func(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error)
	SetArchivedFn        func(ctx context.Context, userID, id int64, archived bool) (bool, error)
	ExistsGlobalByNameFn func(ctx context.Context, name string) (bool, error)
	FindOwnByNameFn      func(ctx context.Context, userID int64, name string) (*PaymentMethod, error)
}

func (m *mockPaymentMethodRepository) Create(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
	if m.CreateFn == nil {
		return nil, false, nil
	}
	return m.CreateFn(ctx, userID, req)
}

func (m *mockPaymentMethodRepository) GetByID(ctx context.Context, id int64) (*PaymentMethod, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}

func (m *mockPaymentMethodRepository) ListForUser(ctx context.Context, userID int64) ([]PaymentMethod, error) {
	if m.ListForUserFn == nil {
		return []PaymentMethod{}, nil
	}
	return m.ListForUserFn(ctx, userID)
}

func (m *mockPaymentMethodRepository) Update(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, userID, id, req)
}

func (m *mockPaymentMethodRepository) SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error) {
	if m.SetArchivedFn == nil {
		return false, nil
	}
	return m.SetArchivedFn(ctx, userID, id, archived)
}

func (m *mockPaymentMethodRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	if m.ExistsGlobalByNameFn == nil {
		return false, nil
	}
	return m.ExistsGlobalByNameFn(ctx, name)
}

func (m *mockPaymentMethodRepository) FindOwnByName(ctx context.Context, userID int64, name string) (*PaymentMethod, error) {
	if m.FindOwnByNameFn == nil {
		return nil, nil
	}
	return m.FindOwnByNameFn(ctx, userID, name)
}
