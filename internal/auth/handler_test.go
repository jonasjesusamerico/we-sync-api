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
			return nil, errors.Join(apperrors.ErrUnauthorized, errors.New("invalid credentials"))
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

func serve(handlerFn http.HandlerFunc, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handlerFn(recorder, request)
	return recorder
}

func assertAPIError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) httputil.APIError {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, status, recorder.Body.String())
	}
	response := decodeAPIError(t, recorder)
	if response.Code != code {
		t.Fatalf("code = %q, want %q", response.Code, code)
	}
	return response
}

func TestAuthHandler_Register_Errors(t *testing.T) {
	tests := map[string]struct {
		body   string
		err    error
		status int
		code   string
	}{
		"malformed body":  {body: `{`, status: http.StatusBadRequest, code: httputil.ErrCodeBadRequest},
		"validation":      {body: `{"email":"x"}`, err: apperrors.NewValidationError("email is invalid"), status: http.StatusUnprocessableEntity, code: httputil.ErrCodeValidation},
		"duplicate email": {body: `{"email":"user@example.com","password":"password123"}`, err: apperrors.ErrConflict, status: http.StatusConflict, code: httputil.ErrCodeConflict},
		"internal error":  {body: `{"email":"user@example.com","password":"password123"}`, err: errors.New("pq: connection refused"), status: http.StatusInternalServerError, code: httputil.ErrCodeInternalServerError},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{
				registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) { return nil, tc.err },
			})
			response := assertAPIError(t, serve(handler.Register, tc.body), tc.status, tc.code)
			if strings.Contains(response.Message, "pq:") {
				t.Fatalf("internal error leaked to client: %q", response.Message)
			}
		})
	}
}

func TestAuthHandler_Register_SuccessBody(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) {
			return &RegisterResponse{ID: 42, Email: "user@example.com"}, nil
		},
	})
	recorder := serve(handler.Register, `{"email":"user@example.com","password":"password123"}`)
	var response RegisterResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusCreated || response.ID != 42 || response.Email != "user@example.com" {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		loginFn: func(_ context.Context, req *LoginRequest) (*TokenResponse, error) {
			if req.Email != "user@example.com" || req.Password != "password123" {
				t.Fatalf("unexpected request %+v", req)
			}
			return &TokenResponse{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer"}, nil
		},
	})
	recorder := serve(handler.Login, `{"email":"user@example.com","password":"password123"}`)
	var response TokenResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || response.AccessToken != "access" || response.RefreshToken != "refresh" || response.TokenType != "Bearer" {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}

func TestAuthHandler_Login_InputErrors(t *testing.T) {
	tests := map[string]struct {
		body   string
		status int
		code   string
	}{
		"malformed body": {body: `not-json`, status: http.StatusBadRequest, code: httputil.ErrCodeBadRequest},
		"missing fields": {body: `{}`, status: http.StatusUnprocessableEntity, code: httputil.ErrCodeValidation},
		"empty password": {body: `{"email":"user@example.com","password":""}`, status: http.StatusUnprocessableEntity, code: httputil.ErrCodeValidation},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{
				loginFn: func(context.Context, *LoginRequest) (*TokenResponse, error) {
					t.Fatal("service must not be called")
					return nil, nil
				},
			})
			assertAPIError(t, serve(handler.Login, tc.body), tc.status, tc.code)
		})
	}
}

func TestAuthHandler_Login_InvalidCredentialsMessage(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		loginFn: func(context.Context, *LoginRequest) (*TokenResponse, error) {
			return nil, newInvalidCredentialsError()
		},
	})
	response := assertAPIError(t, serve(handler.Login, `{"email":"user@example.com","password":"wrongpass"}`),
		http.StatusUnauthorized, httputil.ErrCodeUnauthorized)
	if response.Message != "invalid credentials" {
		t.Fatalf("message = %q", response.Message)
	}
}

func TestAuthHandler_Refresh_Success(t *testing.T) {
	handler := NewAuthHandler(&mockAuthService{
		refreshFn: func(_ context.Context, req *RefreshRequest) (*TokenResponse, error) {
			if req.RefreshToken != "old" {
				t.Fatalf("refresh token = %q", req.RefreshToken)
			}
			return &TokenResponse{AccessToken: "access", RefreshToken: "new", TokenType: "Bearer"}, nil
		},
	})
	recorder := serve(handler.Refresh, `{"refresh_token":"old"}`)
	var response TokenResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || response.RefreshToken != "new" || response.AccessToken != "access" {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}

func TestAuthHandler_Refresh_Errors(t *testing.T) {
	tests := map[string]struct {
		body   string
		err    error
		status int
		code   string
	}{
		"malformed body": {body: `{`, status: http.StatusBadRequest, code: httputil.ErrCodeBadRequest},
		"invalid token":  {body: `{"refresh_token":"x"}`, err: newInvalidCredentialsError(), status: http.StatusUnauthorized, code: httputil.ErrCodeUnauthorized},
		"internal error": {body: `{"refresh_token":"x"}`, err: errors.New("db down"), status: http.StatusInternalServerError, code: httputil.ErrCodeInternalServerError},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{
				refreshFn: func(context.Context, *RefreshRequest) (*TokenResponse, error) { return nil, tc.err },
			})
			assertAPIError(t, serve(handler.Refresh, tc.body), tc.status, tc.code)
		})
	}
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	var received string
	handler := NewAuthHandler(&mockAuthService{
		logoutFn: func(_ context.Context, req *RefreshRequest) error {
			received = req.RefreshToken
			return nil
		},
	})
	recorder := serve(handler.Logout, `{"refresh_token":"token"}`)
	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 || received != "token" {
		t.Fatalf("status=%d body=%q received=%q", recorder.Code, recorder.Body.String(), received)
	}
}

func TestAuthHandler_Logout_Errors(t *testing.T) {
	tests := map[string]struct {
		body   string
		err    error
		status int
		code   string
	}{
		"malformed body": {body: `{`, status: http.StatusBadRequest, code: httputil.ErrCodeBadRequest},
		"missing token":  {body: `{}`, err: apperrors.NewValidationError("refresh_token is required"), status: http.StatusUnprocessableEntity, code: httputil.ErrCodeValidation},
		"internal error": {body: `{"refresh_token":"x"}`, err: errors.New("db down"), status: http.StatusInternalServerError, code: httputil.ErrCodeInternalServerError},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{
				logoutFn: func(context.Context, *RefreshRequest) error { return tc.err },
			})
			assertAPIError(t, serve(handler.Logout, tc.body), tc.status, tc.code)
		})
	}
}
