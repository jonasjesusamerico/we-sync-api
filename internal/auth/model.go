package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

// ErrInvalidPassword is returned when the password sent to confirm an authenticated operation does not
// match (password change, account deletion). It wraps apperrors.ErrForbidden: the answer is 403
// INVALID_PASSWORD, never 401, which the client would take for an expired session (ADR-03 of feature 006).
var ErrInvalidPassword = fmt.Errorf("invalid password: %w", apperrors.ErrForbidden)

// User represents a registered user.
type User struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	SessionVersion int64     `json:"-"` // bumped on password change (ADR-01 of feature 006)
	Theme          *Theme    `json:"-"` // nil = never chosen in the account
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RegisterRequest is the payload accepted by POST /auth/register.
// Name is trimmed and must have 2 to 60 characters (Unicode code points).
type RegisterRequest struct {
	Name     string `json:"name"`
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
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Theme is the color theme saved in the account (RN-07 of feature 006).
type Theme string

const (
	ThemeLight Theme = "LIGHT"
	ThemeDark  Theme = "DARK"
)

// IsValid reports whether t is one of the accepted themes.
func (t Theme) IsValid() bool {
	switch t {
	case ThemeLight, ThemeDark:
		return true
	}
	return false
}

// UserResponse is returned by GET /users/me, PUT /users/me/profile and PUT /users/me/preferences.
// Theme is null when the user never chose one in the account.
type UserResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Theme *Theme `json:"theme"`
}

// UpdateProfileRequest is the payload of PUT /users/me/profile. Any other field (e.g. email) is ignored.
type UpdateProfileRequest struct {
	Name string `json:"name"`
}

// ChangePasswordRequest is the payload of PUT /users/me/password. Passwords are never trimmed.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// DeleteAccountRequest is the payload of DELETE /users/me. The password is never trimmed.
type DeleteAccountRequest struct {
	Password string `json:"password"`
}

// UpdatePreferencesRequest is the payload of PUT /users/me/preferences.
type UpdatePreferencesRequest struct {
	Theme Theme `json:"theme"`
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
	// UserSessionVersion is the owner's current session version, read in the same query, so the
	// refreshed access token carries it.
	UserSessionVersion int64
}

// Claims holds the claims of an access JWT. SessionVersion ("sv") must match users.session_version;
// a token without "sv" (issued before feature 006) counts as 0, the column default.
type Claims struct {
	jwt.RegisteredClaims
	SessionVersion int64 `json:"sv"`
}

const (
	JWTIssuer   = "we-sync-api"
	JWTAudience = "we-sync-api"
)
