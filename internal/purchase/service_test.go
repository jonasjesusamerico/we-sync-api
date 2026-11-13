package purchase

import (
	"context"
	"errors"
	"reflect"
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
		OverviewFn: func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) {
			called = true
			return &OverviewData{}, nil
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
	if _, err := svc.Overview(context.Background(), 2026, 11, nov(10)); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("Overview() error = %v, want unauthorized", err)
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
	twentyEight, twentyNine := 28, 29
	tests := []struct {
		name        string
		year, month int
		today       time.Time
		want        summaryPlan
	}{
		{"(a) current month: until today's day", 2026, 11, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 11, 1), MonthEnd: day(2026, 11, 30), CurrentEnd: day(2026, 11, 10),
				HasPrevious: true, PreviousStart: day(2026, 10, 1), PreviousEnd: day(2026, 10, 10)},
			period: PeriodUntilDay, untilDay: &ten, previousUntilDay: &ten, previousYear: 2026, previousMonth: 10}},
		{"(b) past month: whole months", 2026, 9, day(2026, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 9, 1), MonthEnd: day(2026, 9, 30), CurrentEnd: day(2026, 9, 30),
				HasPrevious: true, PreviousStart: day(2026, 8, 1), PreviousEnd: day(2026, 8, 31)},
			period: PeriodFullMonth, previousYear: 2026, previousMonth: 8}},
		{"(c) previous month shorter: whole February", 2027, 3, day(2027, 3, 31), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2027, 3, 1), MonthEnd: day(2027, 3, 31), CurrentEnd: day(2027, 3, 31),
				HasPrevious: true, PreviousStart: day(2027, 2, 1), PreviousEnd: day(2027, 2, 28)},
			period: PeriodUntilDay, untilDay: &thirtyOne, previousUntilDay: &twentyEight, previousYear: 2027, previousMonth: 2}},
		{"(d) leap February", 2028, 3, day(2028, 3, 30), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2028, 3, 1), MonthEnd: day(2028, 3, 31), CurrentEnd: day(2028, 3, 30),
				HasPrevious: true, PreviousStart: day(2028, 2, 1), PreviousEnd: day(2028, 2, 29)},
			period: PeriodUntilDay, untilDay: &thirty, previousUntilDay: &twentyNine, previousYear: 2028, previousMonth: 2}},
		{"(e) January compares with December", 2027, 1, day(2027, 1, 15), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2027, 1, 1), MonthEnd: day(2027, 1, 31), CurrentEnd: day(2027, 1, 15),
				HasPrevious: true, PreviousStart: day(2026, 12, 1), PreviousEnd: day(2026, 12, 15)},
			period: PeriodUntilDay, untilDay: &fifteen, previousUntilDay: &fifteen, previousYear: 2026, previousMonth: 12}},
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
			period: PeriodUntilDay, untilDay: &thirtyOne, previousUntilDay: &thirty, previousYear: 9999, previousMonth: 11}},
		{"(j) today in year 0 only decides the period", 2026, 11, day(0, 11, 10), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 11, 1), MonthEnd: day(2026, 11, 30), CurrentEnd: day(2026, 11, 30),
				HasPrevious: true, PreviousStart: day(2026, 10, 1), PreviousEnd: day(2026, 10, 31)},
			period: PeriodFullMonth, previousYear: 2026, previousMonth: 10}},
		{"(k) 31/10 against a 30-day September", 2026, 10, day(2026, 10, 31), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(2026, 10, 1), MonthEnd: day(2026, 10, 31), CurrentEnd: day(2026, 10, 31),
				HasPrevious: true, PreviousStart: day(2026, 9, 1), PreviousEnd: day(2026, 9, 30)},
			period: PeriodUntilDay, untilDay: &thirtyOne, previousUntilDay: &thirty, previousYear: 2026, previousMonth: 9}},
		{"(l) today in January of year 1", 1, 1, day(1, 1, 15), summaryPlan{
			ranges: SummaryRanges{MonthStart: day(1, 1, 1), MonthEnd: day(1, 1, 31), CurrentEnd: day(1, 1, 15)},
			period: PeriodUntilDay, untilDay: &fifteen, previousYear: 0, previousMonth: 12}},
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
			switch {
			case tt.want.previousUntilDay == nil && got.previousUntilDay != nil:
				t.Errorf("previousUntilDay = %d, want nil", *got.previousUntilDay)
			case tt.want.previousUntilDay != nil && (got.previousUntilDay == nil || *got.previousUntilDay != *tt.want.previousUntilDay):
				t.Errorf("previousUntilDay = %v, want %d", got.previousUntilDay, *tt.want.previousUntilDay)
			}
		})
	}
}

func TestMonthBounds(t *testing.T) {
	tests := []struct {
		year, month int
		wantEnd     time.Time
	}{
		{2026, 11, day(2026, 11, 30)},
		{2026, 2, day(2026, 2, 28)},
		{2028, 2, day(2028, 2, 29)},
		{2026, 12, day(2026, 12, 31)},
		{1, 1, day(1, 1, 31)},
		{9999, 12, day(9999, 12, 31)},
	}
	for _, tt := range tests {
		start, end := monthBounds(tt.year, tt.month)
		if want := day(tt.year, time.Month(tt.month), 1); !start.Equal(want) {
			t.Errorf("(%d, %d): start = %s, want %s", tt.year, tt.month, start, want)
		}
		if !end.Equal(tt.wantEnd) {
			t.Errorf("(%d, %d): end = %s, want %s", tt.year, tt.month, end, tt.wantEnd)
		}
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
		c.Period != PeriodUntilDay || c.UntilDay == nil || *c.UntilDay != 10 ||
		c.PreviousUntilDay == nil || *c.PreviousUntilDay != 10 || c.PreviousYear != 2026 || c.PreviousMonth != 10 ||
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
	if c.Period != PeriodFullMonth || c.UntilDay != nil || c.PreviousUntilDay != nil ||
		c.Result != ResultMore || c.Percent == nil || *c.Percent != 10 {
		t.Errorf("comparison = %+v", c)
	}
}

// CA-13 of feature 007: the 31st against a 30-day September.
func TestService_Summary_ShorterPreviousMonth(t *testing.T) {
	svc := summaryService(func(_ context.Context, _ int64, ranges SummaryRanges) (*SummarySums, error) {
		if !ranges.PreviousEnd.Equal(day(2026, 9, 30)) {
			t.Errorf("PreviousEnd = %s, want 2026-09-30", ranges.PreviousEnd)
		}
		return &SummarySums{MonthCount: 12, MonthSpent: reais("950"), CurrentAmount: reais("950"), PreviousAmount: reais("1000")}, nil
	})
	got, err := svc.Summary(identity.WithUserID(context.Background(), 7), 2026, 10, day(2026, 10, 31))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := got.Comparison
	if c.Period != PeriodUntilDay || c.UntilDay == nil || *c.UntilDay != 31 ||
		c.PreviousUntilDay == nil || *c.PreviousUntilDay != 30 || c.PreviousYear != 2026 || c.PreviousMonth != 9 ||
		c.Result != ResultLess || c.Percent == nil || *c.Percent != 5 {
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

// ---------------------------------------------------------------------------
// Overview (feature 007)
// ---------------------------------------------------------------------------

// nov is a day of November 2026, the month of the acceptance criteria (today 2026-11-10).
func nov(d int) time.Time { return day(2026, 11, d) }

func dayTotals(date time.Time, paid string, paidN int, pending string, pendingN int) DayTotals {
	return DayTotals{Date: date, PaidAmount: reais(paid), PaidCount: paidN, PendingAmount: reais(pending), PendingCount: pendingN}
}

func catTotal(id int64, name, amount string) CategoryTotal {
	return CategoryTotal{Category: category.CategorySummary{ID: id, Name: name, IsGlobal: true}, Amount: reais(amount)}
}

func pmTotal(id int64, name, amount string) PaymentMethodTotal {
	return PaymentMethodTotal{PaymentMethod: paymentmethod.PaymentMethodSummary{ID: id, Name: name, IsGlobal: true}, Amount: reais(amount)}
}

// Global categories and payment methods of the seed used by the examples.
var (
	catFood      = category.CategorySummary{ID: 1, Name: "Alimentação", IsGlobal: true, Icon: category.IconFood, Color: category.ColorFood}
	catTransport = category.CategorySummary{ID: 2, Name: "Transporte", IsGlobal: true, Icon: category.IconCar, Color: category.ColorTransport}
	catHome      = category.CategorySummary{ID: 3, Name: "Moradia", IsGlobal: true, Icon: category.IconHouse, Color: category.ColorHome}
	catLeisure   = category.CategorySummary{ID: 6, Name: "Lazer", IsGlobal: true, Icon: category.IconSmile, Color: category.ColorLeisure}
	pmCredit     = paymentmethod.PaymentMethodSummary{ID: 2, Name: "Cartão de crédito", IsGlobal: true}
	pmPix        = paymentmethod.PaymentMethodSummary{ID: 4, Name: "Pix", IsGlobal: true}
	pmBoleto     = paymentmethod.PaymentMethodSummary{ID: 6, Name: "Boleto", IsGlobal: true}
)

func e1Purchase(id int64, date time.Time, desc string, c category.CategorySummary,
	pm paymentmethod.PaymentMethodSummary, amount string, status Status) Purchase {
	return Purchase{ID: id, UserID: 7, PurchaseDate: Date{date}, Description: desc, Category: c, PaymentMethod: pm,
		Amount: reais(amount), Status: status, CreatedAt: date, UpdatedAt: date}
}

// e1Data is example E1 of the SDD (the acceptance criteria table, today 2026-11-10), as the
// repository returns it: CANCELED out of the sums, rows already ordered.
func e1Data() *OverviewData {
	return &OverviewData{
		Days: []DayTotals{
			dayTotals(nov(2), "100", 1, "0", 0),
			dayTotals(nov(5), "100", 2, "0", 0),
			dayTotals(nov(25), "0", 0, "1000", 1),
		},
		Categories: []CategoryTotal{
			{Category: catHome, Amount: reais("1000")},
			{Category: catFood, Amount: reais("160")},
			{Category: catTransport, Amount: reais("40")},
		},
		PaymentMethods: []PaymentMethodTotal{
			{PaymentMethod: pmBoleto, Amount: reais("1000")},
			{PaymentMethod: pmPix, Amount: reais("160")},
			{PaymentMethod: pmCredit, Amount: reais("40")},
		},
		Latest: []Purchase{
			e1Purchase(105, nov(25), "Aluguel", catHome, pmBoleto, "1000", StatusPending),
			e1Purchase(104, nov(8), "Cinema", catLeisure, pmCredit, "30", StatusCanceled),
			e1Purchase(103, nov(5), "Padaria", catFood, pmPix, "60", StatusPaid),
			e1Purchase(102, nov(5), "Uber", catTransport, pmCredit, "40", StatusPaid),
			e1Purchase(101, nov(2), "Mercado", catFood, pmPix, "100", StatusPaid),
		},
	}
}

// e2Data is example E2 of the SDD: October 2026 of prototype/assets/app.js (the canceled show out).
func e2Data() *OverviewData {
	oct := func(d int) time.Time { return day(2026, 10, d) }
	paid := func(d int, amount string) DayTotals { return dayTotals(oct(d), amount, 1, "0", 0) }
	pending := func(d int, amount string) DayTotals { return dayTotals(oct(d), "0", 0, amount, 1) }
	return &OverviewData{
		Days: []DayTotals{
			paid(2, "1450.00"), paid(3, "386.45"), paid(5, "210.00"), paid(7, "74.90"), paid(8, "58.00"),
			paid(10, "320.00"), paid(12, "64.00"), paid(14, "182.37"), paid(15, "92.50"), paid(17, "38.70"),
			paid(19, "27.80"), pending(21, "250.00"), paid(23, "412.30"), pending(25, "119.90"), paid(28, "69.90"),
			paid(29, "143.60"), pending(30, "25.00"),
		},
		Categories: []CategoryTotal{
			catTotal(3, "Moradia", "1752.27"), catTotal(1, "Alimentação", "1120.65"), catTotal(5, "Educação", "389.90"),
			catTotal(4, "Saúde", "324.90"), catTotal(2, "Transporte", "273.70"), catTotal(6, "Lazer", "64.00"),
		},
		PaymentMethods: []PaymentMethodTotal{
			pmTotal(6, "Boleto", "1889.90"), pmTotal(2, "Cartão de crédito", "1118.60"), pmTotal(3, "Cartão de débito", "414.25"),
			pmTotal(4, "Pix", "410.17"), pmTotal(1, "Dinheiro", "92.50"),
		},
	}
}

func catPercents(ov *MonthOverview) []int64 {
	out := []int64{}
	for _, c := range ov.Categories {
		out = append(out, c.Percent)
	}
	return out
}

func pmPercents(ov *MonthOverview) []int64 {
	out := []int64{}
	for _, pm := range ov.PaymentMethods {
		out = append(out, pm.Percent)
	}
	return out
}

// forecastDays lists the days marked as forecast.
func forecastDays(ov *MonthOverview) []int {
	out := []int{}
	for _, d := range ov.Daily {
		if d.Forecast {
			out = append(out, d.Day)
		}
	}
	return out
}

func daysRange(from, to int) []int {
	out := []int{}
	for d := from; d <= to; d++ {
		out = append(out, d)
	}
	return out
}

func checkTotal(t *testing.T, name string, got StatusTotal, amount string, count int) {
	t.Helper()
	if !got.Amount.Equal(reais(amount)) || got.Count != count {
		t.Errorf("%s = {%s, %d}, want {%s, %d}", name, got.Amount, got.Count, amount, count)
	}
}

func checkAverage(t *testing.T, got DailyAverage, amount string, days int) {
	t.Helper()
	if got.Amount.String() != amount || got.Days != days {
		t.Errorf("daily average = {%s, %d}, want {%s, %d}", got.Amount, got.Days, amount, days)
	}
}

func checkPeak(t *testing.T, got *int, want int) {
	t.Helper()
	if want == 0 {
		if got != nil {
			t.Errorf("peak day = %d, want nil", *got)
		}
		return
	}
	if got == nil || *got != want {
		t.Errorf("peak day = %v, want %d", got, want)
	}
}

func TestBuildOverview(t *testing.T) {
	oneDay := func(date time.Time, paid string, paidN int, pending string, pendingN int) *OverviewData {
		return &OverviewData{Days: []DayTotals{dayTotals(date, paid, paidN, pending, pendingN)}}
	}
	tests := []struct {
		name        string
		year, month int
		today       time.Time
		data        *OverviewData
		check       func(t *testing.T, ov *MonthOverview)
	}{
		{"CA-01 kpis", 2026, 11, nov(10), e1Data(), func(t *testing.T, ov *MonthOverview) {
			checkTotal(t, "paid", ov.Paid, "200", 3)
			checkTotal(t, "pending", ov.Pending, "1000", 1)
			checkAverage(t, ov.DailyAverage, "20", 10)
			if ov.ActiveCount != 4 || ov.Spent.String() != "1200" || !ov.IsCurrentMonth || ov.DaysInMonth != 30 {
				t.Errorf("active %d spent %s current %v days %d", ov.ActiveCount, ov.Spent, ov.IsCurrentMonth, ov.DaysInMonth)
			}
		}},
		{"CA-02 daily", 2026, 11, nov(10), e1Data(), func(t *testing.T, ov *MonthOverview) {
			if len(ov.Daily) != 30 {
				t.Fatalf("len(daily) = %d, want 30", len(ov.Daily))
			}
			for i, d := range ov.Daily {
				if d.Day != i+1 {
					t.Errorf("daily[%d].Day = %d", i, d.Day)
				}
			}
			want := map[int]string{2: "100", 5: "100", 8: "0", 25: "1000", 1: "0", 30: "0"}
			for d, amount := range want {
				if ov.Daily[d-1].Amount.String() != amount {
					t.Errorf("day %d = %s, want %s", d, ov.Daily[d-1].Amount, amount)
				}
			}
			if got := forecastDays(ov); !reflect.DeepEqual(got, daysRange(11, 30)) {
				t.Errorf("forecast days = %v", got)
			}
			checkPeak(t, ov.PeakDay, 25)
		}},
		{"CA-03 peak tie", 2026, 11, nov(10), &OverviewData{Days: []DayTotals{
			dayTotals(nov(2), "100", 1, "0", 0), dayTotals(nov(5), "100", 1, "0", 0),
		}}, func(t *testing.T, ov *MonthOverview) { checkPeak(t, ov.PeakDay, 2) }},
		{"CA-04/05 shares", 2026, 11, nov(10), e1Data(), func(t *testing.T, ov *MonthOverview) {
			if got := catPercents(ov); !reflect.DeepEqual(got, []int64{83, 13, 3}) {
				t.Errorf("category percents = %v", got)
			}
			if ov.Categories[0].Category != catHome || ov.Categories[1].Category != catFood || ov.Categories[2].Category != catTransport {
				t.Errorf("categories = %+v", ov.Categories)
			}
			if got := pmPercents(ov); !reflect.DeepEqual(got, []int64{83, 13, 3}) {
				t.Errorf("payment method percents = %v", got)
			}
			if ov.PaymentMethods[0].PaymentMethod != pmBoleto || ov.PaymentMethods[1].PaymentMethod != pmPix ||
				ov.PaymentMethods[2].PaymentMethod != pmCredit {
				t.Errorf("payment methods = %+v", ov.PaymentMethods)
			}
		}},
		{"CA-06/07 latest passes through", 2026, 11, nov(10), e1Data(), func(t *testing.T, ov *MonthOverview) {
			ids := []int64{}
			for _, p := range ov.Latest {
				ids = append(ids, p.ID)
			}
			if !reflect.DeepEqual(ids, []int64{105, 104, 103, 102, 101}) {
				t.Errorf("latest ids = %v", ids)
			}
		}},
		{"CA-09 past month", 2026, 9, nov(10), oneDay(day(2026, 9, 10), "900", 3, "0", 0), func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "30", 30)
			if ov.IsCurrentMonth || len(forecastDays(ov)) != 0 {
				t.Errorf("current %v forecast %v", ov.IsCurrentMonth, forecastDays(ov))
			}
		}},
		{"CA-10 future month", 2026, 12, nov(10), oneDay(day(2026, 12, 5), "0", 0, "100", 1), func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "3.23", 31)
			checkPeak(t, ov.PeakDay, 5)
			if ov.DaysInMonth != 31 || ov.ActiveCount != 1 || len(forecastDays(ov)) != 0 {
				t.Errorf("days %d active %d forecast %v", ov.DaysInMonth, ov.ActiveCount, forecastDays(ov))
			}
		}},
		{"CA-11 empty month", 2026, 12, nov(10), &OverviewData{}, func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "0", 31)
			checkPeak(t, ov.PeakDay, 0)
			if ov.ActiveCount != 0 || !ov.Spent.IsZero() || len(ov.Daily) != 31 {
				t.Errorf("active %d spent %s daily %d", ov.ActiveCount, ov.Spent, len(ov.Daily))
			}
			for _, d := range ov.Daily {
				if !d.Amount.IsZero() {
					t.Errorf("day %d = %s", d.Day, d.Amount)
				}
			}
			if ov.Categories == nil || ov.PaymentMethods == nil || ov.Latest == nil ||
				len(ov.Categories)+len(ov.PaymentMethods)+len(ov.Latest) != 0 {
				t.Errorf("lists must be empty and not nil: %#v %#v %#v", ov.Categories, ov.PaymentMethods, ov.Latest)
			}
		}},
		{"CA-12 only canceled", 2026, 8, nov(10), &OverviewData{Latest: []Purchase{
			e1Purchase(120, day(2026, 8, 15), "Show", catLeisure, pmCredit, "80", StatusCanceled),
		}}, func(t *testing.T, ov *MonthOverview) {
			checkPeak(t, ov.PeakDay, 0)
			if ov.ActiveCount != 0 || !ov.Spent.IsZero() || len(ov.Latest) != 1 {
				t.Errorf("active %d spent %s latest %d", ov.ActiveCount, ov.Spent, len(ov.Latest))
			}
		}},
		{"CA-15 leap February", 2028, 2, nov(10), &OverviewData{}, func(t *testing.T, ov *MonthOverview) {
			if ov.DaysInMonth != 29 || len(ov.Daily) != 29 || ov.DailyAverage.Days != 29 {
				t.Errorf("days %d daily %d average days %d", ov.DaysInMonth, len(ov.Daily), ov.DailyAverage.Days)
			}
		}},
		{"CA-15 February", 2027, 2, nov(10), &OverviewData{}, func(t *testing.T, ov *MonthOverview) {
			if ov.DaysInMonth != 28 || len(ov.Daily) != 28 {
				t.Errorf("days %d daily %d", ov.DaysInMonth, len(ov.Daily))
			}
		}},
		{"CA-16 first day", 2026, 11, nov(1), oneDay(nov(1), "50", 1, "0", 0), func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "50", 1)
			if got := forecastDays(ov); !reflect.DeepEqual(got, daysRange(2, 30)) {
				t.Errorf("forecast days = %v", got)
			}
		}},
		{"current month ignores future days in the average", 2026, 11, nov(10), &OverviewData{Days: []DayTotals{
			dayTotals(nov(2), "100", 1, "0", 0), dayTotals(nov(25), "0", 0, "1000", 1),
		}}, func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "10", 10)
			if ov.Spent.String() != "1100" {
				t.Errorf("spent = %s", ov.Spent)
			}
		}},
		{"last day of the month has no forecast", 2026, 10, day(2026, 10, 31), e2Data(), func(t *testing.T, ov *MonthOverview) {
			checkTotal(t, "paid", ov.Paid, "3530.52", 14)
			checkTotal(t, "pending", ov.Pending, "394.9", 3)
			checkAverage(t, ov.DailyAverage, "126.63", 31)
			checkPeak(t, ov.PeakDay, 2)
			if ov.ActiveCount != 17 || ov.Spent.String() != "3925.42" || !ov.IsCurrentMonth || len(forecastDays(ov)) != 0 {
				t.Errorf("active %d spent %s current %v forecast %v", ov.ActiveCount, ov.Spent, ov.IsCurrentMonth, forecastDays(ov))
			}
			if got := catPercents(ov); !reflect.DeepEqual(got, []int64{45, 29, 10, 8, 7, 2}) {
				t.Errorf("category percents = %v (want sum 101)", got)
			}
			if got := pmPercents(ov); !reflect.DeepEqual(got, []int64{48, 28, 11, 10, 2}) {
				t.Errorf("payment method percents = %v (want sum 99)", got)
			}
		}},
		{"average rounds half a cent up", 2026, 11, nov(2), oneDay(nov(1), "0.01", 1, "0", 0), func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "0.01", 2)
		}},
		{"percent rounds half up", 2026, 9, nov(10), &OverviewData{
			Days:       []DayTotals{dayTotals(day(2026, 9, 1), "8", 2, "0", 0)},
			Categories: []CategoryTotal{catTotal(10, "A", "1"), catTotal(11, "B", "7")},
		}, func(t *testing.T, ov *MonthOverview) {
			if got := catPercents(ov); !reflect.DeepEqual(got, []int64{13, 88}) {
				t.Errorf("percents = %v", got)
			}
		}},
		{"percent of a tiny share is 0", 2026, 9, nov(10), &OverviewData{
			Days:       []DayTotals{dayTotals(day(2026, 9, 1), "1000.01", 2, "0", 0)},
			Categories: []CategoryTotal{catTotal(10, "A", "1000"), catTotal(11, "B", "0.01")},
		}, func(t *testing.T, ov *MonthOverview) {
			if got := catPercents(ov); !reflect.DeepEqual(got, []int64{100, 0}) {
				t.Errorf("percents = %v", got)
			}
		}},
		{"same month of another year is not current", 2025, 11, nov(10), &OverviewData{}, func(t *testing.T, ov *MonthOverview) {
			if ov.IsCurrentMonth || ov.DailyAverage.Days != 30 || len(forecastDays(ov)) != 0 {
				t.Errorf("current %v average days %d forecast %v", ov.IsCurrentMonth, ov.DailyAverage.Days, forecastDays(ov))
			}
		}},
		{"today in year 0 only decides the period", 2026, 11, day(0, 11, 10), e1Data(), func(t *testing.T, ov *MonthOverview) {
			checkAverage(t, ov.DailyAverage, "40", 30)
			if ov.IsCurrentMonth {
				t.Error("IsCurrentMonth = true")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ov, err := buildOverview(tt.year, tt.month, tt.today, tt.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ov.Year != tt.year || ov.Month != tt.month {
				t.Errorf("year/month = %d/%d", ov.Year, ov.Month)
			}
			tt.check(t, ov)
		})
	}
}

func TestBuildOverview_DayOutsideMonth(t *testing.T) {
	data := &OverviewData{Days: []DayTotals{dayTotals(day(2026, 12, 1), "10", 1, "0", 0)}}
	if _, err := buildOverview(2026, 11, nov(10), data); err == nil {
		t.Fatal("want an error for a day outside the month")
	}
}

// split divides total (> 0) into n parts > 0, in cents.
func split(t *rapid.T, label string, total int64, n int) []int64 {
	if int64(n) > total {
		n = int(total)
	}
	parts := make([]int64, n)
	rest := total
	for i := 0; i < n-1; i++ {
		// leave at least 1 cent for each remaining part
		p := rapid.Int64Range(1, rest-int64(n-1-i)).Draw(t, label)
		parts[i] = p
		rest -= p
	}
	parts[n-1] = rest
	return parts
}

func TestBuildOverview_Properties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		year := rapid.IntRange(2000, 2100).Draw(t, "year")
		month := rapid.IntRange(1, 12).Draw(t, "month")
		start, end := monthBounds(year, month)
		offset := rapid.IntRange(-1, 1).Draw(t, "todayMonthOffset")
		todayMonthStart := start.AddDate(0, offset, 0)
		_, todayMonthEnd := monthBounds(todayMonthStart.Year(), int(todayMonthStart.Month()))
		today := todayMonthStart.AddDate(0, 0, rapid.IntRange(0, todayMonthEnd.Day()-1).Draw(t, "todayDay"))

		data := &OverviewData{}
		var spentCents int64
		for d := 1; d <= end.Day(); d++ {
			if !rapid.Bool().Draw(t, "hasDay") {
				continue
			}
			paidN := rapid.IntRange(0, 3).Draw(t, "paidN")
			pendingN := rapid.IntRange(0, 3).Draw(t, "pendingN")
			if paidN+pendingN == 0 {
				paidN = 1
			}
			var paid, pending int64
			if paidN > 0 {
				paid = rapid.Int64Range(1, 1e8).Draw(t, "paidCents")
			}
			if pendingN > 0 {
				pending = rapid.Int64Range(1, 1e8).Draw(t, "pendingCents")
			}
			spentCents += paid + pending
			data.Days = append(data.Days, DayTotals{Date: day(year, time.Month(month), d),
				PaidAmount: decimal.New(paid, -2), PaidCount: paidN, PendingAmount: decimal.New(pending, -2), PendingCount: pendingN})
		}
		if spentCents > 0 {
			for i, c := range split(t, "category", spentCents, rapid.IntRange(1, 5).Draw(t, "nCategories")) {
				data.Categories = append(data.Categories, CategoryTotal{
					Category: category.CategorySummary{ID: int64(i + 1)}, Amount: decimal.New(c, -2)})
			}
			for i, c := range split(t, "paymentMethod", spentCents, rapid.IntRange(1, 5).Draw(t, "nPaymentMethods")) {
				data.PaymentMethods = append(data.PaymentMethods, PaymentMethodTotal{
					PaymentMethod: paymentmethod.PaymentMethodSummary{ID: int64(i + 1)}, Amount: decimal.New(c, -2)})
			}
		}

		ov, err := buildOverview(year, month, today, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ov.Daily) != ov.DaysInMonth || ov.DaysInMonth != end.Day() {
			t.Fatalf("len(daily) = %d, days in month = %d", len(ov.Daily), ov.DaysInMonth)
		}
		dailySum, catSum, pmSum := decimal.Zero, decimal.Zero, decimal.Zero
		for i, d := range ov.Daily {
			if d.Day != i+1 {
				t.Fatalf("daily[%d].Day = %d", i, d.Day)
			}
			dailySum = dailySum.Add(d.Amount)
		}
		for _, c := range ov.Categories {
			catSum = catSum.Add(c.Amount)
			if c.Percent < 0 || c.Percent > 100 {
				t.Fatalf("category percent = %d", c.Percent)
			}
		}
		for _, pm := range ov.PaymentMethods {
			pmSum = pmSum.Add(pm.Amount)
			if pm.Percent < 0 || pm.Percent > 100 {
				t.Fatalf("payment method percent = %d", pm.Percent)
			}
		}
		if !dailySum.Equal(ov.Spent) || !catSum.Equal(ov.Spent) || !pmSum.Equal(ov.Spent) ||
			!ov.Paid.Amount.Add(ov.Pending.Amount).Equal(ov.Spent) {
			t.Fatalf("sums differ: daily %s categories %s payment methods %s paid+pending %s spent %s",
				dailySum, catSum, pmSum, ov.Paid.Amount.Add(ov.Pending.Amount), ov.Spent)
		}
		if ov.ActiveCount != ov.Paid.Count+ov.Pending.Count {
			t.Fatalf("active %d != %d + %d", ov.ActiveCount, ov.Paid.Count, ov.Pending.Count)
		}
		if (ov.PeakDay == nil) != ov.Spent.IsZero() {
			t.Fatalf("peak %v with spent %s", ov.PeakDay, ov.Spent)
		}
		if ov.PeakDay != nil {
			peak := ov.Daily[*ov.PeakDay-1].Amount
			for i, d := range ov.Daily {
				if d.Amount.GreaterThan(peak) || (i < *ov.PeakDay-1 && d.Amount.Equal(peak)) {
					t.Fatalf("day %d (%s) beats peak day %d (%s)", d.Day, d.Amount, *ov.PeakDay, peak)
				}
			}
		}
		isCurrent := today.Year() == year && int(today.Month()) == month
		if ov.IsCurrentMonth != isCurrent {
			t.Fatalf("IsCurrentMonth = %v, today %s", ov.IsCurrentMonth, today.Format(dateLayout))
		}
		for _, d := range ov.Daily {
			if d.Forecast != (isCurrent && d.Day > today.Day()) {
				t.Fatalf("day %d forecast = %v, today %s", d.Day, d.Forecast, today.Format(dateLayout))
			}
		}
		avg := ov.DailyAverage
		if avg.Days < 1 || avg.Days > 31 || avg.Amount.Sign() < 0 || !avg.Amount.Equal(avg.Amount.Round(2)) {
			t.Fatalf("daily average = {%s, %d}", avg.Amount, avg.Days)
		}
	})
}

func overviewService(fn func(context.Context, int64, time.Time, time.Time) (*OverviewData, error)) *Service {
	return newSvc(&mockPurchaseRepository{OverviewFn: fn}, globalCatRepo())
}

func TestService_Overview_PassesMonthBounds(t *testing.T) {
	tests := []struct {
		year, month int
		start, end  time.Time
	}{
		{2026, 11, day(2026, 11, 1), day(2026, 11, 30)},
		{2028, 2, day(2028, 2, 1), day(2028, 2, 29)},
		{9999, 12, day(9999, 12, 1), day(9999, 12, 31)},
		{1, 1, day(1, 1, 1), day(1, 1, 31)},
	}
	for _, tt := range tests {
		svc := overviewService(func(_ context.Context, userID int64, start, end time.Time) (*OverviewData, error) {
			if userID != 7 || !start.Equal(tt.start) || !end.Equal(tt.end) {
				t.Errorf("(%d, %d): repository got user %d, %s..%s", tt.year, tt.month, userID,
					start.Format(dateLayout), end.Format(dateLayout))
			}
			return &OverviewData{}, nil
		})
		if _, err := svc.Overview(identity.WithUserID(context.Background(), 7), tt.year, tt.month, nov(10)); err != nil {
			t.Errorf("(%d, %d): unexpected error: %v", tt.year, tt.month, err)
		}
	}
}

func TestService_Overview_BuildsFromRepository(t *testing.T) {
	svc := overviewService(func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) { return e1Data(), nil })
	got, err := svc.Overview(identity.WithUserID(context.Background(), 7), 2026, 11, nov(10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := buildOverview(2026, 11, nov(10), e1Data())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Overview() = %+v\nwant %+v", got, want)
	}
}

func TestService_Overview_InvalidInput(t *testing.T) {
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
		svc := overviewService(func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) {
			called = true
			return &OverviewData{}, nil
		})
		_, err := svc.Overview(identity.WithUserID(context.Background(), 7), tt.year, tt.month, nov(10))
		if !errors.Is(err, apperrors.ErrValidation) || err.Error() != tt.want {
			t.Errorf("(%d, %d): error = %v, want validation %q", tt.year, tt.month, err, tt.want)
		}
		if called {
			t.Errorf("(%d, %d): repository must not be called", tt.year, tt.month)
		}
	}
}

func TestService_Overview_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := overviewService(func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) { return nil, dbErr })
	_, err := svc.Overview(identity.WithUserID(context.Background(), 7), 2026, 11, nov(10))
	if !errors.Is(err, dbErr) || errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("error = %v, want wrapped dbErr", err)
	}
}

func TestService_Overview_NilData(t *testing.T) {
	svc := overviewService(func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) { return nil, nil })
	_, err := svc.Overview(identity.WithUserID(context.Background(), 7), 2026, 11, nov(10))
	if err == nil || errors.Is(err, apperrors.ErrValidation) || errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("error = %v, want an internal error", err)
	}
}

func TestService_Overview_BuildError(t *testing.T) {
	svc := overviewService(func(context.Context, int64, time.Time, time.Time) (*OverviewData, error) {
		return &OverviewData{Days: []DayTotals{dayTotals(day(2026, 12, 1), "10", 1, "0", 0)}}, nil
	})
	_, err := svc.Overview(identity.WithUserID(context.Background(), 7), 2026, 11, nov(10))
	if err == nil || errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("error = %v, want an internal error", err)
	}
}

func TestService_Overview_UserIDFromContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.Int64Range(1, 1_000_000).Draw(t, "userID")
		captured := int64(0)
		svc := overviewService(func(_ context.Context, id int64, _, _ time.Time) (*OverviewData, error) {
			captured = id
			return &OverviewData{}, nil
		})
		if _, err := svc.Overview(identity.WithUserID(context.Background(), userID), 2026, 11, nov(10)); err != nil {
			t.Fatalf("Overview() error = %v", err)
		}
		if captured != userID {
			t.Fatalf("repository received user ID %d, want %d", captured, userID)
		}
	})
}
