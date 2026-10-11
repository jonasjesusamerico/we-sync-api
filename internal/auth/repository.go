package auth

import (
	"context"
	"time"
)

// AuthRepository is the outbound port for authentication persistence.
type AuthRepository interface {
	// CreateUser returns apperrors.ErrConflict when the email already exists.
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	// GetUserByEmail returns (nil, nil) when the user does not exist.
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	// GetRefreshToken returns (nil, nil) when the token does not exist.
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	// RotateRefreshToken revokes the old token and creates a new one atomically.
	RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error
}
