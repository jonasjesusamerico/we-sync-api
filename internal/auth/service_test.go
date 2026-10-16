package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

const testSecret = "01234567890123456789012345678901"

func TestAuthService_RegisterHashesPassword(t *testing.T) {
	var stored string
	svc := NewAuthService(&mockAuthRepository{
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
	svc := NewAuthService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) {
			return &User{ID: 1, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	_, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "wrongpass"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || err.Error() != "credenciais inválidas" {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestAuthService_LoginIssuesClaimsAndHashesRefreshToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	var storedHash string
	svc := NewAuthService(&mockAuthRepository{
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
	svc := NewAuthService(&mockAuthRepository{
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
		svc := NewAuthService(&mockAuthRepository{
			GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
		}, []byte(testSecret), time.Minute, time.Hour)
		// Prefixes guarantee non-empty strings, so the early validation for
		// blank email/password is never triggered and the path that reaches
		// the repository (user not found) always returns "credenciais inválidas".
		_, err := svc.Login(context.Background(), &LoginRequest{
			Email:    "user@example-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "email"),
			Password: "wrong-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "password"),
		})
		if err == nil || err.Error() != "credenciais inválidas" {
			t.Fatalf("error = %v", err)
		}
	})
}
