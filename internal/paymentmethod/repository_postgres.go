package paymentmethod

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
	dbTimeout      = 10 * time.Second
	dbSystem       = "postgresql"
	dbName         = "finance"
	repoTracerName = "we-sync-api/paymentmethod/repository"
)

// dbAttrs returns the base semantic-convention attributes shared by all DB spans.
func dbAttrs(operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.DBSystemNameKey.String(dbSystem),
		semconv.DBNamespaceKey.String(dbName),
		semconv.DBOperationNameKey.String(operation),
		semconv.DBCollectionNameKey.String(table),
	}
}

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

// Create inserts a new personal payment method owned by userID.
// Returns apperrors.ErrConflict on name uniqueness violation (pg error 23505).
func (r *PostgresRepository) Create(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, error) {
	const query = `
		INSERT INTO payment_methods (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at, updated_at`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.insert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "payment_methods")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var pm PaymentMethod
	var userIDVal sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query, userID, req.Name).Scan(
		&pm.ID,
		&userIDVal,
		&pm.Name,
		&pm.CreatedAt,
		&pm.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.Create: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		pm.UserID = &v
	}
	pm.IsGlobal = pm.UserID == nil

	span.SetAttributes(attribute.Int64("db.result.id", pm.ID))
	return &pm, nil
}

// GetByID fetches a payment method by ID, regardless of ownership.
// Returns (nil, nil) when the payment method does not exist.
func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (*PaymentMethod, error) {
	const query = `
		SELECT id, user_id, name, created_at, updated_at
		FROM   payment_methods
		WHERE  id = $1`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.select_by_id")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var pm PaymentMethod
	var userIDVal sql.NullInt64
	err := r.db.Read.QueryRowContext(ctx, query, id).Scan(
		&pm.ID,
		&userIDVal,
		&pm.Name,
		&pm.CreatedAt,
		&pm.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.GetByID: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		pm.UserID = &v
	}
	pm.IsGlobal = pm.UserID == nil

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &pm, nil
}

// ListForUser returns all payment methods visible to the user: global ones plus the user's own.
// Globals are listed first (user_id IS NULL), then personal payment methods alphabetically.
func (r *PostgresRepository) ListForUser(ctx context.Context, userID int64) ([]PaymentMethod, error) {
	// ORDER BY (user_id IS NULL) DESC puts globals first; lower(name) for deterministic order.
	const query = `
		SELECT id, user_id, name, created_at, updated_at
		FROM   payment_methods
		WHERE  user_id IS NULL OR user_id = $1
		ORDER  BY (user_id IS NULL) DESC, lower(name) ASC`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.select_list")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "payment_methods")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	rows, err := r.db.Read.QueryContext(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.ListForUser: %w", err)
	}
	defer rows.Close()

	items := []PaymentMethod{}
	for rows.Next() {
		var pm PaymentMethod
		var userIDVal sql.NullInt64
		if err := rows.Scan(&pm.ID, &userIDVal, &pm.Name, &pm.CreatedAt, &pm.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("paymentMethodPostgresRepository.ListForUser scan: %w", err)
		}
		if userIDVal.Valid {
			v := userIDVal.Int64
			pm.UserID = &v
		}
		pm.IsGlobal = pm.UserID == nil
		items = append(items, pm)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.ListForUser rows: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result.count", len(items)))
	return items, nil
}

// ExistsGlobalByName reports whether a global payment method named name exists (case-insensitive).
func (r *PostgresRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM payment_methods
			WHERE  user_id IS NULL AND lower(name) = lower($1)
		)`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.exists_global_by_name")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "payment_methods")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var exists bool
	if err := r.db.Read.QueryRowContext(ctx, query, name).Scan(&exists); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("paymentMethodPostgresRepository.ExistsGlobalByName: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", exists))
	return exists, nil
}

// Update renames a personal payment method scoped to userID.
// Returns (nil, nil) when the payment method does not exist or belongs to another user.
// Returns apperrors.ErrConflict on name uniqueness violation.
func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	const query = `
		UPDATE payment_methods
		SET    name = $1, updated_at = NOW()
		WHERE  id = $2 AND user_id = $3
		RETURNING id, user_id, name, created_at, updated_at`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var pm PaymentMethod
	var userIDVal sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query, req.Name, id, userID).Scan(
		&pm.ID,
		&userIDVal,
		&pm.Name,
		&pm.CreatedAt,
		&pm.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.Update: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		pm.UserID = &v
	}
	pm.IsGlobal = pm.UserID == nil

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &pm, nil
}

// Delete removes a personal payment method scoped to userID.
// Returns (false, nil) when the payment method does not exist or belongs to another user.
// Returns apperrors.ErrConflict when the payment method is referenced by purchase entries (pg error 23503).
func (r *PostgresRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
	const query = `DELETE FROM payment_methods WHERE id = $1 AND user_id = $2`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.delete")
	defer span.End()
	span.SetAttributes(append(dbAttrs("DELETE", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, id, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			// Foreign key violation: payment method is referenced by purchase_entries.
			return false, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("paymentMethodPostgresRepository.Delete: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("paymentMethodPostgresRepository.Delete RowsAffected: %w", err)
	}

	found := rowsAffected > 0
	span.SetAttributes(attribute.Bool("db.result.found", found))
	return found, nil
}
