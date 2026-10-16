package category

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
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type mockCategoryService struct {
	CreateFn func(ctx context.Context, req *CreateCategoryRequest) (*Category, error)
	ListFn   func(ctx context.Context) ([]Category, error)
	UpdateFn func(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error)
	DeleteFn func(ctx context.Context, id int64) error
}

func (m *mockCategoryService) Create(ctx context.Context, req *CreateCategoryRequest) (*Category, error) {
	if m.CreateFn == nil {
		return nil, nil
	}
	return m.CreateFn(ctx, req)
}
func (m *mockCategoryService) List(ctx context.Context) ([]Category, error) {
	if m.ListFn == nil {
		return []Category{}, nil
	}
	return m.ListFn(ctx)
}
func (m *mockCategoryService) Update(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error) {
	if m.UpdateFn == nil {
		return nil, nil
	}
	return m.UpdateFn(ctx, id, req)
}
func (m *mockCategoryService) Delete(ctx context.Context, id int64) error {
	if m.DeleteFn == nil {
		return nil
	}
	return m.DeleteFn(ctx, id)
}

func sampleCategory() *Category {
	uid := int64(42)
	return &Category{
		ID:        1,
		UserID:    &uid,
		Name:      "Gasolina",
		IsGlobal:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestHandler_Create_Success(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, _ *CreateCategoryRequest) (*Category, error) {
			return sampleCategory(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Gasolina"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	var got Category
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID=1, got %d", got.ID)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{bad}`))
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
	h := NewCategoryHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, _ *CreateCategoryRequest) (*Category, error) {
			return nil, apperrors.NewValidationError("name is required")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":""}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestHandler_Create_Conflict(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, _ *CreateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrConflict
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Alimentação"}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeConflict {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeConflict, apiErr.Code)
	}
}

func TestHandler_Create_Unauthorized(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, _ *CreateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrUnauthorized
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"X"}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandler_Create_InternalError(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, _ *CreateCategoryRequest) (*Category, error) {
			return nil, errors.New("db unavailable")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"X"}`))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestHandler_List_Success(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		ListFn: func(_ context.Context) ([]Category, error) {
			return []Category{*sampleCategory()}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var got []Category
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 item, got %d", len(got))
	}
}

func TestHandler_List_Empty(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		ListFn: func(_ context.Context) ([]Category, error) {
			return []Category{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_List_InternalError(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		ListFn: func(_ context.Context) ([]Category, error) {
			return nil, errors.New("db error")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestHandler_Update_Success(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
			return sampleCategory(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/categories/1", strings.NewReader(`{"name":"Novo"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_Update_InvalidID(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{})

	req := httptest.NewRequest(http.MethodPut, "/categories/abc", strings.NewReader(`{"name":"X"}`))
	req = withChiID(req, "abc")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_Update_NotFound(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/categories/99", strings.NewReader(`{"name":"X"}`))
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_Update_ForbiddenGlobal(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		UpdateFn: func(_ context.Context, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrForbidden
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/categories/1", strings.NewReader(`{"name":"X"}`))
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeForbidden {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeForbidden, apiErr.Code)
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{})

	req := httptest.NewRequest(http.MethodPut, "/categories/1", strings.NewReader(`{bad}`))
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
	h := NewCategoryHandler(&mockCategoryService{
		DeleteFn: func(_ context.Context, _ int64) error { return nil },
	})

	req := httptest.NewRequest(http.MethodDelete, "/categories/1", nil)
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		DeleteFn: func(_ context.Context, _ int64) error { return apperrors.ErrNotFound },
	})

	req := httptest.NewRequest(http.MethodDelete, "/categories/99", nil)
	req = withChiID(req, "99")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_Delete_ForbiddenGlobal(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		DeleteFn: func(_ context.Context, _ int64) error { return apperrors.ErrForbidden },
	})

	req := httptest.NewRequest(http.MethodDelete, "/categories/1", nil)
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandler_Delete_ConflictInUse(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{
		DeleteFn: func(_ context.Context, _ int64) error { return apperrors.ErrConflict },
	})

	req := httptest.NewRequest(http.MethodDelete, "/categories/1", nil)
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
	apiErr := decodeError(t, rec.Body)
	if apiErr.Code != httputil.ErrCodeConflict {
		t.Errorf("expected code %q, got %q", httputil.ErrCodeConflict, apiErr.Code)
	}
}

func TestHandler_Delete_InvalidID(t *testing.T) {
	h := NewCategoryHandler(&mockCategoryService{})

	req := httptest.NewRequest(http.MethodDelete, "/categories/0", nil)
	req = withChiID(req, "0")
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
