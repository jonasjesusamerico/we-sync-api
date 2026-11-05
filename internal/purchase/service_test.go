package purchase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
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

func globalPaymentMethodLookup() *mockPaymentMethodLookup {
	return &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, id int64) (*paymentmethod.PaymentMethod, error) {
			return &paymentmethod.PaymentMethod{ID: id, UserID: nil, Name: "Pix", IsGlobal: true}, nil
		},
	}
}

func validCreateRequest() *CreatePurchaseRequest {
	return &CreatePurchaseRequest{PurchaseRequest{
		PurchaseDate:    Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:     "Energy bill",
		CategoryID:      1,
		PaymentMethodID: 1,
		Amount:          decimal.NewFromFloat(174.65),
		Status:          StatusPending,
	}}
}

func validUpdateRequest() *UpdatePurchaseRequest {
	return &UpdatePurchaseRequest{PurchaseRequest{
		PurchaseDate:    Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:     "Energy bill updated",
		CategoryID:      1,
		PaymentMethodID: 1,
		Amount:          decimal.NewFromFloat(200.00),
		Status:          StatusPaid,
	}}
}

func purchaseFromCreate(req *CreatePurchaseRequest) *Purchase {
	return &Purchase{
		ID:            1,
		PurchaseDate:  req.PurchaseDate,
		Description:   req.Description,
		Category:      category.CategorySummary{ID: req.CategoryID, Name: "Alimentação", IsGlobal: true},
		PaymentMethod: paymentmethod.PaymentMethodSummary{ID: req.PaymentMethodID, Name: "Pix", IsGlobal: true},
		Amount:        req.Amount,
		Status:        req.Status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// newSvc builds a Service whose payment method lookup always resolves to a global entry.
func newSvc(repo Repository, catRepo *mockCategoryLookup) *Service {
	return NewService(repo, catRepo, globalPaymentMethodLookup())
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
		SummarizeFn: func(context.Context, int64, SummaryRanges) (*SummarySums, error) {
			called = true
			return &SummarySums{}, nil
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
	if _, err := svc.Summary(context.Background(), 2026, 11, day(2026, 11, 10)); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("Summary() error = %v, want unauthorized", err)
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
		search := rapid.StringN(-1, maxSearchLength, -1).Draw(t, "search")
		_, err := svc.List(identity.WithUserID(context.Background(), userID), ListFilter{UserID: userID + 1, Search: search})
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

func TestService_Create_InvalidPaymentMethodID(t *testing.T) {
	for _, id := range []int64{0, -1} {
		req := validCreateRequest()
		req.PaymentMethodID = id
		svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
		_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)
		if !errors.Is(err, apperrors.ErrValidation) {
			t.Errorf("payment_method_id=%d: expected ErrValidation, got %v", id, err)
		}
	}
}

func TestService_Create_PaymentMethodNotFound(t *testing.T) {
	pmRepo := &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, _ int64) (*paymentmethod.PaymentMethod, error) { return nil, nil },
	}
	svc := NewService(&mockPurchaseRepository{}, globalCatRepo(), pmRepo)
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), validCreateRequest())
	if !errors.Is(err, paymentmethod.ErrPaymentMethodNotFound) {
		t.Errorf("expected ErrPaymentMethodNotFound, got %v", err)
	}
}

func TestService_Create_PaymentMethodBelongsToAnotherUser(t *testing.T) {
	otherUser := int64(99)
	pmRepo := &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, id int64) (*paymentmethod.PaymentMethod, error) {
			return &paymentmethod.PaymentMethod{ID: id, UserID: &otherUser, IsGlobal: false}, nil
		},
	}
	called := false
	repo := &mockPurchaseRepository{
		CreateFn: func(context.Context, int64, *CreatePurchaseRequest) (*Purchase, error) {
			called = true
			return nil, nil
		},
	}
	svc := NewService(repo, globalCatRepo(), pmRepo)
	// user 1 tries to use a payment method owned by user 99
	_, err := svc.Create(identity.WithUserID(context.Background(), 1), validCreateRequest())
	if !errors.Is(err, paymentmethod.ErrPaymentMethodNotFound) {
		t.Errorf("expected ErrPaymentMethodNotFound (no info leak), got %v", err)
	}
	if called {
		t.Error("repository must not be called when the payment method is not accessible")
	}
}

func TestService_Create_OwnPersonalPaymentMethod(t *testing.T) {
	owner := int64(1)
	pmRepo := &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, id int64) (*paymentmethod.PaymentMethod, error) {
			return &paymentmethod.PaymentMethod{ID: id, UserID: &owner, Name: "Nubank", IsGlobal: false}, nil
		},
	}
	repo := &mockPurchaseRepository{
		CreateFn: func(_ context.Context, _ int64, req *CreatePurchaseRequest) (*Purchase, error) {
			return purchaseFromCreate(req), nil
		},
	}
	svc := NewService(repo, globalCatRepo(), pmRepo)
	if _, err := svc.Create(identity.WithUserID(context.Background(), owner), validCreateRequest()); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestService_List_Search(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trimmed", "  mercado  ", "mercado"},
		{"only spaces means no search", "   ", ""},
		{"empty", "", ""},
		{"255 characters (runes)", strings.Repeat("ç", 255), strings.Repeat("ç", 255)},
		{"255 characters after trim", "  " + strings.Repeat("ç", 255) + "  ", strings.Repeat("ç", 255)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured ListFilter
			svc := newSvc(&mockPurchaseRepository{
				ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
					captured = f
					return &PaginatedResult{Items: []Purchase{}}, nil
				},
			}, globalCatRepo())
			if _, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{Search: tt.input}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.Search != tt.want {
				t.Errorf("repository received Search %q, want %q", captured.Search, tt.want)
			}
		})
	}
}

func TestService_List_SearchTooLong(t *testing.T) {
	for _, search := range []string{strings.Repeat("ç", 256), strings.Repeat("a", 256)} {
		called := false
		svc := newSvc(&mockPurchaseRepository{
			ListFn: func(context.Context, ListFilter) (*PaginatedResult, error) {
				called = true
				return &PaginatedResult{Items: []Purchase{}}, nil
			},
		}, globalCatRepo())
		_, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{Search: search})
		if !errors.Is(err, apperrors.ErrValidation) {
			t.Fatalf("len %d bytes: error = %v, want ErrValidation", len(search), err)
		}
		if err.Error() != "q must be at most 255 characters" {
			t.Errorf("message = %q", err.Error())
		}
		if called {
			t.Error("repository must not be called")
		}
	}
}

func TestService_List_PageSizeCheckedBeforeSearch(t *testing.T) {
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())
	_, err := svc.List(identity.WithUserID(context.Background(), 1),
		ListFilter{PageSize: 200, Search: strings.Repeat("a", 256)})
	if err == nil || err.Error() != "page_size must be at most 100" {
		t.Fatalf("error = %v, want the page_size message", err)
	}
}

func TestService_List_ReturnsTotalAmount(t *testing.T) {
	want := decimal.RequireFromString("263.40")
	svc := newSvc(&mockPurchaseRepository{
		ListFn: func(context.Context, ListFilter) (*PaginatedResult, error) {
			return &PaginatedResult{Items: []Purchase{}, Total: 11, TotalAmount: want}, nil
		},
	}, globalCatRepo())
	got, err := svc.List(identity.WithUserID(context.Background(), 1), ListFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.TotalAmount.Equal(want) {
		t.Errorf("TotalAmount = %s, want %s", got.TotalAmount, want)
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

func TestService_Update_PaymentMethodNotFound(t *testing.T) {
	pmRepo := &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, _ int64) (*paymentmethod.PaymentMethod, error) { return nil, nil },
	}
	svc := NewService(&mockPurchaseRepository{}, globalCatRepo(), pmRepo)
	_, err := svc.Update(identity.WithUserID(context.Background(), 1), 1, validUpdateRequest())
	if !errors.Is(err, paymentmethod.ErrPaymentMethodNotFound) {
		t.Errorf("expected ErrPaymentMethodNotFound, got %v", err)
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

func TestService_Create_AmountLimits(t *testing.T) {
	tests := []struct {
		name    string
		amount  string
		wantErr bool
	}{
		{name: "smallest", amount: "0.01"},
		{name: "column maximum", amount: "99999999.99"},
		{name: "trailing zero beyond two decimals", amount: "12.340"},
		{name: "above column maximum", amount: "100000000.00", wantErr: true},
		{name: "three decimal places", amount: "1.234", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCreateRequest()
			req.Amount = decimal.RequireFromString(tt.amount)
			svc := newSvc(&mockPurchaseRepository{
				CreateFn: func(_ context.Context, _ int64, r *CreatePurchaseRequest) (*Purchase, error) {
					return purchaseFromCreate(r), nil
				},
			}, globalCatRepo())

			_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)

			if tt.wantErr != errors.Is(err, apperrors.ErrValidation) {
				t.Fatalf("amount %s: wantErr=%v, got %v", tt.amount, tt.wantErr, err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestService_Create_DescriptionRules(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantErr     bool
		wantStored  string
	}{
		{name: "trimmed", description: "  Mercado \t", wantStored: "Mercado"},
		{name: "blank", description: "   ", wantErr: true},
		{name: "255 multibyte characters", description: strings.Repeat("é", 255), wantStored: strings.Repeat("é", 255)},
		{name: "256 characters", description: strings.Repeat("a", 256), wantErr: true},
		{name: "limit counted after trimming", description: " " + strings.Repeat("a", 255) + " ", wantStored: strings.Repeat("a", 255)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCreateRequest()
			req.Description = tt.description
			var stored string
			svc := newSvc(&mockPurchaseRepository{
				CreateFn: func(_ context.Context, _ int64, r *CreatePurchaseRequest) (*Purchase, error) {
					stored = r.Description
					return purchaseFromCreate(r), nil
				},
			}, globalCatRepo())

			_, err := svc.Create(identity.WithUserID(context.Background(), 1), req)

			if tt.wantErr {
				if !errors.Is(err, apperrors.ErrValidation) {
					t.Fatalf("expected ErrValidation, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stored != tt.wantStored {
				t.Errorf("stored description = %q, want %q", stored, tt.wantStored)
			}
		})
	}
}

func TestService_Update_AppliesTheSameRules(t *testing.T) {
	req := validUpdateRequest()
	req.Amount = decimal.RequireFromString("100000000.00")
	svc := newSvc(&mockPurchaseRepository{}, globalCatRepo())

	_, err := svc.Update(identity.WithUserID(context.Background(), 1), 1, req)

	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

// day is a UTC midnight date, like purchase.Date.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func reais(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestSummaryRanges(t *testing.T) {
	ten, thirtyOne, thirty, fifteen := 10, 31, 30, 15
	tests := []struct {
		name        string
		year, month int
		today       time.Time
		want        summaryPlan
	}{
		{"(a) current month: until today's day", 2026, 11, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 11, 1), MonthEnd: day(2026, 11, 30), CurrentEnd: day(2026, 11, 10),
				HasPrevious: true, PreviousStart: day(2026, 10, 1), PreviousEnd: day(2026, 10, 10)},
			period: PeriodUntilDay, untilDay: &ten, previousYear: 2026, previousMonth: 10}},
		{"(b) past month: whole months", 2026, 9, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 9, 1), MonthEnd: day(2026, 9, 30), CurrentEnd: day(2026, 9, 30),
				HasPrevious: true, PreviousStart: day(2026, 8, 1), PreviousEnd: day(2026, 8, 31)},
			period: PeriodFullMonth, previousYear: 2026, previousMonth: 8}},
		{"(c) previous month shorter: whole February", 2027, 3, day(2027, 3, 31), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2027, 3, 1), MonthEnd: day(2027, 3, 31), CurrentEnd: day(2027, 3, 31),
				HasPrevious: true, PreviousStart: day(2027, 2, 1), PreviousEnd: day(2027, 2, 28)},
			period: PeriodUntilDay, untilDay: &thirtyOne, previousYear: 2027, previousMonth: 2}},
		{"(d) leap February", 2028, 3, day(2028, 3, 30), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2028, 3, 1), MonthEnd: day(2028, 3, 31), CurrentEnd: day(2028, 3, 30),
				HasPrevious: true, PreviousStart: day(2028, 2, 1), PreviousEnd: day(2028, 2, 29)},
			period: PeriodUntilDay, untilDay: &thirty, previousYear: 2028, previousMonth: 2}},
		{"(e) January compares with December", 2027, 1, day(2027, 1, 15), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2027, 1, 1), MonthEnd: day(2027, 1, 31), CurrentEnd: day(2027, 1, 15),
				HasPrevious: true, PreviousStart: day(2026, 12, 1), PreviousEnd: day(2026, 12, 15)},
			period: PeriodUntilDay, untilDay: &fifteen, previousYear: 2026, previousMonth: 12}},
		{"(f) future month: whole months", 2026, 12, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 12, 1), MonthEnd: day(2026, 12, 31), CurrentEnd: day(2026, 12, 31),
				HasPrevious: true, PreviousStart: day(2026, 11, 1), PreviousEnd: day(2026, 11, 30)},
			period: PeriodFullMonth, previousYear: 2026, previousMonth: 11}},
		{"(g) same month, other year", 2025, 11, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2025, 11, 1), MonthEnd: day(2025, 11, 30), CurrentEnd: day(2025, 11, 30),
				HasPrevious: true, PreviousStart: day(2025, 10, 1), PreviousEnd: day(2025, 10, 31)},
			period: PeriodFullMonth, previousYear: 2025, previousMonth: 10}},
		{"(h) January of year 1 has no previous month", 1, 1, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(1, 1, 1), MonthEnd: day(1, 1, 31), CurrentEnd: day(1, 1, 31)},
			period: PeriodFullMonth, previousYear: 0, previousMonth: 12}},
		{"(i) December of 9999", 9999, 12, day(9999, 12, 31), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(9999, 12, 1), MonthEnd: day(9999, 12, 31), CurrentEnd: day(9999, 12, 31),
				HasPrevious: true, PreviousStart: day(9999, 11, 1), PreviousEnd: day(9999, 11, 30)},
			period: PeriodUntilDay, untilDay: &thirtyOne, previousYear: 9999, previousMonth: 11}},
		{"(j) today in year 0 only decides the period", 2026, 11, day(0, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 11, 1), MonthEnd: day(2026, 11, 30), CurrentEnd: day(2026, 11, 30),
				HasPrevious: true, PreviousStart: day(2026, 10, 1), PreviousEnd: day(2026, 10, 31)},
			period: PeriodFullMonth, previousYear: 2026, previousMonth: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summaryRanges(tt.year, tt.month, tt.today)
			if got.ranges != tt.want.ranges {
				t.Errorf("ranges = %+v\nwant     %+v", got.ranges, tt.want.ranges)
			}
			if got.period != tt.want.period || got.previousYear != tt.want.previousYear || got.previousMonth != tt.want.previousMonth {
				t.Errorf("period/previous = %s %d/%d, want %s %d/%d", got.period, got.previousYear, got.previousMonth,
					tt.want.period, tt.want.previousYear, tt.want.previousMonth)
			}
			switch {
			case tt.want.untilDay == nil && got.untilDay != nil:
				t.Errorf("untilDay = %d, want nil", *got.untilDay)
			case tt.want.untilDay != nil && (got.untilDay == nil || *got.untilDay != *tt.want.untilDay):
				t.Errorf("untilDay = %v, want %d", got.untilDay, *tt.want.untilDay)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		current, previous string
		wantResult        ComparisonResult
		wantPercent       int64 // ignored for NO_BASE
	}{
		{"100", "200", ResultLess, 50},     // CA-03
		{"1100", "1000", ResultMore, 10},   // CA-04
		{"0", "0", ResultNoBase, 0},        // CA-05
		{"500", "0", ResultNoBase, 0},      // RN-03
		{"1000", "1000", ResultEqual, 0},   // CA-05
		{"1004", "1000", ResultEqual, 0},   // 0.4% rounds to 0
		{"201", "200", ResultMore, 1},      // 0.5% rounds up
		{"199", "200", ResultLess, 1},      // 0.5% rounds up
		{"1", "3", ResultLess, 67},         // 66.67%
		{"0", "200", ResultLess, 100},      // nothing this month
		{"400", "200", ResultMore, 100},    // doubled
		{"3925.42", "4120", ResultLess, 5}, // prototype: October of app.js against PREVIOUS_MONTH (4.72%)
	}
	for _, tt := range tests {
		t.Run(tt.current+" vs "+tt.previous, func(t *testing.T) {
			result, percent := compare(reais(tt.current), reais(tt.previous))
			if result != tt.wantResult {
				t.Errorf("result = %s, want %s", result, tt.wantResult)
			}
			if tt.wantResult == ResultNoBase {
				if percent != nil {
					t.Errorf("percent = %d, want nil", *percent)
				}
				return
			}
			if percent == nil || *percent != tt.wantPercent {
				t.Errorf("percent = %v, want %d", percent, tt.wantPercent)
			}
		})
	}
}

func TestCompare_Properties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		current := decimal.New(rapid.Int64Range(0, 1e12).Draw(t, "currentCents"), -2)
		previous := decimal.New(rapid.Int64Range(0, 1e12).Draw(t, "previousCents"), -2)
		result, percent := compare(current, previous)

		noBase := result == ResultNoBase
		if noBase != previous.IsZero() || noBase != (percent == nil) {
			t.Fatalf("NO_BASE=%v previous=%s percent=%v: must all agree", noBase, previous, percent)
		}
		if noBase {
			return
		}
		if *percent < 0 {
			t.Fatalf("percent = %d, want >= 0", *percent)
		}
		if (result == ResultEqual) != (*percent == 0) {
			t.Fatalf("result = %s with percent %d", result, *percent)
		}
		if result == ResultMore && !current.GreaterThan(previous) {
			t.Fatalf("MORE with %s <= %s", current, previous)
		}
		if result == ResultLess && !current.LessThan(previous) {
			t.Fatalf("LESS with %s >= %s", current, previous)
		}
	})
}

func summaryService(fn func(context.Context, int64, SummaryRanges) (*SummarySums, error)) *Service {
	return newSvc(&mockPurchaseRepository{SummarizeFn: fn}, globalCatRepo())
}

func TestService_Summary_CurrentMonth(t *testing.T) {
	wantRanges := summaryRanges(2026, 11, day(2026, 11, 10)).ranges
	svc := summaryService(func(_ context.Context, userID int64, ranges SummaryRanges) (*SummarySums, error) {
		if userID != 7 {
			t.Errorf("userID = %d, want 7", userID)
		}
		if ranges != wantRanges {
			t.Errorf("ranges = %+v, want %+v", ranges, wantRanges)
		}
		return &SummarySums{MonthCount: 3, MonthSpent: reais("150"), CurrentAmount: reais("100"), PreviousAmount: reais("200")}, nil
	})
	got, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 11, day(2026, 11, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := got.Comparison
	if got.Year != 2026 || got.Month != 11 || got.PurchaseCount != 3 || !got.Spent.Equal(reais("150")) ||
		c.Period != PeriodUntilDay || c.UntilDay == nil || *c.UntilDay != 10 || c.PreviousYear != 2026 || c.PreviousMonth != 10 ||
		!c.CurrentAmount.Equal(reais("100")) || !c.PreviousAmount.Equal(reais("200")) ||
		c.Result != ResultLess || c.Percent == nil || *c.Percent != 50 {
		t.Errorf("summary = %+v, comparison = %+v", got, c)
	}
}

func TestService_Summary_OtherMonth(t *testing.T) {
	svc := summaryService(func(context.Context, int64, SummaryRanges) (*SummarySums, error) {
		return &SummarySums{MonthCount: 14, MonthSpent: reais("1100"), CurrentAmount: reais("1100"), PreviousAmount: reais("1000")}, nil
	})
	got, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 9, day(2026, 11, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := got.Comparison
	if c.Period != PeriodFullMonth || c.UntilDay != nil || c.Result != ResultMore || c.Percent == nil || *c.Percent != 10 {
		t.Errorf("comparison = %+v", c)
	}
}

func TestService_Summary_EmptyMonth(t *testing.T) {
	svc := summaryService(func(context.Context, int64, SummaryRanges) (*SummarySums, error) {
		return &SummarySums{}, nil
	})
	got, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 9, day(2026, 11, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PurchaseCount != 0 || !got.Spent.IsZero() || got.Comparison.Result != ResultNoBase || got.Comparison.Percent != nil {
		t.Errorf("summary = %+v", got)
	}
}

func TestService_Summary_NoPreviousMonth(t *testing.T) {
	var received SummaryRanges
	svc := summaryService(func(_ context.Context, _ int64, ranges SummaryRanges) (*SummarySums, error) {
		received = ranges
		return &SummarySums{PreviousAmount: reais("50")}, nil // must be ignored
	})
	got, err := svc.Summary(identity.WithUserID(context.Background(), 7), 1, 1, day(2026, 11, 10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.HasPrevious {
		t.Error("repository received HasPrevious = true")
	}
	if !got.Comparison.PreviousAmount.IsZero() || got.Comparison.Result != ResultNoBase {
		t.Errorf("comparison = %+v", got.Comparison)
	}
}

func TestService_Summary_InvalidInput(t *testing.T) {
	tests := []struct {
		year, month int
		want        string
	}{
		{2026, 0, "month must be an integer between 1 and 12"},
		{2026, 13, "month must be an integer between 1 and 12"},
		{0, 1, "year must be an integer between 1 and 9999"},
		{10000, 1, "year must be an integer between 1 and 9999"},
	}
	for _, tt := range tests {
		called := false
		svc := summaryService(func(context.Context, int64, SummaryRanges) (*SummarySums, error) {
			called = true
			return &SummarySums{}, nil
		})
		_, err := svc.Summary(identity.WithUserID(context.Background(), 7), tt.year, tt.month, day(2026, 11, 10))
		if !errors.Is(err, apperrors.ErrValidation) || err.Error() != tt.want {
			t.Errorf("(%d, %d): error = %v, want validation %q", tt.year, tt.month, err, tt.want)
		}
		if called {
			t.Errorf("(%d, %d): repository must not be called", tt.year, tt.month)
		}
	}
}

func TestService_Summary_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := summaryService(func(context.Context, int64, SummaryRanges) (*SummarySums, error) { return nil, dbErr })
	_, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 11, day(2026, 11, 10))
	if !errors.Is(err, dbErr) || errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("error = %v, want wrapped dbErr", err)
	}
}

func TestService_Summary_NilSums(t *testing.T) {
	svc := summaryService(func(context.Context, int64, SummaryRanges) (*SummarySums, error) { return nil, nil })
	_, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 11, day(2026, 11, 10))
	if err == nil || errors.Is(err, apperrors.ErrValidation) || errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("error = %v, want an internal error", err)
	}
}

func TestService_Summary_UserIDFromContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.Int64Range(1, 1_000_000).Draw(t, "userID")
		captured := int64(0)
		svc := summaryService(func(_ context.Context, id int64, _ SummaryRanges) (*SummarySums, error) {
			captured = id
			return &SummarySums{}, nil
		})
		if _, err := svc.Summary(identity.WithUserID(context.Background(), userID), 2026, 11, day(2026, 11, 10)); err != nil {
			t.Fatalf("Summary() error = %v", err)
		}
		if captured != userID {
			t.Fatalf("repository received user ID %d, want %d", captured, userID)
		}
	})
}

// ---------------------------------------------------------------------------
// archived categories and payment methods (feature 006, ADR-11)
// ---------------------------------------------------------------------------

var archivedAt = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

// archivedCategoryLookup resolves id to an own archived category of userID and any other id to an
// active global one.
func archivedCategoryLookup(id, userID int64) *mockCategoryLookup {
	return &mockCategoryLookup{
		GetByIDFn: func(_ context.Context, got int64) (*category.Category, error) {
			if got == id {
				return &category.Category{ID: id, UserID: &userID, Name: "Pets", ArchivedAt: &archivedAt}, nil
			}
			return &category.Category{ID: got, Name: "Lazer", IsGlobal: true}, nil
		},
	}
}

// archivedPaymentMethodLookup resolves id to an own archived payment method of userID and any other id
// to an active global one.
func archivedPaymentMethodLookup(id, userID int64) *mockPaymentMethodLookup {
	return &mockPaymentMethodLookup{
		GetByIDFn: func(_ context.Context, got int64) (*paymentmethod.PaymentMethod, error) {
			if got == id {
				return &paymentmethod.PaymentMethod{ID: id, UserID: &userID, Name: "Cartão antigo", ArchivedAt: &archivedAt}, nil
			}
			return &paymentmethod.PaymentMethod{ID: got, Name: "Pix", IsGlobal: true}, nil
		},
	}
}

// currentPurchase is the stored purchase the update reads: categoryID and paymentMethodID are its
// current references.
func currentPurchase(categoryID, paymentMethodID int64) *Purchase {
	return &Purchase{
		ID:            9,
		Category:      category.CategorySummary{ID: categoryID},
		PaymentMethod: paymentmethod.PaymentMethodSummary{ID: paymentMethodID},
	}
}

// updateRepo returns a repository whose GetByID returns current (counting calls and checking the user)
// and whose Update records that it was called.
func updateRepo(t *testing.T, current *Purchase, getErr error, reads, updates *int) *mockPurchaseRepository {
	return &mockPurchaseRepository{
		GetByIDFn: func(_ context.Context, userID, id int64) (*Purchase, error) {
			*reads++
			if userID != 42 || id != 9 {
				t.Errorf("GetByID(%d, %d), want (42, 9)", userID, id)
			}
			return current, getErr
		},
		UpdateFn: func(_ context.Context, _, _ int64, req *UpdatePurchaseRequest) (*Purchase, error) {
			*updates++
			return currentPurchase(req.CategoryID, req.PaymentMethodID), nil
		},
	}
}

func updateWith(categoryID, paymentMethodID int64) *UpdatePurchaseRequest {
	req := validUpdateRequest()
	req.CategoryID, req.PaymentMethodID = categoryID, paymentMethodID
	return req
}

func TestService_Create_ArchivedCategory(t *testing.T) {
	repo := &mockPurchaseRepository{
		CreateFn: func(context.Context, int64, *CreatePurchaseRequest) (*Purchase, error) {
			t.Fatal("repo.Create must not be called")
			return nil, nil
		},
	}
	svc := newSvc(repo, archivedCategoryLookup(101, 42))
	req := validCreateRequest()
	req.CategoryID = 101

	_, err := svc.Create(identity.WithUserID(context.Background(), 42), req)
	if !errors.Is(err, category.ErrCategoryNotFound) {
		t.Fatalf("err = %v, want ErrCategoryNotFound", err)
	}
}

func TestService_Create_ArchivedPaymentMethod(t *testing.T) {
	repo := &mockPurchaseRepository{
		CreateFn: func(context.Context, int64, *CreatePurchaseRequest) (*Purchase, error) {
			t.Fatal("repo.Create must not be called")
			return nil, nil
		},
	}
	svc := NewService(repo, globalCatRepo(), archivedPaymentMethodLookup(203, 42))
	req := validCreateRequest()
	req.PaymentMethodID = 203

	_, err := svc.Create(identity.WithUserID(context.Background(), 42), req)
	if !errors.Is(err, paymentmethod.ErrPaymentMethodNotFound) {
		t.Fatalf("err = %v, want ErrPaymentMethodNotFound", err)
	}
}

func TestService_Update_ArchivedReferences(t *testing.T) {
	ctx := identity.WithUserID(context.Background(), 42)
	readErr := errors.New("db failure")
	tests := []struct {
		name        string
		cats        *mockCategoryLookup
		pms         *mockPaymentMethodLookup
		current     *Purchase
		getErr      error
		req         *UpdatePurchaseRequest
		wantErr     error
		wantReads   int
		wantUpdates int
	}{
		{
			name: "keeps archived category", cats: archivedCategoryLookup(101, 42), pms: globalPaymentMethodLookup(),
			current: currentPurchase(101, 1), req: updateWith(101, 1), wantReads: 1, wantUpdates: 1,
		},
		{
			name: "switch to archived category", cats: archivedCategoryLookup(101, 42), pms: globalPaymentMethodLookup(),
			current: currentPurchase(6, 1), req: updateWith(101, 1), wantErr: category.ErrCategoryNotFound, wantReads: 1,
		},
		{
			name: "keeps archived payment method", cats: globalCatRepo(), pms: archivedPaymentMethodLookup(203, 42),
			current: currentPurchase(1, 203), req: updateWith(1, 203), wantReads: 1, wantUpdates: 1,
		},
		{
			name: "switch to archived payment method", cats: globalCatRepo(), pms: archivedPaymentMethodLookup(203, 42),
			current: currentPurchase(1, 4), req: updateWith(1, 203), wantErr: paymentmethod.ErrPaymentMethodNotFound, wantReads: 1,
		},
		{
			name: "both archived read once", cats: archivedCategoryLookup(101, 42), pms: archivedPaymentMethodLookup(203, 42),
			current: currentPurchase(101, 203), req: updateWith(101, 203), wantReads: 1, wantUpdates: 1,
		},
		{
			name: "archived category, purchase missing", cats: archivedCategoryLookup(101, 42), pms: globalPaymentMethodLookup(),
			current: nil, req: updateWith(101, 1), wantErr: category.ErrCategoryNotFound, wantReads: 1,
		},
		{
			name: "archived payment method, purchase missing", cats: globalCatRepo(), pms: archivedPaymentMethodLookup(203, 42),
			current: nil, req: updateWith(1, 203), wantErr: paymentmethod.ErrPaymentMethodNotFound, wantReads: 1,
		},
		{
			name: "active references do not read the purchase", cats: globalCatRepo(), pms: globalPaymentMethodLookup(),
			current: currentPurchase(1, 1), req: updateWith(1, 1), wantReads: 0, wantUpdates: 1,
		},
		{
			name: "read error", cats: archivedCategoryLookup(101, 42), pms: globalPaymentMethodLookup(),
			getErr: readErr, req: updateWith(101, 1), wantErr: readErr, wantReads: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reads, updates int
			svc := NewService(updateRepo(t, tc.current, tc.getErr, &reads, &updates), tc.cats, tc.pms)

			_, err := svc.Update(ctx, 9, tc.req)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if errors.Is(tc.wantErr, readErr) && errors.Is(err, category.ErrCategoryNotFound) {
				t.Fatalf("a read error must not become ErrCategoryNotFound: %v", err)
			}
			if reads != tc.wantReads || updates != tc.wantUpdates {
				t.Fatalf("GetByID calls = %d, Update calls = %d; want %d and %d", reads, updates, tc.wantReads, tc.wantUpdates)
			}
		})
	}
}
