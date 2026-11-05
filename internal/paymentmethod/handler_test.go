package paymentmethod

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

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type mockPaymentMethodService struct {
	CreateFn    func(ctx context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error)
	ListFn      func(ctx context.Context) ([]PaymentMethod, error)
	UpdateFn    func(ctx context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error)
	ArchiveFn   func(ctx context.Context, id int64) error
	UnarchiveFn func(ctx context.Context, id int64) error
}

func (m *mockPaymentMethodService) Create(ctx context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
	if m.CreateFn == nil {
		return nil, false, nil
	}
	return m.CreateFn(ctx, req)
}
func (m *mockPaymentMethodService) List(ctx context.Context) ([]PaymentMethod, error) {
	if m.ListFn == nil {
		return []PaymentMethod{}, nil
	}
	return m.ListFn(ctx)
}
func (m *mockPaymentMethodService) Update(ctx context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, id, req)
}
func (m *mockPaymentMethodService) Archive(ctx context.Context, id int64) error {
	if m.ArchiveFn == nil {
		return nil
	}
	return m.ArchiveFn(ctx, id)
}
func (m *mockPaymentMethodService) Unarchive(ctx context.Context, id int64) error {
	if m.UnarchiveFn == nil {
		return nil
	}
	return m.UnarchiveFn(ctx, id)
}

func samplePaymentMethod() *PaymentMethod {
	uid := int64(42)
	return &PaymentMethod{
		ID:         1,
		UserID:     &uid,
		Name:       "Vale-refeição",
		IsGlobal:   false,
		ArchivedAt: nil,
		CreatedAt:  fixedTime,
		UpdatedAt:  fixedTime,
	}
}

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

const valeBody = `{"name":"Vale-refeição"}`

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestHandler_Create_Created(t *testing.T) {
	var got CreatePaymentMethodRequest
	h := NewHandler(&mockPaymentMethodService{
		CreateFn: func(_ context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			got = *req
			return samplePaymentMethod(), true, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payment-methods", strings.NewReader(valeBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if got != (CreatePaymentMethodRequest{Name: "Vale-refeição"}) {
		t.Fatalf("service received %+v", got)
	}
	body := rec.Body.String()
	for _, want := range []string{`"name":"Vale-refeição"`, `"archived_at":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s does not contain %s", body, want)
		}
	}
}

func TestHandler_Create_Reactivated(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{
		CreateFn: func(context.Context, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			return samplePaymentMethod(), false, nil
		},
	})

	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/payment-methods", strings.NewReader(valeBody)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got PaymentMethod
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != 1 || got.ArchivedAt != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{
		CreateFn: func(context.Context, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			t.Fatal("service must not be called")
			return nil, false, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payment-methods", strings.NewReader(`{bad}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeBadRequest || apiErr.Message != "invalid request body" {
		t.Errorf("got %+v", apiErr)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestHandler_List_Success(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{
		ListFn: func(_ context.Context) ([]PaymentMethod, error) {
			return []PaymentMethod{*samplePaymentMethod()}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payment-methods", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var got []PaymentMethod
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 item, got %d", len(got))
	}
}

func TestHandler_List_Empty(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{})

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/payment-methods", nil))

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("expected 200 with [], got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_List_InternalError(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{
		ListFn: func(_ context.Context) ([]PaymentMethod, error) {
			return nil, errors.New("db error")
		},
	})

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/payment-methods", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestHandler_Update_Success(t *testing.T) {
	var gotID int64
	var got UpdatePaymentMethodRequest
	h := NewHandler(&mockPaymentMethodService{
		UpdateFn: func(_ context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			gotID, got = id, *req
			return samplePaymentMethod(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/payment-methods/1",
		strings.NewReader(`{"name":"Cartão corporativo"}`))
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotID != 1 || got != (UpdatePaymentMethodRequest{Name: "Cartão corporativo"}) {
		t.Fatalf("service received id=%d req=%+v", gotID, got)
	}
}

func TestHandler_Update_InvalidID(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{})

	req := withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/abc", strings.NewReader(valeBody)), "abc")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockPaymentMethodService{})

	req := withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/1", strings.NewReader(`{bad}`)), "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Archive / Unarchive
// ---------------------------------------------------------------------------

func TestHandler_Archive_Success(t *testing.T) {
	var gotID int64
	h := NewHandler(&mockPaymentMethodService{
		ArchiveFn: func(_ context.Context, id int64) error { gotID = id; return nil },
	})

	rec := httptest.NewRecorder()
	h.Archive(rec, withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/5/archive", nil), "5"))

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("expected 204 with an empty body, got %d %q", rec.Code, rec.Body.String())
	}
	if gotID != 5 {
		t.Fatalf("service received id %d, want 5", gotID)
	}
}

func TestHandler_Unarchive_Success(t *testing.T) {
	var gotID int64
	h := NewHandler(&mockPaymentMethodService{
		UnarchiveFn: func(_ context.Context, id int64) error { gotID = id; return nil },
	})

	rec := httptest.NewRecorder()
	h.Unarchive(rec, withChiID(httptest.NewRequest(http.MethodDelete, "/payment-methods/5/archive", nil), "5"))

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("expected 204 with an empty body, got %d %q", rec.Code, rec.Body.String())
	}
	if gotID != 5 {
		t.Fatalf("service received id %d, want 5", gotID)
	}
}

func TestHandler_Archive_InvalidID(t *testing.T) {
	fail := func(context.Context, int64) error {
		t.Fatal("service must not be called")
		return nil
	}
	h := NewHandler(&mockPaymentMethodService{ArchiveFn: fail, UnarchiveFn: fail})
	for _, id := range []string{"0", "abc"} {
		for _, op := range []http.HandlerFunc{h.Archive, h.Unarchive} {
			rec := httptest.NewRecorder()
			op(rec, withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/"+id+"/archive", nil), id))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("id %q: expected 400, got %d", id, rec.Code)
			}
			if apiErr := decodeError(t, rec.Body); apiErr.Message != "id must be a positive integer" {
				t.Fatalf("id %q: message = %q", id, apiErr.Message)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// error mapping
// ---------------------------------------------------------------------------

func TestHandler_Errors(t *testing.T) {
	errs := []struct {
		err     error
		status  int
		code    string
		message string
	}{
		{apperrors.ErrUnauthorized, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "authentication required"},
		{apperrors.ErrForbidden, http.StatusForbidden, httputil.ErrCodeForbidden, "default payment methods cannot be changed"},
		{apperrors.ErrNotFound, http.StatusNotFound, httputil.ErrCodeNotFound, "payment method not found"},
		{ErrDefaultNameConflict, http.StatusConflict, httputil.ErrCodeDefaultNameConflict, "name already used by a default payment method"},
		{ErrArchivedNameConflict, http.StatusConflict, httputil.ErrCodeArchivedNameConflict, "name already used by an archived payment method"},
		{ErrNameConflict, http.StatusConflict, httputil.ErrCodeNameConflict, "name already used by another payment method"},
		{apperrors.ErrConflict, http.StatusConflict, httputil.ErrCodeNameConflict, "name already used by another payment method"},
		{apperrors.NewValidationError("name is required"), http.StatusUnprocessableEntity, httputil.ErrCodeValidation, "name is required"},
		{errors.New("db unavailable"), http.StatusInternalServerError, httputil.ErrCodeInternalServerError, "internal server error"},
	}
	for _, e := range errs {
		h := NewHandler(&mockPaymentMethodService{
			CreateFn: func(context.Context, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
				return nil, false, e.err
			},
			UpdateFn:    func(context.Context, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) { return nil, e.err },
			ArchiveFn:   func(context.Context, int64) error { return e.err },
			UnarchiveFn: func(context.Context, int64) error { return e.err },
		})
		ops := []struct {
			name string
			call func(*httptest.ResponseRecorder)
		}{
			{"Create", func(rec *httptest.ResponseRecorder) {
				h.Create(rec, httptest.NewRequest(http.MethodPost, "/payment-methods", strings.NewReader(valeBody)))
			}},
			{"Update", func(rec *httptest.ResponseRecorder) {
				h.Update(rec, withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/1", strings.NewReader(valeBody)), "1"))
			}},
			{"Archive", func(rec *httptest.ResponseRecorder) {
				h.Archive(rec, withChiID(httptest.NewRequest(http.MethodPut, "/payment-methods/1/archive", nil), "1"))
			}},
			{"Unarchive", func(rec *httptest.ResponseRecorder) {
				h.Unarchive(rec, withChiID(httptest.NewRequest(http.MethodDelete, "/payment-methods/1/archive", nil), "1"))
			}},
		}
		for _, op := range ops {
			t.Run(fmt.Sprintf("%s/%v", op.name, e.err), func(t *testing.T) {
				rec := httptest.NewRecorder()
				op.call(rec)
				if rec.Code != e.status {
					t.Fatalf("status = %d, want %d", rec.Code, e.status)
				}
				apiErr := decodeError(t, rec.Body)
				if apiErr.Code != e.code || apiErr.Message != e.message {
					t.Fatalf("got %+v, want {%s %s}", apiErr, e.code, e.message)
				}
			})
		}
	}
}

func TestHandler_Create_IgnoresIcon(t *testing.T) {
	var got CreatePaymentMethodRequest
	h := NewHandler(&mockPaymentMethodService{
		CreateFn: func(_ context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			got = *req
			return samplePaymentMethod(), true, nil
		},
	})

	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/payment-methods",
		strings.NewReader(`{"name":"Vale-refeição","icon":"HEART"}`)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if got != (CreatePaymentMethodRequest{Name: "Vale-refeição"}) {
		t.Fatalf("service received %+v", got)
	}
}
