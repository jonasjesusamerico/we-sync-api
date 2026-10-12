package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
)

type mockAuthService struct {
	registerFn func(context.Context, *RegisterRequest) (*RegisterResponse, error)
	loginFn    func(context.Context, *LoginRequest) (*TokenResponse, error)
	refreshFn  func(context.Context, *RefreshRequest) (*TokenResponse, error)
	logoutFn   func(context.Context, *RefreshRequest) error
}

func (m *mockAuthService) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	return m.registerFn(ctx, req)
}
func (m *mockAuthService) Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error) {
	return m.loginFn(ctx, req)
}
func (m *mockAuthService) Refresh(ctx context.Context, req *RefreshRequest) (*TokenResponse, error) {
	return m.refreshFn(ctx, req)
}
func (m *mockAuthService) Logout(ctx context.Context, req *RefreshRequest) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, req)
	}
	return nil
}

func decodeAPIError(t *testing.T, recorder *httptest.ResponseRecorder) httputil.APIError {
	t.Helper()
	var response httputil.APIError
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestAuthHandler_RegisterSuccess(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) {
			return &RegisterResponse{ID: 1, Email: "user@example.com"}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"email":"user@example.com","password":"password123"}`))
	recorder := httptest.NewRecorder()
	handler.Register(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
}

func TestAuthHandlerLoginInvalidCredentials(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		loginFn: func(context.Context, *LoginRequest) (*TokenResponse, error) {
			return nil, errors.Join(apperrors.ErrUnauthorized, errors.New("credenciais inválidas"))
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"user@example.com","password":"wrongpass"}`))
	recorder := httptest.NewRecorder()
	handler.Login(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	response := decodeAPIError(t, recorder)
	if response.Code != httputil.ErrCodeUnauthorized {
		t.Fatalf("code = %q, want %q", response.Code, httputil.ErrCodeUnauthorized)
	}
}

func TestAuthHandlerRefreshMissingToken(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{})
	request := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	handler.Refresh(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}
