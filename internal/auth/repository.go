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
	// GetUserByEmail fills SessionVersion and Theme and returns (nil, nil) when the user does not exist.
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	// GetUserByID fills SessionVersion and Theme (not the password hash) and returns (nil, nil) when the
	// user does not exist. It reads from the write pool: the client calls GET /users/me right after
	// register and login.
	GetUserByID(ctx context.Context, id int64) (*User, error)
	// UpdateName stores an already trimmed name and returns the updated user (without the password
	// hash), or (nil, nil) when the user does not exist.
	UpdateName(ctx context.Context, userID int64, name string) (*User, error)
	// UpdateTheme stores the theme and returns the updated user (without the password hash), or
	// (nil, nil) when the user does not exist.
	UpdateTheme(ctx context.Context, userID int64, theme Theme) (*User, error)
	// SessionVersion returns the user's current session version, read by the JWT middleware on every
	// authenticated request. It returns (0, false, nil) when the user does not exist. It reads from the
	// write pool: a password change has just bumped the version.
	SessionVersion(ctx context.Context, userID int64) (version int64, found bool, err error)
	// GetPasswordHash returns the user's password hash, or ("", false, nil) when the user does not exist.
	// It reads from the write pool.
	GetPasswordHash(ctx context.Context, userID int64) (hash string, found bool, err error)
	// ChangePassword, in one transaction, replaces the password hash only if it is still oldHash, bumps
	// session_version, revokes every refresh token of the user and stores the new one. It returns the
	// new session version, or ok == false when the hash changed meanwhile or the user does not exist
	// (nothing is written).
	ChangePassword(ctx context.Context, userID int64, oldHash, newHash, newRefreshHash string, refreshExpiresAt time.Time) (newVersion int64, ok bool, err error)
	// DeleteUser deletes the user only if the password hash is still passwordHash; the dependent rows
	// (refresh tokens, purchases, own categories and payment methods) go by ON DELETE CASCADE. It
	// returns (false, nil) when no row was deleted.
	DeleteUser(ctx context.Context, userID int64, passwordHash string) (bool, error)
	CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	// GetRefreshToken fills UserSessionVersion with the owner's current version and returns
	// (nil, nil) when the token does not exist.
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	// RotateRefreshToken revokes the old token and creates a new one atomically.
	RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}
