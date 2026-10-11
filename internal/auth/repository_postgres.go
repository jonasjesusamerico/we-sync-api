package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/configs"
)

const (
	authDBTimeout      = 10 * time.Second
	authDBSystem       = "postgresql"
	authDBName         = "finance"
	authRepoTracerName = "we-sync-api/auth/repository"
)

type authPostgresRepository struct {
	db *configs.DB
}

// NewAuthRepository creates a PostgreSQL-backed AuthRepository.
func NewAuthRepository(db *configs.DB) AuthRepository {
	return &authPostgresRepository{db: db}
}

func authDBAttrs(operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.DBSystemNameKey.String(authDBSystem),
		semconv.DBNamespaceKey.String(authDBName),
		semconv.DBOperationNameKey.String(operation),
		semconv.DBCollectionNameKey.String(table),
	}
}

// CreateUser inserts a user using the write database.
func (r *authPostgresRepository) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	const query = `
		INSERT INTO users (email, password_hash, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id, email, password_hash, created_at, updated_at`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.insert")
	defer span.End()
	span.SetAttributes(authDBAttrs("INSERT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var user User
	err := r.db.Write.QueryRowContext(ctx, query, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("authPostgresRepository.CreateUser: %w", err)
	}

	span.SetAttributes(attribute.Int64("db.result.id", user.ID))
	return &user, nil
}

// GetUserByEmail retrieves a user using the read database.
// It returns (nil, nil) when no user exists for the email.
func (r *authPostgresRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	const query = `
		SELECT id, email, password_hash, created_at, updated_at
		FROM users
		WHERE email = $1`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.select_by_email")
	defer span.End()
	span.SetAttributes(authDBAttrs("SELECT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var user User
	err := r.db.Read.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("authPostgresRepository.GetUserByEmail: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &user, nil
}

// CreateRefreshToken stores a refresh-token hash using the write database.
func (r *authPostgresRepository) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	const query = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, FALSE, NOW())`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.refresh_tokens.insert")
	defer span.End()
	span.SetAttributes(authDBAttrs("INSERT", "refresh_tokens")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	if _, err := r.db.Write.ExecContext(ctx, query, userID, tokenHash, expiresAt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.CreateRefreshToken: %w", err)
	}
	return nil
}

// GetRefreshToken reads a refresh token from the write database to avoid replica lag.
// It returns (nil, nil) when the token does not exist.
func (r *authPostgresRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, revoked, created_at
		FROM refresh_tokens
		WHERE token_hash = $1`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.refresh_tokens.select_by_hash")
	defer span.End()
	span.SetAttributes(authDBAttrs("SELECT", "refresh_tokens")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var token RefreshToken
	err := r.db.Write.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.Revoked,
		&token.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("authPostgresRepository.GetRefreshToken: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &token, nil
}

// RotateRefreshToken revokes the old token and inserts its replacement in one transaction.
func (r *authPostgresRepository) RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error {
	const revokeQuery = `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE id = $1 AND user_id = $2 AND revoked = FALSE`
	const insertQuery = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, FALSE, NOW())`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.refresh_tokens.rotate")
	defer span.End()
	span.SetAttributes(authDBAttrs("UPDATE", "refresh_tokens")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	tx, err := r.db.Write.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RotateRefreshToken begin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, revokeQuery, oldID, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RotateRefreshToken revoke: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RotateRefreshToken revoked rows: %w", err)
	}
	if rowsAffected != 1 {
		err := apperrors.ErrUnauthorized
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if _, err := tx.ExecContext(ctx, insertQuery, userID, newTokenHash, newExpiresAt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RotateRefreshToken insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RotateRefreshToken commit: %w", err)
	}

	return nil
}
