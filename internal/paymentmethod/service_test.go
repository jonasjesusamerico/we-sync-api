package paymentmethod

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

func ptr(id int64) *int64 { return &id }

func globalPaymentMethod(id int64, name string) *PaymentMethod {
	return &PaymentMethod{ID: id, UserID: nil, Name: name, IsGlobal: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func personalPaymentMethod(id, userID int64, name string) *PaymentMethod {
	return &PaymentMethod{ID: id, UserID: ptr(userID), Name: name, IsGlobal: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func ctxWithUser(userID int64) context.Context {
	return identity.WithUserID(context.Background(), userID)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestService_Create_Success(t *testing.T) {
	want := personalPaymentMethod(1, 42, "Nubank")
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(_ context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, error) {
			return want, nil
		},
	})

	got, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: "Nubank"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_Create_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Create(context.Background(), &CreatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Create(ctxWithUser(1), &CreatePaymentMethodRequest{Name: "   "})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_NilRequest(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Create(ctxWithUser(1), nil)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_CollidesWithGlobal(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		ExistsGlobalByNameFn: func(_ context.Context, name string) (bool, error) {
			return strings.EqualFold(name, "Pix"), nil
		},
	})
	_, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: "pix"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for global name collision, got %v", err)
	}
}

func TestService_Create_ConflictFromRepository(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(_ context.Context, _ int64, _ *CreatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Create(ctxWithUser(1), &CreatePaymentMethodRequest{Name: "Duplicada"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(_ context.Context, _ int64, _ *CreatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, repoErr
		},
	})
	_, err := svc.Create(ctxWithUser(1), &CreatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestService_List_Success(t *testing.T) {
	expected := []PaymentMethod{
		*globalPaymentMethod(1, "Pix"),
		*personalPaymentMethod(7, 42, "Nubank"),
	}
	svc := NewService(&mockPaymentMethodRepository{
		ListForUserFn: func(_ context.Context, userID int64) ([]PaymentMethod, error) {
			if userID != 42 {
				t.Errorf("expected userID 42, got %d", userID)
			}
			return expected, nil
		},
	})

	got, err := svc.List(ctxWithUser(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 items, got %d", len(got))
	}
}

func TestService_List_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.List(context.Background())
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_List_RepositoryError(t *testing.T) {
	repoErr := errors.New("connection lost")
	svc := NewService(&mockPaymentMethodRepository{
		ListForUserFn: func(_ context.Context, _ int64) ([]PaymentMethod, error) {
			return nil, repoErr
		},
	})
	_, err := svc.List(ctxWithUser(1))
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestService_Update_Success(t *testing.T) {
	original := personalPaymentMethod(5, 42, "Antigo")
	updated := personalPaymentMethod(5, 42, "Novo")
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return original, nil
		},
		UpdateFn: func(_ context.Context, _, _ int64, _ *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			return updated, nil
		},
	})

	got, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: "Novo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Novo" {
		t.Errorf("expected name Novo, got %s", got.Name)
	}
}

func TestService_Update_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Update(context.Background(), 1, &UpdatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Update_EmptyName(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Update(ctxWithUser(1), 1, &UpdatePaymentMethodRequest{Name: ""})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*PaymentMethod, error) { return nil, nil },
	})
	_, err := svc.Update(ctxWithUser(1), 99, &UpdatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Update_GlobalPaymentMethodForbidden(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return globalPaymentMethod(id, "Pix"), nil
		},
	})
	_, err := svc.Update(ctxWithUser(1), 1, &UpdatePaymentMethodRequest{Name: "Comida"})
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestService_Update_OtherUserPaymentMethod(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 99, "Alheia"), nil // belongs to user 99
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: "X"}) // user 42 tries to update
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound (no info leak), got %v", err)
	}
}

func TestService_Update_CollidesWithGlobal(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 42, "Minha"), nil
		},
		ExistsGlobalByNameFn: func(_ context.Context, name string) (bool, error) {
			return strings.EqualFold(name, "Pix"), nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: "PIX"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for global name collision, got %v", err)
	}
}

func TestService_Update_ConflictFromRepository(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 42, "Original"), nil
		},
		UpdateFn: func(_ context.Context, _, _ int64, _ *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: "Duplicada"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestService_Delete_Success(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 42, "Nubank"), nil
		},
		DeleteFn: func(_ context.Context, _, _ int64) (bool, error) { return true, nil },
	})
	if err := svc.Delete(ctxWithUser(42), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Delete_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	if err := svc.Delete(context.Background(), 1); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*PaymentMethod, error) { return nil, nil },
	})
	if err := svc.Delete(ctxWithUser(1), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_GlobalPaymentMethodForbidden(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return globalPaymentMethod(id, "Boleto"), nil
		},
	})
	if err := svc.Delete(ctxWithUser(1), 3); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestService_Delete_OtherUserPaymentMethod(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 99, "Alheia"), nil
		},
	})
	if err := svc.Delete(ctxWithUser(42), 5); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound (no info leak), got %v", err)
	}
}

func TestService_Delete_ConflictInUse(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(_ context.Context, id int64) (*PaymentMethod, error) {
			return personalPaymentMethod(id, 42, "Em uso"), nil
		},
		DeleteFn: func(_ context.Context, _, _ int64) (bool, error) {
			return false, apperrors.ErrConflict
		},
	})
	if err := svc.Delete(ctxWithUser(42), 5); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict (payment method in use), got %v", err)
	}
}
