package auth

import (
	"context"
	"time"
)

type mockAuthRepository struct {
	CreateUserFn         func(context.Context, string, string, string) (*User, error)
	GetUserByEmailFn     func(context.Context, string) (*User, error)
	GetUserByIDFn        func(context.Context, int64) (*User, error)
	SessionVersionFn     func(context.Context, int64) (int64, bool, error)
	UpdateNameFn         func(context.Context, int64, string) (*User, error)
	UpdateThemeFn        func(context.Context, int64, Theme) (*User, error)
	GetPasswordHashFn    func(context.Context, int64) (string, bool, error)
	ChangePasswordFn     func(context.Context, int64, string, string, string, time.Time) (int64, bool, error)
	DeleteUserFn         func(context.Context, int64, string) (bool, error)
	CreateRefreshTokenFn func(context.Context, int64, string, time.Time) error
	GetRefreshTokenFn    func(context.Context, string) (*RefreshToken, error)
	RotateRefreshTokenFn func(context.Context, int64, int64, string, time.Time) error
	RevokeRefreshTokenFn func(context.Context, string) error
}

func (m *mockAuthRepository) CreateUser(ctx context.Context, name, email, passwordHash string) (*User, error) {
	if m.CreateUserFn == nil {
		return nil, nil
	}
	return m.CreateUserFn(ctx, name, email, passwordHash)
}

func (m *mockAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	if m.GetUserByEmailFn == nil {
		return nil, nil
	}
	return m.GetUserByEmailFn(ctx, email)
}

func (m *mockAuthRepository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	if m.GetUserByIDFn == nil {
		return nil, nil
	}
	return m.GetUserByIDFn(ctx, id)
}

func (m *mockAuthRepository) SessionVersion(ctx context.Context, userID int64) (int64, bool, error) {
	if m.SessionVersionFn == nil {
		return 0, true, nil
	}
	return m.SessionVersionFn(ctx, userID)
}

func (m *mockAuthRepository) UpdateName(ctx context.Context, userID int64, name string) (*User, error) {
	if m.UpdateNameFn == nil {
		return nil, nil
	}
	return m.UpdateNameFn(ctx, userID, name)
}

func (m *mockAuthRepository) UpdateTheme(ctx context.Context, userID int64, theme Theme) (*User, error) {
	if m.UpdateThemeFn == nil {
		return nil, nil
	}
	return m.UpdateThemeFn(ctx, userID, theme)
}

func (m *mockAuthRepository) GetPasswordHash(ctx context.Context, userID int64) (string, bool, error) {
	if m.GetPasswordHashFn == nil {
		return "", false, nil
	}
	return m.GetPasswordHashFn(ctx, userID)
}

func (m *mockAuthRepository) ChangePassword(ctx context.Context, userID int64, oldHash, newHash, newRefreshHash string, refreshExpiresAt time.Time) (int64, bool, error) {
	if m.ChangePasswordFn == nil {
		return 0, false, nil
	}
	return m.ChangePasswordFn(ctx, userID, oldHash, newHash, newRefreshHash, refreshExpiresAt)
}

func (m *mockAuthRepository) DeleteUser(ctx context.Context, userID int64, passwordHash string) (bool, error) {
	if m.DeleteUserFn == nil {
		return false, nil
	}
	return m.DeleteUserFn(ctx, userID, passwordHash)
}

func (m *mockAuthRepository) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	if m.CreateRefreshTokenFn == nil {
		return nil
	}
	return m.CreateRefreshTokenFn(ctx, userID, tokenHash, expiresAt)
}

func (m *mockAuthRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	if m.GetRefreshTokenFn == nil {
		return nil, nil
	}
	return m.GetRefreshTokenFn(ctx, tokenHash)
}

func (m *mockAuthRepository) RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error {
	if m.RotateRefreshTokenFn == nil {
		return nil
	}
	return m.RotateRefreshTokenFn(ctx, oldID, userID, newTokenHash, newExpiresAt)
}

func (m *mockAuthRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if m.RevokeRefreshTokenFn == nil {
		return nil
	}
	return m.RevokeRefreshTokenFn(ctx, tokenHash)
}
