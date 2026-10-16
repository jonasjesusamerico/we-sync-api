package purchase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func globalCatRepo() *mockCategoryLookup {
	return &mockCategoryLookup{
		GetByIDFn: func(_ context.Context, id int64) (*category.Category, error) {
			return &category.Category{ID: id, UserID: nil, Name: "Alimentação", IsGlobal: true}, nil
		},
	}
}

func validCreateRequest() *CreatePurchaseRequest {
	return &CreatePurchaseRequest{PurchaseRequest{
		PurchaseDate:  Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:   "Energy bill",
		CategoryID:    1,
		PaymentMethod: PaymentMethodPix,
		Amount:        decimal.NewFromFloat(174.65),
		Status:        StatusPending,
	}}
}

func validUpdateRequest() *UpdatePurchaseRequest {
	return &UpdatePurchaseRequest{PurchaseRequest{
		PurchaseDate:  Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:   "Energy bill updated",
		CategoryID:    1,
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
		Category:      category.CategorySummary{ID: req.CategoryID, Name: "Alimentação", IsGlobal: true},
		PaymentMethod: req.PaymentMethod,
		Amount:        req.Amount,
		Status:        req.Status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func newSvc(repo Repository, catRepo *mockCategoryLookup) *Service {
	return NewService(repo, catRepo)
}

// ---------------------------------------------------------------------------
// Unauthorized — operations require user ID in context
// ---------------------------------------------------------------------------

func TestService_OperationsRequireUserID(t *testing.T) {
	called := false
	repo := &mockPurchaseRepository{
		CreateFn: func(context.Context, int64, *CreatePurchaseRequest) (*Purchase, error) {
			called = true
			return nil, nil
		},
		GetByIDFn: func(context.Context, int64, int64) (*Purchase, error) {
			called = true
			return nil, nil
		},
		ListFn: func(context.Context, ListFilter) (*PaginatedResult, error) {
			called = true
			return nil, nil
		},
		UpdateFn: func(context.Context, int64, int64, *UpdatePurchaseRequest) (*Purchase, error) {
			called = true
			return nil, nil
		},
		DeleteFn: func(context.Context, int64, int64) (bool, error) {
			called = true
			return false, nil
		},
	}
	svc := newSvc(repo, globalCatRepo())
	if _, err := svc.Create(context.Background(), validCreateRequest()); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("Create() error = %v, want unauthorized", err)
	}
	if _, err := svc.GetByID(context.Background(), 1); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("GetByID() error = %v, want unauthorized", err)
	}
	if _, err := svc.List(context.Background(), ListFilter{}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("List() error = %v, want unauthorized", err)
	}
	if _, err := svc.Update(context.Background(), 1, validUpdateRequest()); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("Update() error = %v, want unauthorized", err)
	}
	if err := svc.Delete(context.Background(), 1); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("Delete() error = %v, want unauthorized", err)
	}
	if called {
		t.Fatal("repository was called without user ID")
	}
}

func TestService_UserIDAlwaysComesFromContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.Int64Range(1, 1_000_000).Draw(t, "userID")
		captured := int64(0)
		repo := &mockPurchaseRepository{
			ListFn: func(_ context.Context, filter ListFilter) (*PaginatedResult, error) {
				captured = filter.UserID
				return &PaginatedResult{Items: []Purchase{}}, nil
			},
		}
		svc := newSvc(repo, globalCatRepo())
		_, err := svc.List(identity.WithUserID(context.Background(), userID), ListFilter{UserID: userID + 1})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if captured != userID {
			t.Fatalf("repository received user ID %d, want %d", captured, userID)
		}
	})
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestService_Create_Success(t *testing.T) {
	req := validCreateRequest()
	expected := purchaseFromCreate(req)

	svc := newSvc(&mockPurchaseRepository{
		CreateFn: func(_ context.Context, _ int64, r *CreatePurchaseRequest) (*Purchase, error) {
			return expected, nil
		},
	}, globalCatRepo())

	got, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
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
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_MissingDescription(t *testing.T) {
	req := validCreateRequest()
	req.Description = ""
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_MissingCategoryID(t *testing.T) {
	req := validCreateRequest()
	req.CategoryID = 0
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for zero category_id, got %v", err)
	}
}

func TestService_Create_CategoryNotFound(t *testing.T) {
	req := validCreateRequest()
	catRepo := &mockCategoryLookup{
		GetByIDFn: func(_ context.Context, _ int64) (*category.Category, error) { return nil, nil },
	}
	svc := newSvc(&mockPurchaseRepository{}, catRepo)
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, category.ErrCategoryNotFound) {
		t.Errorf("expected ErrNotFound for missing category, got %v", err)
	}
}

func TestService_Create_CategoryBelongsToAnotherUser(t *testing.T) {
	req := validCreateRequest()
	otherUser := int64(99)
	catRepo := &mockCategoryLookup{
		GetByIDFn: func(_ context.Context, id int64) (*category.Category, error) {
			return &category.Category{ID: id, UserID: &otherUser, IsGlobal: false}, nil
		},
	}
	svc := newSvc(&mockPurchaseRepository{}, catRepo)
	// user 1 tries to use category owned by user 99
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, category.ErrCategoryNotFound) {
		t.Errorf("expected ErrNotFound (no info leak), got %v", err)
	}
}

func TestService_Create_InvalidPaymentMethod(t *testing.T) {
	req := validCreateRequest()
	req.PaymentMethod = "BITCOIN"
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_AmountZero(t *testing.T) {
	req := validCreateRequest()
	req.Amount = decimal.Zero
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_AmountNegative(t *testing.T) {
	req := validCreateRequest()
	req.Amount = decimal.NewFromFloat(-10)
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_InvalidStatus(t *testing.T) {
	req := validCreateRequest()
	req.Status = "UNKNOWN"
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db error")
	svc := newSvc(&mockPurchaseRepository{
		CreateFn: func(_ context.Context, _ int64, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, repoErr
		},
	}, globalCatRepo())
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), validCreateRequest())
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetByID
// ---------------------------------------------------------------------------

func TestService_GetByID_Found(t *testing.T) {
	want := &Purchase{ID: 42, Description: "Test"}
	svc := newSvc(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, _ int64, id int64) (*Purchase, error) { return want, nil },
	}, globalCatRepo())
	got, err := svc.GetByID(identity.WithUserID(context.Background(), 1), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := newSvc(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, _ int64, _ int64) (*Purchase, error) { return nil, nil },
	}, globalCatRepo())
	_, err := svc.GetByID(identity.WithUserID(context.Background(), 1), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_GetByID_RepositoryError(t *testing.T) {
	repoErr := errors.New("connection failed")
	svc := newSvc(&mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, _ int64, _ int64) (*Purchase, error) { return nil, repoErr },
	}, globalCatRepo())
	_, err := svc.GetByID(identity.WithUserID(context.Background(), 1), 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestService_List_DefaultPagination(t *testing.T) {
	var capturedFilter ListFilter
	svc := newSvc(&mockPurchaseRepository{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: f.Page, PageSize: f.PageSize}, nil
		},
	}, globalCatRepo())
	_, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{})
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
	svc := newSvc(&mockPurchaseRepository{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: f.Page, PageSize: f.PageSize}, nil
		},
	}, globalCatRepo())
	_, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{Page: 3, PageSize: 5})
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
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{PageSize: MaxPageSize + 1})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for page_size > MaxPageSize, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestService_Update_Success(t *testing.T) {
	req := validUpdateRequest()
	want := &Purchase{ID: 1, Description: req.Description}
	svc := newSvc(&mockPurchaseRepository{
		UpdateFn: func(_ context.Context, _ int64, id int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return want, nil
		},
	}, globalCatRepo())
	got, err := svc.Update(identity.WithUserID(context.Background(), 1), 1, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := newSvc(&mockPurchaseRepository{
		UpdateFn: func(_ context.Context, _ int64, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, nil
		},
	}, globalCatRepo())
	_, err := svc.Update(identity.WithUserID(context.Background(), 1), 99, validUpdateRequest())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Update_ValidationError(t *testing.T) {
	req := validUpdateRequest()
	req.Amount = decimal.NewFromFloat(-5)
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.Update(identity.WithUserID(context.Background(), 1), 1, req)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_CategoryNotFound(t *testing.T) {
	catRepo := &mockCategoryLookup{
		GetByIDFn: func(_ context.Context, _ int64) (*category.Category, error) { return nil, nil },
	}
	svc := newSvc(&mockPurchaseRepository{}, catRepo)
	_, err := svc.Update(identity.WithUserID(context.Background(), 1), 1, validUpdateRequest())
	if !errors.Is(err, category.ErrCategoryNotFound) {
		t.Errorf("expected ErrNotFound for missing category, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestService_Delete_Success(t *testing.T) {
	svc := newSvc(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64, _ int64) (bool, error) { return true, nil },
	}, globalCatRepo())
	if err := svc.Delete(identity.WithUserID(context.Background(), 1), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := newSvc(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil },
	}, globalCatRepo())
	err := svc.Delete(identity.WithUserID(context.Background(), 1), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_RepositoryError(t *testing.T) {
	repoErr := errors.New("delete failed")
	svc := newSvc(&mockPurchaseRepository{
		DeleteFn: func(_ context.Context, _ int64, _ int64) (bool, error) { return false, repoErr },
	}, globalCatRepo())
	err := svc.Delete(identity.WithUserID(context.Background(), 1), 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Domain helpers
// ---------------------------------------------------------------------------

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
