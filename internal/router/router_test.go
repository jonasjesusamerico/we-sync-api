package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
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
