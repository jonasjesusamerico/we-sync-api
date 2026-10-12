package auth

import (
	"context"
	"time"
)

type mockAuthRepository struct {
	CreateUserFn         func(context.Context, string, string) (*User, error)
	GetUserByEmailFn     func(context.Context, string) (*User, error)
	CreateRefreshTokenFn func(context.Context, int64, string, time.Time) error
	GetRefreshTokenFn    func(context.Context, string) (*RefreshToken, error)
	RotateRefreshTokenFn func(context.Context, int64, int64, string, time.Time) error
	RevokeRefreshTokenFn func(context.Context, string) error
}

func (m *mockAuthRepository) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	if m.CreateUserFn == nil {
		return nil, nil
	}
	return m.CreateUserFn(ctx, email, passwordHash)
}

func (m *mockAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	if m.GetUserByEmailFn == nil {
		return nil, nil
	}
	return m.GetUserByEmailFn(ctx, email)
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
