package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

const testSecret = "01234567890123456789012345678901"

func TestAuthService_RegisterHashesPassword(t *testing.T) {
	var stored string
	svc := NewService(&mockAuthRepository{
		CreateUserFn: func(_ context.Context, email, passwordHash string) (*User, error) {
			stored = passwordHash
			return &User{ID: 1, Email: email}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	got, err := svc.Register(context.Background(), &RegisterRequest{Email: "user@example.com", Password: "password123"})
	if err != nil || got.ID != 1 || got.Email != "user@example.com" {
		t.Fatalf("Register() = %+v, %v", got, err)
	}
	if stored == "password123" || bcrypt.CompareHashAndPassword([]byte(stored), []byte("password123")) != nil {
		t.Fatal("password was not stored as a valid bcrypt hash")
	}
}

func TestAuthService_LoginInvalidCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) {
			return &User{ID: 1, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	_, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "wrongpass"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || err.Error() != "invalid credentials" {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestAuthService_LoginIssuesClaimsAndHashesRefreshToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	var storedHash string
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) {
			return &User{ID: 7, PasswordHash: string(hash)}, nil
		},
		CreateRefreshTokenFn: func(_ context.Context, _ int64, tokenHash string, _ time.Time) error {
			storedHash = tokenHash
			return nil
		},
	}, []byte(testSecret), 15*time.Minute, time.Hour)
	response, err := svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if storedHash != sha256Hex(response.RefreshToken) || storedHash == response.RefreshToken {
		t.Fatal("refresh token was not persisted as a SHA-256 hash")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(response.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil || !token.Valid || claims.Subject != "7" {
		t.Fatalf("invalid access token: valid=%v err=%v subject=%q", token.Valid, err, claims.Subject)
	}
	if claims.Issuer != JWTIssuer || len(claims.Audience) != 1 || claims.Audience[0] != JWTAudience {
		t.Fatalf("unexpected issuer/audience: %q %v", claims.Issuer, claims.Audience)
	}
}

func TestAuthService_NormalizesEmailForRegisterAndLogin(t *testing.T) {
	var registeredEmail, loginEmail string
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&mockAuthRepository{
		CreateUserFn: func(_ context.Context, email, _ string) (*User, error) {
			registeredEmail = email
			return &User{ID: 1, Email: email}, nil
		},
		GetUserByEmailFn: func(_ context.Context, email string) (*User, error) {
			loginEmail = email
			return &User{ID: 1, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	if _, err := svc.Register(context.Background(), &RegisterRequest{Email: " User@Example.COM ", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), &LoginRequest{Email: " User@Example.COM ", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if registeredEmail != "user@example.com" || loginEmail != registeredEmail {
		t.Fatalf("emails = %q, %q", registeredEmail, loginEmail)
	}
}

func TestAuthService_ValidatePasswordProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.String().Draw(t, "password")
		err := validatePassword(password)
		if (len(password) < 8 || len(password) > 72) != (err != nil) {
			t.Fatalf("validation mismatch for length %d: %v", len(password), err)
		}
	})
}

func TestAuthService_LoginErrorMessageProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := withMinPasswordCost(NewService(&mockAuthRepository{
			GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
		}, []byte(testSecret), time.Minute, time.Hour))
		// Prefixes guarantee non-empty strings, so the early validation for
		// blank email/password is never triggered and the path that reaches
		// the repository (user not found) always returns "invalid credentials".
		_, err := svc.Login(context.Background(), &LoginRequest{
			Email:    "user@example-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "email"),
			Password: "wrong-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "password"),
		})
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("error = %v", err)
		}
	})
}

func newTestAuthService(repo Repository) *Service {
	return NewService(repo, []byte(testSecret), 15*time.Minute, time.Hour)
}

// withMinPasswordCost lowers the bcrypt cost so tests that hash many times
// (e.g. property tests hitting the unknown-user path) stay fast.
func withMinPasswordCost(svc *Service) *Service {
	svc.passwordCost = bcrypt.MinCost
	return svc
}

func TestAuthService_Login_UnknownUserComparesDummyHash(t *testing.T) {
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
	}, []byte(testSecret), time.Minute, time.Hour)

	_, err := svc.Login(context.Background(), &LoginRequest{Email: "nobody@example.com", Password: "password123"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || err.Error() != "invalid credentials" {
		t.Fatalf("Login() error = %v", err)
	}
	// The dummy hash must cost the same as a stored hash, otherwise timing leaks existence.
	cost, err := bcrypt.Cost(svc.dummyPasswordHash())
	if err != nil {
		t.Fatalf("dummy hash is not a valid bcrypt hash: %v", err)
	}
	if cost != defaultPasswordCost {
		t.Fatalf("dummy hash cost = %d, want %d", cost, defaultPasswordCost)
	}
}

func TestRecordAuthError_RecordsOnlyUnexpectedErrors(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantEvents int
	}{
		"validation":          {apperrors.NewValidationError("email is invalid"), 0},
		"invalid credentials": {newInvalidCredentialsError(), 0},
		"wrapped conflict":    {fmt.Errorf("authService.Register: %w", apperrors.ErrConflict), 0},
		"unexpected":          {errors.New("connection refused"), 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			_, span := provider.Tracer("test").Start(context.Background(), "op")

			if got := recordAuthError(span, tt.err); got != tt.err {
				t.Fatalf("recordAuthError() returned %v, want the same error", got)
			}
			span.End()

			ended := recorder.Ended()[0]
			if ended.Status().Code != codes.Error {
				t.Errorf("status = %v, want Error", ended.Status().Code)
			}
			if len(ended.Events()) != tt.wantEvents {
				t.Errorf("events = %d, want %d", len(ended.Events()), tt.wantEvents)
			}
		})
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		CreateUserFn: func(context.Context, string, string) (*User, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Register(context.Background(), &RegisterRequest{Email: "user@example.com", Password: "password123"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("Register() error = %v, want ErrConflict", err)
	}
}

func TestAuthService_Register_InvalidInput(t *testing.T) {
	tests := map[string]RegisterRequest{
		"invalid email":  {Email: "not-an-email", Password: "password123"},
		"email too long": {Email: strings.Repeat("a", 250) + "@x.io", Password: "password123"},
		"short password": {Email: "user@example.com", Password: "short"},
		"long password":  {Email: "user@example.com", Password: strings.Repeat("a", 73)},
		"empty fields":   {},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			called := false
			svc := newTestAuthService(&mockAuthRepository{
				CreateUserFn: func(context.Context, string, string) (*User, error) {
					called = true
					return &User{ID: 1}, nil
				},
			})
			_, err := svc.Register(context.Background(), &req)
			if !errors.Is(err, apperrors.ErrValidation) {
				t.Fatalf("Register() error = %v, want ErrValidation", err)
			}
			if called {
				t.Fatal("repository must not be called for invalid input")
			}
		})
	}
}

func TestAuthService_Refresh_ValidToken(t *testing.T) {
	const rawToken = "old-refresh-token"
	var rotatedOldID, rotatedUserID int64
	var newHash string
	var newExpiresAt time.Time
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(_ context.Context, tokenHash string) (*RefreshToken, error) {
			if tokenHash != sha256Hex(rawToken) {
				t.Fatalf("lookup hash = %q, want SHA-256 of raw token", tokenHash)
			}
			return &RefreshToken{ID: 10, UserID: 7, TokenHash: tokenHash, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		RotateRefreshTokenFn: func(_ context.Context, oldID, userID int64, hash string, expiresAt time.Time) error {
			rotatedOldID, rotatedUserID, newHash, newExpiresAt = oldID, userID, hash, expiresAt
			return nil
		},
	})

	response, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: rawToken})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rotatedOldID != 10 || rotatedUserID != 7 {
		t.Fatalf("rotated oldID=%d userID=%d, want 10 and 7", rotatedOldID, rotatedUserID)
	}
	if response.RefreshToken == rawToken || newHash != sha256Hex(response.RefreshToken) {
		t.Fatal("new refresh token must differ from the old one and be persisted as its SHA-256 hash")
	}
	if time.Until(newExpiresAt) <= 0 || response.TokenType != "Bearer" {
		t.Fatalf("unexpected expiresAt=%v tokenType=%q", newExpiresAt, response.TokenType)
	}
	claims := &Claims{}
	if _, err := jwt.ParseWithClaims(response.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	}); err != nil || claims.Subject != "7" {
		t.Fatalf("invalid access token: err=%v subject=%q", err, claims.Subject)
	}
}

func TestAuthService_Refresh_InvalidToken(t *testing.T) {
	tests := map[string]*RefreshToken{
		"not found": nil,
		"expired":   {ID: 1, UserID: 7, ExpiresAt: time.Now().Add(-time.Second)},
		"revoked":   {ID: 1, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), Revoked: true},
	}
	for name, stored := range tests {
		t.Run(name, func(t *testing.T) {
			rotated := false
			svc := newTestAuthService(&mockAuthRepository{
				GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) { return stored, nil },
				RotateRefreshTokenFn: func(context.Context, int64, int64, string, time.Time) error {
					rotated = true
					return nil
				},
			})
			_, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
			if !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Fatalf("Refresh() error = %v, want ErrUnauthorized", err)
			}
			if rotated {
				t.Fatal("token must not be rotated")
			}
		})
	}
}

func TestAuthService_Refresh_ConcurrentReuse(t *testing.T) {
	// The repository returns ErrUnauthorized when another request already
	// revoked the token between the lookup and the rotation.
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) {
			return &RefreshToken{ID: 1, UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		RotateRefreshTokenFn: func(context.Context, int64, int64, string, time.Time) error {
			return apperrors.ErrUnauthorized
		},
	})
	response, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || response != nil {
		t.Fatalf("Refresh() = %+v, %v, want nil and ErrUnauthorized", response, err)
	}
}

func TestAuthService_Refresh_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) { return nil, dbErr },
	})
	_, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
	if !errors.Is(err, dbErr) || errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("Refresh() error = %v, want wrapped repository error", err)
	}
}

func TestAuthService_Logout_RevokesTokenHash(t *testing.T) {
	var revokedHash string
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(_ context.Context, tokenHash string) error {
			revokedHash = tokenHash
			return nil
		},
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{RefreshToken: "raw-token"}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if revokedHash != sha256Hex("raw-token") {
		t.Fatalf("revoked hash = %q, want SHA-256 of raw token", revokedHash)
	}
}

func TestAuthService_Logout_MissingToken(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(context.Context, string) error {
			t.Fatal("repository must not be called")
			return nil
		},
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{}); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("Logout() error = %v, want ErrValidation", err)
	}
}

func TestAuthService_Logout_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(context.Context, string) error { return dbErr },
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{RefreshToken: "token"}); !errors.Is(err, dbErr) {
		t.Fatalf("Logout() error = %v, want wrapped repository error", err)
	}
}
