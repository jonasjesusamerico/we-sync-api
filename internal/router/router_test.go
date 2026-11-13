package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
	"github.com/jonasjesusamerico/we-sync-api/internal/purchase"
)

// testJWTSecret is a fake 32-byte secret used only by these tests.
const testJWTSecret = "router-test-secret-not-for-prod!"

// stubAuthService implements the auth handler's service interface. Only Me is
// expected to be reached; it echoes the user id found in the request context.
type stubAuthService struct {
	t        *testing.T
	meCalled bool
}

func (s *stubAuthService) Register(context.Context, *auth.RegisterRequest) (*auth.RegisterResponse, error) {
	s.t.Fatal("Register must not be called")
	return nil, nil
}

func (s *stubAuthService) Login(context.Context, *auth.LoginRequest) (*auth.TokenResponse, error) {
	s.t.Fatal("Login must not be called")
	return nil, nil
}

func (s *stubAuthService) Refresh(context.Context, *auth.RefreshRequest) (*auth.TokenResponse, error) {
	s.t.Fatal("Refresh must not be called")
	return nil, nil
}

func (s *stubAuthService) Logout(context.Context, *auth.RefreshRequest) error {
	s.t.Fatal("Logout must not be called")
	return nil
}

func (s *stubAuthService) Me(ctx context.Context) (*auth.UserResponse, error) {
	s.meCalled = true
	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		s.t.Fatal("Me called without a user in the context")
	}
	return &auth.UserResponse{ID: userID, Name: "Ana Souza", Email: "ana@exemplo.com"}, nil
}

// allowAllSessions is a session-version stub for tokens without "sv": every user exists with version 0.
type allowAllSessions struct{}

func (allowAllSessions) SessionVersion(context.Context, int64) (int64, bool, error) {
	return 0, true, nil
}

// fixedSessions answers every lookup with the same version.
type fixedSessions struct{ version int64 }

func (s fixedSessions) SessionVersion(context.Context, int64) (int64, bool, error) {
	return s.version, true, nil
}

func (s *stubAuthService) UpdateProfile(context.Context, *auth.UpdateProfileRequest) (*auth.UserResponse, error) {
	s.t.Fatal("UpdateProfile must not be called")
	return nil, nil
}

func (s *stubAuthService) UpdatePreferences(context.Context, *auth.UpdatePreferencesRequest) (*auth.UserResponse, error) {
	s.t.Fatal("UpdatePreferences must not be called")
	return nil, nil
}

func (s *stubAuthService) ChangePassword(context.Context, *auth.ChangePasswordRequest) (*auth.TokenResponse, error) {
	s.t.Fatal("ChangePassword must not be called")
	return nil, nil
}

func (s *stubAuthService) DeleteAccount(context.Context, *auth.DeleteAccountRequest) error {
	s.t.Fatal("DeleteAccount must not be called")
	return nil
}

func newTestRouter(t *testing.T) (http.Handler, *stubAuthService) {
	t.Helper()
	stub := &stubAuthService{t: t}
	h := New(
		Handlers{Auth: auth.NewHandler(stub)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
		false,
		false,
		[]string{"http://localhost:3000"},
		[]byte(testJWTSecret),
		allowAllSessions{},
	)
	return h, stub
}

func validToken(t *testing.T, subject string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    auth.JWTIssuer,
		Audience:  jwt.ClaimStrings{auth.JWTAudience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) httputil.APIError {
	t.Helper()
	var body httputil.APIError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRouter_UsersMe_RequiresToken(t *testing.T) {
	h, stub := newTestRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body.Message != "missing authentication token" {
		t.Fatalf("message = %q", body.Message)
	}
	if stub.meCalled {
		t.Fatal("Me must not be called without a token")
	}
}

func TestRouter_UsersMe_InvalidToken(t *testing.T) {
	h, stub := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body.Message != "invalid authentication token" {
		t.Fatalf("message = %q", body.Message)
	}
	if stub.meCalled {
		t.Fatal("Me must not be called with an invalid token")
	}
}

func TestRouter_UsersMe_ValidToken(t *testing.T) {
	h, stub := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body auth.UserResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !stub.meCalled || body.ID != 42 {
		t.Fatalf("meCalled=%v id=%d, want the id from the token (42)", stub.meCalled, body.ID)
	}
}

func TestRouter_UsersMe_SessionVersionMismatch(t *testing.T) {
	stub := &stubAuthService{t: t}
	h := New(
		Handlers{Auth: auth.NewHandler(stub)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
		false,
		false,
		[]string{"http://localhost:3000"},
		[]byte(testJWTSecret),
		fixedSessions{version: 1},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42")) // no "sv": version 0
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body.Message != "invalid authentication token" {
		t.Fatalf("message = %q", body.Message)
	}
	if stub.meCalled {
		t.Fatal("Me must not be called for a session that ended")
	}
}

func TestRouter_UsersMe_MethodNotAllowed(t *testing.T) {
	h, stub := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if stub.meCalled {
		t.Fatal("Me must not be called for POST")
	}
}

// stubPurchaseService implements the purchase handler's service interface. Only Summary and
// Overview are expected to be reached; they record the user id found in the request context.
type stubPurchaseService struct {
	t              *testing.T
	summaryCalled  bool
	summaryUserID  int64
	overviewCalled bool
	overviewUserID int64
}

func (s *stubPurchaseService) Create(context.Context, *purchase.CreatePurchaseRequest) (*purchase.Purchase, error) {
	s.t.Fatal("Create must not be called")
	return nil, nil
}

func (s *stubPurchaseService) GetByID(context.Context, int64) (*purchase.Purchase, error) {
	s.t.Fatal("GetByID must not be called: /summary and /overview must not fall into /{id}")
	return nil, nil
}

func (s *stubPurchaseService) List(context.Context, purchase.ListFilter) (*purchase.PaginatedResult, error) {
	s.t.Fatal("List must not be called")
	return nil, nil
}

func (s *stubPurchaseService) Update(context.Context, int64, *purchase.UpdatePurchaseRequest) (*purchase.Purchase, error) {
	s.t.Fatal("Update must not be called")
	return nil, nil
}

func (s *stubPurchaseService) Delete(context.Context, int64) error {
	s.t.Fatal("Delete must not be called")
	return nil
}

func (s *stubPurchaseService) Summary(ctx context.Context, year, month int, _ time.Time) (*purchase.MonthSummary, error) {
	s.summaryCalled = true
	s.summaryUserID, _ = identity.UserIDFromContext(ctx)
	return &purchase.MonthSummary{Year: year, Month: month}, nil
}

func (s *stubPurchaseService) Overview(ctx context.Context, year, month int, _ time.Time) (*purchase.MonthOverview, error) {
	s.overviewCalled = true
	s.overviewUserID, _ = identity.UserIDFromContext(ctx)
	return &purchase.MonthOverview{Year: year, Month: month}, nil
}

func newPurchaseTestRouter(t *testing.T) (http.Handler, *stubPurchaseService) {
	t.Helper()
	stub := &stubPurchaseService{t: t}
	h := New(
		Handlers{Purchase: purchase.NewHandler(stub)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
		false,
		false,
		[]string{"http://localhost:3000"},
		[]byte(testJWTSecret),
		allowAllSessions{},
	)
	return h, stub
}

func TestRouter_PurchasesSummary_RoutesToSummary(t *testing.T) {
	h, stub := newPurchaseTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/purchases/summary?year=2026&month=11&today=2026-11-10", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !stub.summaryCalled || stub.summaryUserID != 42 {
		t.Fatalf("summaryCalled=%v userID=%d, want the id from the token (42)", stub.summaryCalled, stub.summaryUserID)
	}
}

func TestRouter_PurchasesSummary_RequiresToken(t *testing.T) {
	h, stub := newPurchaseTestRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/purchases/summary?year=2026&month=11&today=2026-11-10", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body.Message != "missing authentication token" {
		t.Fatalf("message = %q", body.Message)
	}
	if stub.summaryCalled {
		t.Fatal("Summary must not be called without a token")
	}
}

func TestRouter_PurchasesOverview_RoutesToOverview(t *testing.T) {
	h, stub := newPurchaseTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/purchases/overview?year=2026&month=11&today=2026-11-10", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !stub.overviewCalled || stub.overviewUserID != 42 {
		t.Fatalf("overviewCalled=%v userID=%d, want the id from the token (42)", stub.overviewCalled, stub.overviewUserID)
	}
}

func TestRouter_PurchasesOverview_RequiresToken(t *testing.T) {
	h, stub := newPurchaseTestRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/purchases/overview?year=2026&month=11&today=2026-11-10", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body := decodeError(t, rec); body.Message != "missing authentication token" {
		t.Fatalf("message = %q", body.Message)
	}
	if stub.overviewCalled {
		t.Fatal("Overview must not be called without a token")
	}
}

func TestRouter_UsersMeProfile_RequiresToken(t *testing.T) {
	h, _ := newTestRouter(t)
	for _, path := range []string{"/api/v1/users/me/profile", "/api/v1/users/me/preferences", "/api/v1/users/me/password"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("PUT %s status = %d, want 401", path, rec.Code)
		}
	}
}

func TestRouter_UsersMeDelete_RequiresToken(t *testing.T) {
	h, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/me", strings.NewReader(`{"password":"x"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// stubCategoryService implements the category handler's service interface. Only Archive and
// Unarchive are expected to be reached; they record the id they received.
type stubCategoryService struct {
	t          *testing.T
	archived   []int64
	unarchived []int64
}

func (s *stubCategoryService) Create(context.Context, *category.CreateCategoryRequest) (*category.Category, bool, error) {
	s.t.Fatal("Create must not be called")
	return nil, false, nil
}

func (s *stubCategoryService) List(context.Context) ([]category.Category, error) {
	s.t.Fatal("List must not be called")
	return nil, nil
}

func (s *stubCategoryService) Update(context.Context, int64, *category.UpdateCategoryRequest) (*category.Category, error) {
	s.t.Fatal("Update must not be called")
	return nil, nil
}

func (s *stubCategoryService) Archive(_ context.Context, id int64) error {
	s.archived = append(s.archived, id)
	return nil
}

func (s *stubCategoryService) Unarchive(_ context.Context, id int64) error {
	s.unarchived = append(s.unarchived, id)
	return nil
}

func newCategoryTestRouter(t *testing.T) (http.Handler, *stubCategoryService) {
	t.Helper()
	stub := &stubCategoryService{t: t}
	h := New(
		Handlers{Category: category.NewHandler(stub)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
		false,
		false,
		[]string{"http://localhost:3000"},
		[]byte(testJWTSecret),
		allowAllSessions{},
	)
	return h, stub
}

func TestRouter_CategoriesDelete_MethodNotAllowed(t *testing.T) {
	h, stub := newCategoryTestRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/categories/5", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if len(stub.archived) != 0 || len(stub.unarchived) != 0 {
		t.Fatalf("no service method must be called: archived=%v unarchived=%v", stub.archived, stub.unarchived)
	}
}

func TestRouter_CategoriesArchive_Routes(t *testing.T) {
	h, stub := newCategoryTestRouter(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/categories/5/archive", nil)
		req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204 (body: %s)", method, rec.Code, rec.Body.String())
		}
	}
	if len(stub.archived) != 1 || stub.archived[0] != 5 {
		t.Fatalf("archived = %v, want [5]", stub.archived)
	}
	if len(stub.unarchived) != 1 || stub.unarchived[0] != 5 {
		t.Fatalf("unarchived = %v, want [5]", stub.unarchived)
	}
}

// stubPaymentMethodService implements the payment method handler's service interface. Only Archive and
// Unarchive are expected to be reached; they record the id they received.
type stubPaymentMethodService struct {
	t          *testing.T
	archived   []int64
	unarchived []int64
}

func (s *stubPaymentMethodService) Create(context.Context, *paymentmethod.CreatePaymentMethodRequest) (*paymentmethod.PaymentMethod, bool, error) {
	s.t.Fatal("Create must not be called")
	return nil, false, nil
}

func (s *stubPaymentMethodService) List(context.Context) ([]paymentmethod.PaymentMethod, error) {
	s.t.Fatal("List must not be called")
	return nil, nil
}

func (s *stubPaymentMethodService) Update(context.Context, int64, *paymentmethod.UpdatePaymentMethodRequest) (*paymentmethod.PaymentMethod, error) {
	s.t.Fatal("Update must not be called")
	return nil, nil
}

func (s *stubPaymentMethodService) Archive(_ context.Context, id int64) error {
	s.archived = append(s.archived, id)
	return nil
}

func (s *stubPaymentMethodService) Unarchive(_ context.Context, id int64) error {
	s.unarchived = append(s.unarchived, id)
	return nil
}

func newPaymentMethodTestRouter(t *testing.T) (http.Handler, *stubPaymentMethodService) {
	t.Helper()
	stub := &stubPaymentMethodService{t: t}
	h := New(
		Handlers{PaymentMethod: paymentmethod.NewHandler(stub)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
		false,
		false,
		[]string{"http://localhost:3000"},
		[]byte(testJWTSecret),
		allowAllSessions{},
	)
	return h, stub
}

func TestRouter_PaymentMethodsDelete_MethodNotAllowed(t *testing.T) {
	h, stub := newPaymentMethodTestRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/payment-methods/5", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if len(stub.archived) != 0 || len(stub.unarchived) != 0 {
		t.Fatalf("no service method must be called: archived=%v unarchived=%v", stub.archived, stub.unarchived)
	}
}

func TestRouter_PaymentMethodsArchive_Routes(t *testing.T) {
	h, stub := newPaymentMethodTestRouter(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/payment-methods/5/archive", nil)
		req.Header.Set("Authorization", "Bearer "+validToken(t, "42"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204 (body: %s)", method, rec.Code, rec.Body.String())
		}
	}
	if len(stub.archived) != 1 || stub.archived[0] != 5 {
		t.Fatalf("archived = %v, want [5]", stub.archived)
	}
	if len(stub.unarchived) != 1 || stub.unarchived[0] != 5 {
		t.Fatalf("unarchived = %v, want [5]", stub.unarchived)
	}
}
