package category

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

type mockCategoryService struct {
	CreateFn    func(ctx context.Context, req *CreateCategoryRequest) (*Category, bool, error)
	ListFn      func(ctx context.Context) ([]Category, error)
	UpdateFn    func(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error)
	ArchiveFn   func(ctx context.Context, id int64) error
	UnarchiveFn func(ctx context.Context, id int64) error
}

func (m *mockCategoryService) Create(ctx context.Context, req *CreateCategoryRequest) (*Category, bool, error) {
	if m.CreateFn == nil {
		return nil, false, nil
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
func (m *mockCategoryService) Archive(ctx context.Context, id int64) error {
	if m.ArchiveFn == nil {
		return nil
	}
	return m.ArchiveFn(ctx, id)
}
func (m *mockCategoryService) Unarchive(ctx context.Context, id int64) error {
	if m.UnarchiveFn == nil {
		return nil
	}
	return m.UnarchiveFn(ctx, id)
}

func sampleCategory() *Category {
	uid := int64(42)
	return &Category{
		ID:         1,
		UserID:     &uid,
		Name:       "Pets",
		IsGlobal:   false,
		Icon:       IconHeart,
		Color:      ColorLeisure,
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

const petsBody = `{"name":"Pets","icon":"HEART","color":"LEISURE"}`

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestHandler_Create_Created(t *testing.T) {
	var got CreateCategoryRequest
	h := NewHandler(&mockCategoryService{
		CreateFn: func(_ context.Context, req *CreateCategoryRequest) (*Category, bool, error) {
			got = *req
			return sampleCategory(), true, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(petsBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if got != (CreateCategoryRequest{Name: "Pets", Icon: IconHeart, Color: ColorLeisure}) {
		t.Fatalf("service received %+v", got)
	}
	body := rec.Body.String()
	for _, want := range []string{`"icon":"HEART"`, `"color":"LEISURE"`, `"archived_at":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s does not contain %s", body, want)
		}
	}
}

func TestHandler_Create_Reactivated(t *testing.T) {
	h := NewHandler(&mockCategoryService{
		CreateFn: func(context.Context, *CreateCategoryRequest) (*Category, bool, error) {
			return sampleCategory(), false, nil
		},
	})

	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(petsBody)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got Category
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != 1 || got.ArchivedAt != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockCategoryService{
		CreateFn: func(context.Context, *CreateCategoryRequest) (*Category, bool, error) {
			t.Fatal("service must not be called")
			return nil, false, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{bad}`))
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
	h := NewHandler(&mockCategoryService{
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
	h := NewHandler(&mockCategoryService{})

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/categories", nil))

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("expected 200 with [], got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_List_InternalError(t *testing.T) {
	h := NewHandler(&mockCategoryService{
		ListFn: func(_ context.Context) ([]Category, error) {
			return nil, errors.New("db error")
		},
	})

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/categories", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestHandler_Update_Success(t *testing.T) {
	var gotID int64
	var got UpdateCategoryRequest
	h := NewHandler(&mockCategoryService{
		UpdateFn: func(_ context.Context, id int64, req *UpdateCategoryRequest) (*Category, error) {
			gotID, got = id, *req
			return sampleCategory(), nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/categories/1",
		strings.NewReader(`{"name":"Animais","icon":"HOUSE","color":"HOME"}`))
	req = withChiID(req, "1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotID != 1 || got != (UpdateCategoryRequest{Name: "Animais", Icon: IconHouse, Color: ColorHome}) {
		t.Fatalf("service received id=%d req=%+v", gotID, got)
	}
}

func TestHandler_Update_InvalidID(t *testing.T) {
	h := NewHandler(&mockCategoryService{})

	req := withChiID(httptest.NewRequest(http.MethodPut, "/categories/abc", strings.NewReader(petsBody)), "abc")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockCategoryService{})

	req := withChiID(httptest.NewRequest(http.MethodPut, "/categories/1", strings.NewReader(`{bad}`)), "1")
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
	h := NewHandler(&mockCategoryService{
		ArchiveFn: func(_ context.Context, id int64) error { gotID = id; return nil },
	})

	rec := httptest.NewRecorder()
	h.Archive(rec, withChiID(httptest.NewRequest(http.MethodPut, "/categories/5/archive", nil), "5"))

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("expected 204 with an empty body, got %d %q", rec.Code, rec.Body.String())
	}
	if gotID != 5 {
		t.Fatalf("service received id %d, want 5", gotID)
	}
}

func TestHandler_Unarchive_Success(t *testing.T) {
	var gotID int64
	h := NewHandler(&mockCategoryService{
		UnarchiveFn: func(_ context.Context, id int64) error { gotID = id; return nil },
	})

	rec := httptest.NewRecorder()
	h.Unarchive(rec, withChiID(httptest.NewRequest(http.MethodDelete, "/categories/5/archive", nil), "5"))

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
	h := NewHandler(&mockCategoryService{ArchiveFn: fail, UnarchiveFn: fail})
	for _, id := range []string{"0", "abc"} {
		for _, op := range []http.HandlerFunc{h.Archive, h.Unarchive} {
			rec := httptest.NewRecorder()
			op(rec, withChiID(httptest.NewRequest(http.MethodPut, "/categories/"+id+"/archive", nil), id))
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
		{apperrors.ErrForbidden, http.StatusForbidden, httputil.ErrCodeForbidden, "default categories cannot be changed"},
		{apperrors.ErrNotFound, http.StatusNotFound, httputil.ErrCodeNotFound, "category not found"},
		{ErrDefaultNameConflict, http.StatusConflict, httputil.ErrCodeDefaultNameConflict, "name already used by a default category"},
		{ErrArchivedNameConflict, http.StatusConflict, httputil.ErrCodeArchivedNameConflict, "name already used by an archived category"},
		{ErrNameConflict, http.StatusConflict, httputil.ErrCodeNameConflict, "name already used by another category"},
		{apperrors.ErrConflict, http.StatusConflict, httputil.ErrCodeNameConflict, "name already used by another category"},
		{apperrors.NewValidationError("name is required"), http.StatusUnprocessableEntity, httputil.ErrCodeValidation, "name is required"},
		{errors.New("db unavailable"), http.StatusInternalServerError, httputil.ErrCodeInternalServerError, "internal server error"},
	}
	for _, e := range errs {
		h := NewHandler(&mockCategoryService{
			CreateFn:    func(context.Context, *CreateCategoryRequest) (*Category, bool, error) { return nil, false, e.err },
			UpdateFn:    func(context.Context, int64, *UpdateCategoryRequest) (*Category, error) { return nil, e.err },
			ArchiveFn:   func(context.Context, int64) error { return e.err },
			UnarchiveFn: func(context.Context, int64) error { return e.err },
		})
		ops := []struct {
			name string
			call func(*httptest.ResponseRecorder)
		}{
			{"Create", func(rec *httptest.ResponseRecorder) {
				h.Create(rec, httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(petsBody)))
			}},
			{"Update", func(rec *httptest.ResponseRecorder) {
				h.Update(rec, withChiID(httptest.NewRequest(http.MethodPut, "/categories/1", strings.NewReader(petsBody)), "1"))
			}},
			{"Archive", func(rec *httptest.ResponseRecorder) {
				h.Archive(rec, withChiID(httptest.NewRequest(http.MethodPut, "/categories/1/archive", nil), "1"))
			}},
			{"Unarchive", func(rec *httptest.ResponseRecorder) {
				h.Unarchive(rec, withChiID(httptest.NewRequest(http.MethodDelete, "/categories/1/archive", nil), "1"))
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
