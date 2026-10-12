package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// User represents a registered user.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RegisterRequest is the payload accepted by POST /auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the payload accepted by POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest is the payload accepted by POST /auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse is returned after a successful login or token refresh.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// RegisterResponse is returned after a successful user registration.
type RegisterResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// RefreshToken represents a refresh_tokens database record.
// TokenHash contains the SHA-256 hex digest of the raw UUID token.
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}

// Claims holds the registered claims of an access JWT.
type Claims struct {
	jwt.RegisteredClaims
}

const (
	JWTIssuer   = "we-sync-api"
	JWTAudience = "we-sync-api"
)
