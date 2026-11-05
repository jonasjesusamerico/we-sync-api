package purchase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// mockPurchaseService is a test double for service used in handler tests.
type mockPurchaseService struct {
	CreateFn  func(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByIDFn func(ctx context.Context, id int64) (*Purchase, error)
	ListFn    func(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	UpdateFn  func(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	DeleteFn  func(ctx context.Context, id int64) error

	SummaryFn func(ctx context.Context, year, month int, today time.Time) (*MonthSummary, error)
}

func (m *mockPurchaseService) Summary(ctx context.Context, year, month int, today time.Time) (*MonthSummary, error) {
	if m.SummaryFn == nil {
		return &MonthSummary{}, nil
	}
	return m.SummaryFn(ctx, year, month, today)
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
		PaymentMethod: paymentmethod.PaymentMethodSummary{ID: 4, Name: "Pix", IsGlobal: true},
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
		"payment_method_id": 4,
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
	h := NewHandler(&mockPurchaseService{
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
	if want := samplePurchase().PaymentMethod; got.PaymentMethod != want {
		t.Errorf("expected payment_method %+v, got %+v", want, got.PaymentMethod)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockPurchaseService{})

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
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodGet, "/purchases/abc", nil)
	req = withChiID(req, "abc")
	rec := httptest.NewRecorder()

	h.GetByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodGet, "/purchases?month=13", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_List_InvalidPaymentMethodID(t *testing.T) {
	for _, v := range []string{"0", "-3", "abc"} {
		h := NewHandler(&mockPurchaseService{})

		req := httptest.NewRequest(http.MethodGet, "/purchases?payment_method_id="+v, nil)
		rec := httptest.NewRecorder()

		h.List(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("payment_method_id=%s: expected 400, got %d", v, rec.Code)
		}
	}
}

func TestHandler_List_InvalidPageSize(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
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

	h := NewHandler(&mockPurchaseService{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			capturedFilter = f
			return &PaginatedResult{Items: []Purchase{}, Total: 0, Page: 1, PageSize: 20}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/purchases?month=7&year=2024&status=PAID&category_id=3&payment_method_id=5&page=2&page_size=10&q=Pad%C3%A1ria%20", nil)
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
	if capturedFilter.PaymentMethodID != 5 {
		t.Errorf("expected PaymentMethodID=5, got %d", capturedFilter.PaymentMethodID)
	}
	if capturedFilter.Page != 2 {
		t.Errorf("expected Page=2, got %d", capturedFilter.Page)
	}
	if capturedFilter.PageSize != 10 {
		t.Errorf("expected PageSize=10, got %d", capturedFilter.PageSize)
	}
	// The handler passes q as received: trimming is a rule of the service.
	if capturedFilter.Search != "Padária " {
		t.Errorf("expected Search=%q, got %q", "Padária ", capturedFilter.Search)
	}
}

func TestHandler_List_NoSearch(t *testing.T) {
	captured := ListFilter{Search: "sentinel"}
	h := NewHandler(&mockPurchaseService{
		ListFn: func(_ context.Context, f ListFilter) (*PaginatedResult, error) {
			captured = f
			return &PaginatedResult{Items: []Purchase{}}, nil
		},
	})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/purchases?month=11&year=2026", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.Search != "" {
		t.Errorf("expected empty Search, got %q", captured.Search)
	}
}

func TestHandler_List_SearchTooLong(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		ListFn: func(context.Context, ListFilter) (*PaginatedResult, error) {
			return nil, apperrors.NewValidationError("q must be at most 255 characters")
		},
	})
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/purchases?q="+strings.Repeat("a", 256), nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	body := decodeError(t, rec.Body)
	if body.Code != httputil.ErrCodeValidation || body.Message != "q must be at most 255 characters" {
		t.Errorf("body = %+v", body)
	}
}

func TestHandler_List_TotalAmountJSON(t *testing.T) {
	tests := []struct {
		name   string
		amount decimal.Decimal
		want   string
	}{
		{"trailing zeros removed", decimal.RequireFromString("263.40"), "263.4"},
		{"zero", decimal.Zero, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(&mockPurchaseService{
				ListFn: func(context.Context, ListFilter) (*PaginatedResult, error) {
					return &PaginatedResult{Items: []Purchase{}, TotalAmount: tt.amount, Page: 1, PageSize: 8}, nil
				},
			})
			rec := httptest.NewRecorder()
			h.List(rec, httptest.NewRequest(http.MethodGet, "/purchases", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}

			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if got, ok := body["total_amount"].(string); !ok || got != tt.want {
				t.Errorf("total_amount = %#v, want string %q", body["total_amount"], tt.want)
			}
			wantKeys := []string{"items", "total", "total_amount", "page", "page_size", "total_pages"}
			if len(body) != len(wantKeys) {
				t.Errorf("keys = %v, want exactly %v", body, wantKeys)
			}
			for _, k := range wantKeys {
				if _, ok := body[k]; !ok {
					t.Errorf("missing key %q", k)
				}
			}
			if items, ok := body["items"].([]any); !ok || len(items) != 0 {
				t.Errorf("items = %#v, want []", body["items"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestHandler_Update_Success(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
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

// assertNotFoundCode checks a 404 response with the exact error code and message of the contract.
func assertNotFoundCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode, wantMessage string) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != wantCode {
		t.Errorf("expected code %q, got %q", wantCode, apiErr.Code)
	}
	if apiErr.Message != wantMessage {
		t.Errorf("expected message %q, got %q", wantMessage, apiErr.Message)
	}
}

func newUpdateRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/purchases/"+id, strings.NewReader(validCreateBody()))
	return withChiID(req, id)
}

func TestHandler_Update_NotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, apperrors.ErrNotFound
		},
	})
	rec := httptest.NewRecorder()

	h.Update(rec, newUpdateRequest("99"))

	assertNotFoundCode(t, rec, httputil.ErrCodeNotFound, "purchase not found")
}

func TestHandler_Update_CategoryNotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, category.ErrCategoryNotFound
		},
	})
	rec := httptest.NewRecorder()

	h.Update(rec, newUpdateRequest("1"))

	assertNotFoundCode(t, rec, httputil.ErrCodeCategoryNotFound, "category not found")
}

func TestHandler_Update_PaymentMethodNotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
			return nil, paymentmethod.ErrPaymentMethodNotFound
		},
	})
	rec := httptest.NewRecorder()

	h.Update(rec, newUpdateRequest("1"))

	assertNotFoundCode(t, rec, httputil.ErrCodePaymentMethodNotFound, "payment method not found")
}

func TestHandler_Create_CategoryNotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, category.ErrCategoryNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assertNotFoundCode(t, rec, httputil.ErrCodeCategoryNotFound, "category not found")
}

func TestHandler_Create_PaymentMethodNotFound(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
			return nil, paymentmethod.ErrPaymentMethodNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody()))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assertNotFoundCode(t, rec, httputil.ErrCodePaymentMethodNotFound, "payment method not found")
}

// TestHandler_ReferenceNotFound_Wrapped covers the race path: the reference is deleted between the
// service check and the INSERT/UPDATE, the repository maps the FK violation to the same sentinel and
// the service returns it wrapped. The client must still get the specific code.
func TestHandler_ReferenceNotFound_Wrapped(t *testing.T) {
	tests := []struct {
		name        string
		op          string
		err         error
		wantCode    string
		wantMessage string
	}{
		{"create_category", "Create", fmt.Errorf("purchaseService.Create: %w", category.ErrCategoryNotFound), httputil.ErrCodeCategoryNotFound, "category not found"},
		{"create_payment_method", "Create", fmt.Errorf("purchaseService.Create: %w", paymentmethod.ErrPaymentMethodNotFound), httputil.ErrCodePaymentMethodNotFound, "payment method not found"},
		{"update_category", "Update", fmt.Errorf("purchaseService.Update: %w", category.ErrCategoryNotFound), httputil.ErrCodeCategoryNotFound, "category not found"},
		{"update_payment_method", "Update", fmt.Errorf("purchaseService.Update: %w", paymentmethod.ErrPaymentMethodNotFound), httputil.ErrCodePaymentMethodNotFound, "payment method not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(&mockPurchaseService{
				CreateFn: func(_ context.Context, _ *CreatePurchaseRequest) (*Purchase, error) {
					return nil, tt.err
				},
				UpdateFn: func(_ context.Context, _ int64, _ *UpdatePurchaseRequest) (*Purchase, error) {
					return nil, tt.err
				},
			})
			rec := httptest.NewRecorder()

			if tt.op == "Create" {
				h.Create(rec, httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(validCreateBody())))
			} else {
				h.Update(rec, newUpdateRequest("1"))
			}

			assertNotFoundCode(t, rec, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockPurchaseService{})

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
	h := NewHandler(&mockPurchaseService{
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
	h := NewHandler(&mockPurchaseService{
		DeleteFn: func(_ context.Context, _ int64) error { return apperrors.ErrNotFound },
	})

	req := httptest.NewRequest(http.MethodDelete, "/purchases/99", nil)
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assertNotFoundCode(t, rec, httputil.ErrCodeNotFound, "purchase not found")
}

func TestHandler_Delete_InvalidID(t *testing.T) {
	h := NewHandler(&mockPurchaseService{})

	req := httptest.NewRequest(http.MethodDelete, "/purchases/0", nil)
	req = withChiID(req, "0")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

func intPtr(n int) *int       { return &n }
func int64Ptr(n int64) *int64 { return &n }

// ca03Summary is the contract example (CA-03, today 2026-11-10).
func ca03Summary() *MonthSummary {
	return &MonthSummary{
		Year: 2026, Month: 11, PurchaseCount: 3, Spent: decimal.RequireFromString("150.00"),
		Comparison: Comparison{
			Period: PeriodUntilDay, UntilDay: intPtr(10), PreviousYear: 2026, PreviousMonth: 10,
			CurrentAmount: decimal.RequireFromString("100.00"), PreviousAmount: decimal.RequireFromString("200.00"),
			Result: ResultLess, Percent: int64Ptr(50),
		},
	}
}

func assertExactKeys(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()
	if len(obj) != len(keys) {
		t.Errorf("keys = %v, want exactly %v", obj, keys)
	}
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
}

func TestHandler_Summary_Success(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		SummaryFn: func(_ context.Context, year, month int, today time.Time) (*MonthSummary, error) {
			if year != 2026 || month != 11 || !today.Equal(time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("Summary(%d, %d, %v)", year, month, today)
			}
			return ca03Summary(), nil
		},
	})
	rec := httptest.NewRecorder()
	h.Summary(rec, httptest.NewRequest(http.MethodGet, "/purchases/summary?year=2026&month=11&today=2026-11-10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	assertExactKeys(t, body, "year", "month", "purchase_count", "spent", "comparison")
	cmp, _ := body["comparison"].(map[string]any)
	assertExactKeys(t, cmp, "period", "until_day", "previous_year", "previous_month",
		"current_amount", "previous_amount", "result", "percent")

	if body["spent"] != "150" || body["purchase_count"] != float64(3) {
		t.Errorf("spent = %#v, purchase_count = %#v", body["spent"], body["purchase_count"])
	}
	if cmp["until_day"] != float64(10) || cmp["percent"] != float64(50) || cmp["result"] != "LESS" ||
		cmp["period"] != "UNTIL_DAY" || cmp["current_amount"] != "100" || cmp["previous_amount"] != "200" {
		t.Errorf("comparison = %#v", cmp)
	}
}

func TestHandler_Summary_NullFields(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		SummaryFn: func(context.Context, int, int, time.Time) (*MonthSummary, error) {
			return &MonthSummary{Year: 2026, Month: 9, Comparison: Comparison{
				Period: PeriodFullMonth, PreviousYear: 2026, PreviousMonth: 8, Result: ResultNoBase,
			}}, nil
		},
	})
	rec := httptest.NewRecorder()
	h.Summary(rec, httptest.NewRequest(http.MethodGet, "/purchases/summary?year=2026&month=9&today=2026-11-10", nil))

	var body struct {
		Comparison map[string]any `json:"comparison"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"until_day", "percent"} {
		v, ok := body.Comparison[k]
		if !ok || v != nil {
			t.Errorf("%s: present=%v value=%#v, want present and null", k, ok, v)
		}
	}
}

func TestHandler_Summary_InvalidQuery(t *testing.T) {
	const (
		yearMsg  = "year must be an integer between 1 and 9999"
		monthMsg = "month must be an integer between 1 and 12"
		todayMsg = "today must be a date in YYYY-MM-DD format"
	)
	tests := []struct {
		query string
		want  string
	}{
		{"month=11&today=2026-11-10", yearMsg},
		{"year=0&month=11&today=2026-11-10", yearMsg},
		{"year=10000&month=11&today=2026-11-10", yearMsg},
		{"year=abc&month=11&today=2026-11-10", yearMsg},
		{"year=2026&today=2026-11-10", monthMsg},
		{"year=2026&month=0&today=2026-11-10", monthMsg},
		{"year=2026&month=13&today=2026-11-10", monthMsg},
		{"year=2026&month=11", todayMsg},
		{"year=2026&month=11&today=2026-11-31", todayMsg},
		{"year=2026&month=11&today=10/11/2026", todayMsg},
		{"year=2026&month=11&today=2026-11-10T00:00:00Z", todayMsg},
		{"year=abc&month=13", yearMsg},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			h := NewHandler(&mockPurchaseService{
				SummaryFn: func(context.Context, int, int, time.Time) (*MonthSummary, error) {
					t.Error("service must not be called")
					return nil, nil
				},
			})
			rec := httptest.NewRecorder()
			h.Summary(rec, httptest.NewRequest(http.MethodGet, "/purchases/summary?"+tt.query, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			body := decodeError(t, rec.Body)
			if body.Code != httputil.ErrCodeBadRequest || body.Message != tt.want {
				t.Errorf("body = %+v, want message %q", body, tt.want)
			}
		})
	}
}

func TestHandler_Summary_Errors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
		wantMsg  string
	}{
		{"unauthorized", apperrors.ErrUnauthorized, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "authentication required"},
		{"validation", apperrors.NewValidationError("month must be an integer between 1 and 12"),
			http.StatusUnprocessableEntity, httputil.ErrCodeValidation, "month must be an integer between 1 and 12"},
		{"internal", errors.New("pq: connection refused"), http.StatusInternalServerError, httputil.ErrCodeInternalServerError, "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(&mockPurchaseService{
				SummaryFn: func(context.Context, int, int, time.Time) (*MonthSummary, error) { return nil, tt.err },
			})
			rec := httptest.NewRecorder()
			h.Summary(rec, httptest.NewRequest(http.MethodGet, "/purchases/summary?year=2026&month=11&today=2026-11-10", nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			raw := rec.Body.String()
			if strings.Contains(raw, "pq:") {
				t.Errorf("internal detail leaked: %s", raw)
			}
			body := decodeError(t, bytes.NewBufferString(raw))
			if body.Code != tt.wantBody || body.Message != tt.wantMsg {
				t.Errorf("body = %+v", body)
			}
		})
	}
}

func TestHandler_Get_CategoryIconColor(t *testing.T) {
	h := NewHandler(&mockPurchaseService{
		GetByIDFn: func(context.Context, int64) (*Purchase, error) {
			p := samplePurchase()
			p.Category = category.CategorySummary{ID: 101, Name: "Pets", IsGlobal: false,
				Icon: category.IconHeart, Color: category.ColorLeisure}
			return p, nil
		},
	})

	rec := httptest.NewRecorder()
	h.GetByID(rec, withChiID(httptest.NewRequest(http.MethodGet, "/purchases/1", nil), "1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	want := `"category":{"id":101,"name":"Pets","is_global":false,"icon":"HEART","color":"LEISURE"}`
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("body %s does not contain %s", rec.Body.String(), want)
	}
}
