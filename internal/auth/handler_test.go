package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	meFn       func(context.Context) (*UserResponse, error)
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

func (m *mockAuthService) Me(ctx context.Context) (*UserResponse, error) {
	return m.meFn(ctx)
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
	handler := NewHandler(&mockAuthService{
		registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) {
			return &RegisterResponse{ID: 1, Email: "user@example.com"}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"name":"Ana Souza","email":"user@example.com","password":"password123"}`))
	recorder := httptest.NewRecorder()
	handler.Register(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
}

func TestAuthHandlerLoginInvalidCredentials(t *testing.T) {
	handler := NewHandler(&mockAuthService{
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
	handler := NewHandler(&mockAuthService{})
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
		"validation":      {body: `{"name":"Ana Souza","email":"x"}`, err: apperrors.NewValidationError("email is invalid"), status: http.StatusUnprocessableEntity, code: httputil.ErrCodeValidation},
		"duplicate email": {body: `{"name":"Ana Souza","email":"user@example.com","password":"password123"}`, err: apperrors.ErrConflict, status: http.StatusConflict, code: httputil.ErrCodeConflict},
		"internal error":  {body: `{"name":"Ana Souza","email":"user@example.com","password":"password123"}`, err: errors.New("pq: connection refused"), status: http.StatusInternalServerError, code: httputil.ErrCodeInternalServerError},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(&mockAuthService{
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
	handler := NewHandler(&mockAuthService{
		registerFn: func(_ context.Context, req *RegisterRequest) (*RegisterResponse, error) {
			if req.Name != "Ana Souza" {
				t.Fatalf("request name = %q, want %q", req.Name, "Ana Souza")
			}
			return &RegisterResponse{ID: 42, Name: "Ana Souza", Email: "user@example.com"}, nil
		},
	})
	recorder := serve(handler.Register, `{"name":"Ana Souza","email":"user@example.com","password":"password123"}`)
	var response RegisterResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusCreated || response.ID != 42 || response.Name != "Ana Souza" || response.Email != "user@example.com" {
		t.Fatalf("status=%d response=%+v", recorder.Code, response)
	}
}

func TestHandler_Register_NameValidation(t *testing.T) {
	handler := NewHandler(&mockAuthService{
		registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) {
			return nil, apperrors.NewValidationError("name must be between 2 and 60 characters")
		},
	})
	response := assertAPIError(t, serve(handler.Register, `{"email":"ana@exemplo.com","password":"Senha123!"}`),
		http.StatusUnprocessableEntity, httputil.ErrCodeValidation)
	if response.Message != "name must be between 2 and 60 characters" {
		t.Fatalf("message = %q", response.Message)
	}
}

func TestHandler_Register_NameWrongType(t *testing.T) {
	handler := NewHandler(&mockAuthService{
		registerFn: func(context.Context, *RegisterRequest) (*RegisterResponse, error) {
			t.Fatal("service must not be called")
			return nil, nil
		},
	})
	assertAPIError(t, serve(handler.Register, `{"name":123,"email":"a@b.co","password":"Senha123!"}`),
		http.StatusBadRequest, httputil.ErrCodeBadRequest)
}

func TestAuthHandler_Login_Success(t *testing.T) {
	handler := NewHandler(&mockAuthService{
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
			handler := NewHandler(&mockAuthService{
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
	handler := NewHandler(&mockAuthService{
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
	handler := NewHandler(&mockAuthService{
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
			handler := NewHandler(&mockAuthService{
				refreshFn: func(context.Context, *RefreshRequest) (*TokenResponse, error) { return nil, tc.err },
			})
			assertAPIError(t, serve(handler.Refresh, tc.body), tc.status, tc.code)
		})
	}
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	var received string
	handler := NewHandler(&mockAuthService{
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
			handler := NewHandler(&mockAuthService{
				logoutFn: func(context.Context, *RefreshRequest) error { return tc.err },
			})
			assertAPIError(t, serve(handler.Logout, tc.body), tc.status, tc.code)
		})
	}
}

func TestHandler_Me_Success(t *testing.T) {
	handler := NewHandler(&mockAuthService{
		meFn: func(context.Context) (*UserResponse, error) {
			return &UserResponse{ID: 42, Name: "Ana Souza", Email: "ana@exemplo.com"}, nil
		},
	})
	recorder := httptest.NewRecorder()
	handler.Me(recorder, httptest.NewRequest(http.MethodGet, "/users/me", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if ct := recorder.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": float64(42), "name": "Ana Souza", "email": "ana@exemplo.com"}
	if len(body) != len(want) {
		t.Fatalf("body keys = %v, want exactly id, name, email", body)
	}
	for key, value := range want {
		if body[key] != value {
			t.Fatalf("body[%q] = %v, want %v", key, body[key], value)
		}
	}
}

func TestHandler_Me_Errors(t *testing.T) {
	tests := map[string]struct {
		err     error
		status  int
		code    string
		message string
	}{
		"unauthorized":      {apperrors.ErrUnauthorized, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, ""},
		"user not found":    {fmt.Errorf("user %w", apperrors.ErrNotFound), http.StatusNotFound, httputil.ErrCodeNotFound, "user not found"},
		"wrapped not found": {fmt.Errorf("authService.Me: %w", apperrors.ErrNotFound), http.StatusNotFound, httputil.ErrCodeNotFound, "user not found"},
		"internal error":    {errors.New("pq: connection refused"), http.StatusInternalServerError, httputil.ErrCodeInternalServerError, "internal server error"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(&mockAuthService{
				meFn: func(context.Context) (*UserResponse, error) { return nil, tc.err },
			})
			recorder := httptest.NewRecorder()
			handler.Me(recorder, httptest.NewRequest(http.MethodGet, "/users/me", nil))
			response := assertAPIError(t, recorder, tc.status, tc.code)
			if tc.message != "" && response.Message != tc.message {
				t.Fatalf("message = %q, want %q", response.Message, tc.message)
			}
			if strings.Contains(response.Message, "pq:") {
				t.Fatalf("internal error leaked to client: %q", response.Message)
			}
		})
	}
}
