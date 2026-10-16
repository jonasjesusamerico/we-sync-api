package purchase

import (
	"context"

	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
)

// mockPaymentMethodLookup is a test double for paymentMethodLookup used in purchase
// service tests. Follows the nil-check pattern.
type mockPaymentMethodLookup struct {
	GetByIDFn func(ctx context.Context, id int64) (*paymentmethod.PaymentMethod, error)
}

func (m *mockPaymentMethodLookup) GetByID(ctx context.Context, id int64) (*paymentmethod.PaymentMethod, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}
