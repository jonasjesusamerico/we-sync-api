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

// paymentMethodColumns are the columns scanPaymentMethod reads, in order.
const paymentMethodColumns = `id, user_id, name, archived_at, created_at, updated_at`

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

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanPaymentMethod reads paymentMethodColumns, followed by extra destinations, and converts the nullable columns.
func scanPaymentMethod(s rowScanner, extra ...any) (*PaymentMethod, error) {
	var pm PaymentMethod
	var userID sql.NullInt64
	var archivedAt sql.NullTime
	dest := append([]any{&pm.ID, &userID, &pm.Name, &archivedAt, &pm.CreatedAt, &pm.UpdatedAt}, extra...)
	if err := s.Scan(dest...); err != nil {
		return nil, err
	}
	if userID.Valid {
		v := userID.Int64
		pm.UserID = &v
	}
	if archivedAt.Valid {
		v := archivedAt.Time
		pm.ArchivedAt = &v
	}
	pm.IsGlobal = pm.UserID == nil
	return &pm, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create inserts a personal payment method, or reactivates the user's archived one with the same name key.
// Returns (nil, false, nil) when an active payment method with the same name key exists, and
// apperrors.ErrConflict on any other unique violation (not expected).
func (r *PostgresRepository) Create(ctx context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
	// `xmax = 0` is true only for a freshly inserted row; a row updated by ON CONFLICT carries the locking
	// transaction id. The ON CONFLICT WHERE skips an active row, so no row comes back.
	const query = `
		INSERT INTO payment_methods (user_id, name)
		VALUES ($1, $2)
		ON CONFLICT (user_id, catalog_name_key(name)) WHERE user_id IS NOT NULL
		DO UPDATE SET archived_at = NULL, updated_at = NOW()
		   WHERE payment_methods.archived_at IS NOT NULL
		RETURNING ` + paymentMethodColumns + `, (xmax = 0) AS inserted`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.upsert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "payment_methods")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var inserted bool
	pm, err := scanPaymentMethod(r.db.Write.QueryRowContext(ctx, query, userID, req.Name), &inserted)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, false, nil
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, false, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, fmt.Errorf("paymentMethodPostgresRepository.Create: %w", err)
	}

	span.SetAttributes(attribute.Int64("db.result.id", pm.ID), attribute.Bool("db.result.inserted", inserted))
	return pm, inserted, nil
}

// GetByID fetches a payment method by ID, regardless of ownership.
// Returns (nil, nil) when the payment method does not exist.
func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (*PaymentMethod, error) {
	const query = `
		SELECT ` + paymentMethodColumns + `
		FROM   payment_methods
		WHERE  id = $1`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.select_by_id")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	pm, err := scanPaymentMethod(r.db.Read.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.GetByID: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return pm, nil
}

// ListForUser returns all payment methods visible to the user, archived ones included: global ones plus
// the user's own. Globals are listed first (user_id IS NULL), then personal payment methods alphabetically.
// No limit: these are hand-made lists of one person, tens of items (RN-20 of feature 006).
func (r *PostgresRepository) ListForUser(ctx context.Context, userID int64) ([]PaymentMethod, error) {
	// ORDER BY (user_id IS NULL) DESC puts globals first; lower(name) and id give a deterministic order.
	const query = `
		SELECT ` + paymentMethodColumns + `
		FROM   payment_methods
		WHERE  user_id IS NULL OR user_id = $1
		ORDER  BY (user_id IS NULL) DESC, lower(name) ASC, id ASC`

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
		pm, err := scanPaymentMethod(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("paymentMethodPostgresRepository.ListForUser scan: %w", err)
		}
		items = append(items, *pm)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.ListForUser rows: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result.count", len(items)))
	return items, nil
}

// ExistsGlobalByName reports whether a global payment method has the same name key (case- and
// accent-insensitive).
func (r *PostgresRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM payment_methods
			WHERE  user_id IS NULL AND catalog_name_key(name) = catalog_name_key($1)
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

// FindOwnByName returns the user's own payment method (active or archived) with the same name key, reading
// from the write database because it runs right after a unique violation. Returns (nil, nil) if none.
func (r *PostgresRepository) FindOwnByName(ctx context.Context, userID int64, name string) (*PaymentMethod, error) {
	const query = `
		SELECT ` + paymentMethodColumns + `
		FROM   payment_methods
		WHERE  user_id = $1 AND catalog_name_key(name) = catalog_name_key($2)`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.select_own_by_name")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "payment_methods")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	pm, err := scanPaymentMethod(r.db.Write.QueryRowContext(ctx, query, userID, name))
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.FindOwnByName: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return pm, nil
}

// Update renames a personal payment method scoped to userID.
// Returns (nil, nil) when the payment method does not exist or belongs to another user.
// Returns apperrors.ErrConflict on name uniqueness violation.
func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	const query = `
		UPDATE payment_methods
		SET    name = $1, updated_at = NOW()
		WHERE  id = $2 AND user_id = $3
		RETURNING ` + paymentMethodColumns

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.payment_methods.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	pm, err := scanPaymentMethod(r.db.Write.QueryRowContext(ctx, query, req.Name, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodPostgresRepository.Update: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return pm, nil
}

// SetArchived archives or reactivates a personal payment method scoped to userID. Archiving keeps the
// original archived_at of an already archived payment method. A global payment method never matches
// (user_id IS NULL), and the payment_methods_global_not_archived check also protects it.
// Returns false when the payment method does not exist or belongs to another user.
func (r *PostgresRepository) SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error) {
	const archiveQuery = `
		UPDATE payment_methods SET archived_at = COALESCE(archived_at, NOW()), updated_at = NOW()
		WHERE  id = $1 AND user_id = $2`
	const unarchiveQuery = `
		UPDATE payment_methods SET archived_at = NULL, updated_at = NOW()
		WHERE  id = $1 AND user_id = $2`

	query, spanName := unarchiveQuery, "db.payment_methods.unarchive"
	if archived {
		query, spanName = archiveQuery, "db.payment_methods.archive"
	}

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, spanName)
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "payment_methods"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, id, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("paymentMethodPostgresRepository.SetArchived: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("paymentMethodPostgresRepository.SetArchived RowsAffected: %w", err)
	}

	found := rowsAffected > 0
	span.SetAttributes(attribute.Bool("db.result.found", found))
	return found, nil
}
