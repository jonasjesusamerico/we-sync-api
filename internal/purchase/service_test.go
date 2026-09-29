package purchase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/shopspring/decimal"
)

// helpers

func validCreateRequest() *CreatePurchaseRequest {
	return &CreatePurchaseRequest{PurchaseRequest{
		PurchaseDate:  Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:   "Energy bill",
		Sector:        "Utilities",
		PaymentMethod: PaymentMethodPix,
		Amount:        decimal.NewFromFloat(174.65),
		Status:        StatusPending,
	}}
}

func validUpdateRequest() *UpdatePurchaseRequest {
	return &UpdatePurchaseRequest{PurchaseRequest{
		PurchaseDate:  Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:   "Energy bill updated",
		Sector:        "Utilities",
		PaymentMethod: PaymentMethodPix,
		Amount:        decimal.NewFromFloat(200.00),
		Status:        StatusPaid,
	}}
}

func purchaseFromCreate(req *CreatePurchaseRequest) *Purchase {
	return &Purchase{
		ID:            1,
		PurchaseDate:  req.PurchaseDate,
		Description:   req.Description,
		Sector:        req.Sector,
		PaymentMethod: req.PaymentMethod,
		Amount:        req.Amount,
		Status:        req.Status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// --- Create ---

func TestService_Create_Success(t *testing.T) {
	req := validCreateRequest()
	expected := purchaseFromCreate(req)

	svc := NewPurchaseService(&mockPurchaseRepository{
		CreateFn: func(_ context.Context, r *CreatePurchaseRequest) (*Purchase, error) {
			return expected, nil
		},
	})

	got, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != expected.ID {
		t.Errorf("expected ID %d, got %d", expected.ID, got.ID)
	}
}

func TestService_Create_MissingPurchaseDate(t *testing.T) {
	req := validCreateRequest()
	req.PurchaseDate = Date{}

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_MissingDescription(t *testing.T) {
	req := validCreateRequest()
	req.Description = ""

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_MissingSector(t *testing.T) {
	req := validCreateRequest()
	req.Sector = ""

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_InvalidPaymentMethod(t *testing.T) {
	req := validCreateRequest()
	req.PaymentMethod = "BITCOIN"

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_AmountZero(t *testing.T) {
	req := validCreateRequest()
	req.Amount = decimal.Zero

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_AmountNegative(t *testing.T) {
	req := validCreateRequest()
	req.Amount = decimal.NewFromFloat(-10)

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_InvalidStatus(t *testing.T) {
	req := validCreateRequest()
	req.Status = "UNKNOWN"

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db error")

	svc := NewPurchaseService(&mockPurchaseRepository{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, repoErr
		},
	})

	_, err := svc.Create(context.Background(), validCreateRequest())
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// --- GetByID ---

func TestService_GetByID_Found(t *testing.T) {
	want := &Purchase{ID: 42, Description: "Test"}

	svc := NewPurchaseService(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Purchase, error) {
			return want, nil
		},
	})

	got, err := svc.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := NewPurchaseService(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*Purchase, error) {
			return nil, nil
		},
	})

	_, err := svc.GetByID(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_GetByID_RepositoryError(t *testing.T) {
	repoErr := errors.New("connection failed")

	svc := NewPurchaseService(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*Purchase, error) {
			return nil, repoErr
		},
	})

	_, err := svc.GetByID(context.Background(), 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// --- List ---

func TestService_List_DefaultPagination(t *testing.T) {
	var capturedFilter ListFilter

	svc := NewPurchaseService(&mockPurchaseRepository{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: f.Page, PageSize: f.PageSize}, nil
		},
	})

	// Pass zero values — service must apply defaults.
	_, err := svc.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter.Page != 1 {
		t.Errorf("expected default Page=1, got %d", capturedFilter.Page)
	}
	if capturedFilter.PageSize != 20 {
		t.Errorf("expected default PageSize=20, got %d", capturedFilter.PageSize)
	}
}

func TestService_List_CustomPagination(t *testing.T) {
	var capturedFilter ListFilter

	svc := NewPurchaseService(&mockPurchaseRepository{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: f.Page, PageSize: f.PageSize}, nil
		},
	})

	_, err := svc.List(context.Background(), ListFilter{Page: 3, PageSize: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter.Page != 3 {
		t.Errorf("expected Page=3, got %d", capturedFilter.Page)
	}
	if capturedFilter.PageSize != 5 {
		t.Errorf("expected PageSize=5, got %d", capturedFilter.PageSize)
	}
}

func TestService_List_PageSizeExceedsMax(t *testing.T) {
	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.List(context.Background(), ListFilter{PageSize: MaxPageSize + 1})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for page_size > MaxPageSize, got %v", err)
	}
}

// --- Update ---

func TestService_Update_Success(t *testing.T) {
	req := validUpdateRequest()
	want := &Purchase{ID: 1, Description: req.Description}

	svc := NewPurchaseService(&mockPurchaseRepository{
		UpdateFn: func(_ context.Context, id int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return want, nil
		},
	})

	got, err := svc.Update(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := NewPurchaseService(&mockPurchaseRepository{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, nil
		},
	})

	_, err := svc.Update(context.Background(), 99, validUpdateRequest())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Update_ValidationError(t *testing.T) {
	req := validUpdateRequest()
	req.Amount = decimal.NewFromFloat(-5)

	svc := NewPurchaseService(&mockPurchaseRepository{})

	_, err := svc.Update(context.Background(), 1, req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// --- Delete ---

func TestService_Delete_Success(t *testing.T) {
	svc := NewPurchaseService(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64) (bool, error) {
			return true, nil
		},
	})

	if err := svc.Delete(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := NewPurchaseService(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64) (bool, error) {
			return false, nil
		},
	})

	err := svc.Delete(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_RepositoryError(t *testing.T) {
	repoErr := errors.New("delete failed")

	svc := NewPurchaseService(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64) (bool, error) {
			return false, repoErr
		},
	})

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// --- Domain helpers ---

func TestIsValidStatus(t *testing.T) {
	valid := []Status{StatusPending, StatusPaid, StatusCanceled}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []Status{"", "UNKNOWN", "pending", "paid"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestIsValidPaymentMethod(t *testing.T) {
	valid := []PaymentMethod{
		PaymentMethodCash, PaymentMethodCreditCard, PaymentMethodDebitCard,
		PaymentMethodPix, PaymentMethodBankTransfer, PaymentMethodBoleto,
	}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("expected %q to be valid", p)
		}
	}

	invalid := []PaymentMethod{"", "BITCOIN", "cash", "pix"}
	for _, p := range invalid {
		if p.IsValid() {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}
