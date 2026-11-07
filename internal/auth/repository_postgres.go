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
	"github.com/jonasjesusamerico/we-sync-api/internal/database"
)

const (
	authDBTimeout      = 10 * time.Second
	authDBSystem       = "postgresql"
	authDBName         = "finance"
	authRepoTracerName = "we-sync-api/auth/repository"
)

// PostgresRepository is the PostgreSQL implementation of Repository.
type PostgresRepository struct {
	db *database.DB
}

// Compile-time check that PostgresRepository satisfies Repository.
var _ Repository = (*PostgresRepository)(nil)

// NewRepository creates a PostgreSQL-backed Repository.
func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
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
func (r *PostgresRepository) CreateUser(ctx context.Context, name, email, passwordHash string) (*User, error) {
	const query = `
		INSERT INTO users (name, email, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, name, email, password_hash, created_at, updated_at`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.insert")
	defer span.End()
	span.SetAttributes(authDBAttrs("INSERT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var user User
	err := r.db.Write.QueryRowContext(ctx, query, name, email, passwordHash).Scan(
		&user.ID,
		&user.Name,
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

// GetUserByEmail retrieves a user using the write database. Authentication
// needs read-after-write consistency when a user logs in immediately after
// registration; ordinary read-only queries continue to use the read pool.
// It returns (nil, nil) when no user exists for the email.
func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	const query = `
		SELECT id, name, email, password_hash, session_version, theme, created_at, updated_at
		FROM users
		WHERE email = $1`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.select_by_email")
	defer span.End()
	span.SetAttributes(authDBAttrs("SELECT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var user User
	var theme sql.NullString
	err := r.db.Write.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.SessionVersion,
		&theme,
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
	user.Theme = themeFromNull(theme)

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &user, nil
}

// GetUserByID retrieves a user's public data using the write database, because
// the client calls GET /users/me right after register and login. The password
// hash is not read. It returns (nil, nil) when no user exists for the id.
func (r *PostgresRepository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	const query = `
		SELECT ` + publicUserColumns + `
		FROM users
		WHERE id = $1`
	return r.queryPublicUser(ctx, "db.users.select_by_id", "SELECT", "GetUserByID", query, id)
}

// UpdateName stores the user's name using the write database. It returns (nil, nil) when no user
// exists for the id.
func (r *PostgresRepository) UpdateName(ctx context.Context, userID int64, name string) (*User, error) {
	const query = `
		UPDATE users SET name = $2, updated_at = NOW()
		WHERE  id = $1
		RETURNING ` + publicUserColumns
	return r.queryPublicUser(ctx, "db.users.update_name", "UPDATE", "UpdateName", query, userID, name)
}

// UpdateTheme stores the user's theme using the write database. It returns (nil, nil) when no user
// exists for the id.
func (r *PostgresRepository) UpdateTheme(ctx context.Context, userID int64, theme Theme) (*User, error) {
	const query = `
		UPDATE users SET theme = $2, updated_at = NOW()
		WHERE  id = $1
		RETURNING ` + publicUserColumns
	return r.queryPublicUser(ctx, "db.users.update_theme", "UPDATE", "UpdateTheme", query, userID, string(theme))
}

// GetPasswordHash reads the user's password hash from the write database. It returns ("", false, nil)
// when no user exists for the id.
func (r *PostgresRepository) GetPasswordHash(ctx context.Context, userID int64) (string, bool, error) {
	const query = `SELECT password_hash FROM users WHERE id = $1`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.select_password_hash")
	defer span.End()
	span.SetAttributes(authDBAttrs("SELECT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var hash string
	err := r.db.Write.QueryRowContext(ctx, query, userID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return "", false, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", false, fmt.Errorf("authPostgresRepository.GetPasswordHash: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return hash, true, nil
}

// ChangePassword replaces the password hash (compare-and-set on oldHash), bumps the session version,
// revokes every refresh token of the user and stores the caller's new one, in one transaction. Locks
// are taken in a fixed order: the users row first, then the refresh_tokens rows.
func (r *PostgresRepository) ChangePassword(ctx context.Context, userID int64, oldHash, newHash, newRefreshHash string, refreshExpiresAt time.Time) (int64, bool, error) {
	// Compare-and-set: only if the hash is still the one the service checked.
	const updateUserQuery = `
		UPDATE users
		SET    password_hash = $3, session_version = session_version + 1, updated_at = NOW()
		WHERE  id = $1 AND password_hash = $2
		RETURNING session_version`
	// Every other session ends.
	const revokeQuery = `UPDATE refresh_tokens SET revoked = TRUE WHERE user_id = $1 AND revoked = FALSE`
	// The caller's new session.
	const insertQuery = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, FALSE, NOW())`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.change_password")
	defer span.End()
	span.SetAttributes(authDBAttrs("UPDATE", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	tx, err := r.db.Write.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.ChangePassword begin: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var version int64
	err = tx.QueryRowContext(ctx, updateUserQuery, userID, oldHash, newHash).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return 0, false, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.ChangePassword update user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, revokeQuery, userID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.ChangePassword revoke: %w", err)
	}
	if _, err := tx.ExecContext(ctx, insertQuery, userID, newRefreshHash, refreshExpiresAt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.ChangePassword insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.ChangePassword commit: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return version, true, nil
}

// DeleteUser deletes the user in one statement, only if the password hash is still the one the service
// checked. refresh_tokens, categories, payment_methods and purchase_entries cascade on user_id; the
// purchase_entries foreign keys to categories and payment methods are checked at the end of the
// statement, when the user's purchases are already gone. It returns (false, nil) when no row matched.
func (r *PostgresRepository) DeleteUser(ctx context.Context, userID int64, passwordHash string) (bool, error) {
	const query = `DELETE FROM users WHERE id = $1 AND password_hash = $2`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.delete")
	defer span.End()
	span.SetAttributes(authDBAttrs("DELETE", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, userID, passwordHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("authPostgresRepository.DeleteUser: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("authPostgresRepository.DeleteUser rows: %w", err)
	}
	span.SetAttributes(attribute.Int64("db.response.returned_rows", rows))
	return rows > 0, nil
}

// publicUserColumns are the users columns read without the password hash, in the order
// queryPublicUser scans them.
const publicUserColumns = `id, name, email, session_version, theme, created_at, updated_at`

// queryPublicUser runs a single-row users query (by primary key) that selects or returns
// publicUserColumns. It returns (nil, nil) when no row matches.
func (r *PostgresRepository) queryPublicUser(ctx context.Context, spanName, operation, method, query string, args ...any) (*User, error) {
	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, spanName)
	defer span.End()
	span.SetAttributes(authDBAttrs(operation, "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var user User
	var theme sql.NullString
	err := r.db.Write.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.SessionVersion,
		&theme,
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
		return nil, fmt.Errorf("authPostgresRepository.%s: %w", method, err)
	}
	user.Theme = themeFromNull(theme)

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &user, nil
}

// themeFromNull converts the nullable users.theme column; NULL means never chosen.
func themeFromNull(value sql.NullString) *Theme {
	if !value.Valid {
		return nil
	}
	theme := Theme(value.String)
	return &theme
}

// SessionVersion reads the user's session version from the write database, because a password
// change has just bumped it. It returns (0, false, nil) when no user exists for the id.
func (r *PostgresRepository) SessionVersion(ctx context.Context, userID int64) (int64, bool, error) {
	const query = `SELECT session_version FROM users WHERE id = $1`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.users.select_session_version")
	defer span.End()
	span.SetAttributes(authDBAttrs("SELECT", "users")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	var version int64
	err := r.db.Write.QueryRowContext(ctx, query, userID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return 0, false, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, false, fmt.Errorf("authPostgresRepository.SessionVersion: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return version, true, nil
}

// CreateRefreshToken stores a refresh-token hash using the write database.
func (r *PostgresRepository) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
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

// GetRefreshToken reads a refresh token and its owner's session version from the write database to
// avoid replica lag. The foreign key cascades on user delete, so the JOIN never drops a token row.
// It returns (nil, nil) when the token does not exist.
func (r *PostgresRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	const query = `
		SELECT rt.id, rt.user_id, rt.token_hash, rt.expires_at, rt.revoked, rt.created_at, u.session_version
		FROM   refresh_tokens rt
		JOIN   users u ON u.id = rt.user_id
		WHERE  rt.token_hash = $1`

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
		&token.UserSessionVersion,
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
func (r *PostgresRepository) RotateRefreshToken(ctx context.Context, oldID, userID int64, newTokenHash string, newExpiresAt time.Time) error {
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

	defer func() { _ = tx.Rollback() }()

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

// RevokeRefreshToken marks a refresh token as revoked using the write database.
func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	const query = `UPDATE refresh_tokens SET revoked = TRUE WHERE token_hash = $1 AND revoked = FALSE`

	ctx, span := otel.Tracer(authRepoTracerName).Start(ctx, "db.refresh_tokens.revoke")
	defer span.End()
	span.SetAttributes(authDBAttrs("UPDATE", "refresh_tokens")...)

	ctx, cancel := context.WithTimeout(ctx, authDBTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, tokenHash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("authPostgresRepository.RevokeRefreshToken: %w", err)
	}
	if rows, err := result.RowsAffected(); err == nil {
		span.SetAttributes(attribute.Int64("db.response.returned_rows", rows))
	}
	return nil
}
