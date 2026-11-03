package auth

import (
	"context"
	"time"
)

// Repository is the outbound port for authentication persistence.
type Repository interface {
	// CreateUser stores a user with an already trimmed name and normalized email.
	// It returns apperrors.ErrConflict when the email already exists.
	CreateUser(ctx context.Context, name, email, passwordHash string) (*User, error)
	// GetUserByEmail returns (nil, nil) when the user does not exist.
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	// GetUserByID returns (nil, nil) when the user does not exist. It reads from the
	// write pool: the client calls GET /users/me right after register and login.
	GetUserByID(ctx context.Context, id int64) (*User, error)
	CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	// GetRefreshToken returns (nil, nil) when the token does not exist.
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	// RotateRefreshToken revokes the old token and creates a new one atomically.
	RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}
