package purchase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// mockPurchaseService is a test double for PurchaseService used in handler tests.
type mockPurchaseService struct {
	CreateFn  func(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByIDFn func(ctx context.Context, id int64) (*Purchase, error)
	ListFn    func(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	UpdateFn  func(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	DeleteFn  func(ctx context.Context, id int64) error
}

func (m *mockPurchaseService) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	if m.CreateFn == nil {
		return nil, nil
	}
	return m.CreateFn(ctx, req)
}
func (m *mockPurchaseService) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	if m.GetByIDFn == nil {
		return nil, nil
	}
	return m.GetByIDFn(ctx, id)
}
func (m *mockPurchaseService) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	if m.ListFn == nil {
		return &PaginatedResult{Items: []Purchase{}}, nil
	}
	return m.ListFn(ctx, filter)
}
func (m *mockPurchaseService) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, id, req)
}
func (m *mockPurchaseService) Delete(ctx context.Context, id int64) error {
	if m.DeleteFn == nil {
		return nil
	}
	return m.DeleteFn(ctx, id)
}

func samplePurchase() *Purchase {
	return &Purchase{
		ID:            1,
		PurchaseDate:  Date{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		Description:   "Energy bill",
		Category:      category.CategorySummary{ID: 1, Name: "Moradia", IsGlobal: true},
		PaymentMethod: PaymentMethodPix,
		Amount:        decimal.NewFromFloat(174.65),
		Status:        StatusPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func validCreateBody() string {
	return `{
		"purchase_date":  "2024-07-01",
		"description":    "Energy bill",
		"category_id":    1,
		"payment_method": "PIX",
		"amount":         "174.65",
		"status":         "PENDING"
	}`
}

// withChiID injects a chi URL param "id" into the request context.
func withChiID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func decodeError(t *testing.T, body *bytes.Buffer) httputil.APIError {
	t.Helper()
	var apiErr httputil.APIError
	if err := json.NewDecoder(body).Decode(&apiErr); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	return apiErr
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestHandler_Create_Success(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return samplePurchase(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	var got Purchase
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID=1, got %d", got.ID)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(`{invalid}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeBadRequest {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeBadRequest, apiErr.Code)
	}
}

func TestHandler_Create_ValidationError(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, apperrors.NewValidationError("amount must be greater than zero")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeValidation {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeValidation, apiErr.Code)
	}
}

func TestHandler_Create_InternalError(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, errors.New("db unavailable")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GetByID
// ---------------------------------------------------------------------------

func TestHandler_GetByID_Success(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		GetByIDFn: func(_ context.Context, id int64) (*Purchase, error) {
			return samplePurchase(), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases/1", nil)
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_GetByID_InvalidID(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodGet, "/purchases/abc", nil)
	req = withChiID(req, "abc")
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		GetByIDFn: func(_ context.Context, _ int64) (*Purchase, error) {
			return nil, apperrors.ErrNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases/99", nil)
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeNotFound {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeNotFound, apiErr.Code)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestHandler_List_Success(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		ListFn: func(_ context.Context, _ ListFilter) (*PaginatedResult, error) {
			return &PaginatedResult{Items: []Purchase{*samplePurchase()}, Total: 1, Page: 1, PageSize: 20, TotalPages: 1}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_List_InvalidMonth(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodGet, "/purchases?month=13", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_List_InvalidPageSize(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		ListFn: func(_ context.Context, _ ListFilter) (*PaginatedResult, error) {
			return nil, apperrors.NewValidationError("page_size must be at most 100")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases?page_size=200", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestHandler_List_FiltersPassedCorrectly(t *testing.T) {
	var capturedFilter ListFilter

	h := NewPurchaseHandler(&mockPurchaseService{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: 1, PageSize: 20}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases?month=7&year=2024&status=PAID&category_id=3&payment_method=PIX&page=2&page_size=10", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedFilter.Month != 7 {
		t.Errorf("expected Month=7, got %d", capturedFilter.Month)
	}
	if capturedFilter.Year != 2024 {
		t.Errorf("expected Year=2024, got %d", capturedFilter.Year)
	}
	if capturedFilter.Status != StatusPaid {
		t.Errorf("expected Status=%q, got %q", StatusPaid, capturedFilter.Status)
	}
	if capturedFilter.CategoryID != 3 {
		t.Errorf("expected CategoryID=3, got %d", capturedFilter.CategoryID)
	}
	if capturedFilter.PaymentMethod != PaymentMethodPix {
		t.Errorf("expected PaymentMethod=PIX, got %q", capturedFilter.PaymentMethod)
	}
	if capturedFilter.Page != 2 {
		t.Errorf("expected Page=2, got %d", capturedFilter.Page)
	}
	if capturedFilter.PageSize != 10 {
		t.Errorf("expected PageSize=10, got %d", capturedFilter.PageSize)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestHandler_Update_Success(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return samplePurchase(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/purchases/1", strings.NewReader(validCreateBody()))
	req.Header.Set("Content-Type", "application/json")
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_Update_NotFound(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, apperrors.ErrNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/purchases/99", strings.NewReader(validCreateBody()))
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_Create_CategoryNotFound(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, category.ErrCategoryNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if apiErr := decodeError(t, rec.Body); apiErr.Message != "category not found" {
		t.Errorf("expected message %q, got %q", "category not found", apiErr.Message)
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodPut, "/purchases/1", strings.NewReader(`{bad}`))
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestHandler_Delete_Success(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		DeleteFn: func(_ context.Context, _ int64) error { return nil },
	})

	req := httptest.NewRequest(http.MethodDelete, "/purchases/1", nil)
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{
		DeleteFn: func(_ context.Context, _ int64) error { return apperrors.ErrNotFound },
	})

	req := httptest.NewRequest(http.MethodDelete, "/purchases/99", nil)
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_Delete_InvalidID(t *testing.T) {
	h := NewPurchaseHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodDelete, "/purchases/0", nil)
	req = withChiID(req, "0")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
