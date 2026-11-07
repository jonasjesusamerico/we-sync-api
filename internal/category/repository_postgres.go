package category

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
	repoTracerName = "we-sync-api/category/repository"
)

// categoryColumns are the columns scanCategory reads, in order.
const categoryColumns = `id, user_id, name, icon, color, archived_at, created_at, updated_at`

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

// scanCategory reads categoryColumns, followed by extra destinations, and converts the nullable columns.
func scanCategory(s rowScanner, extra ...any) (*Category, error) {
	var c Category
	var userID sql.NullInt64
	var archivedAt sql.NullTime
	dest := append([]any{&c.ID, &userID, &c.Name, &c.Icon, &c.Color, &archivedAt, &c.CreatedAt, &c.UpdatedAt}, extra...)
	if err := s.Scan(dest...); err != nil {
		return nil, err
	}
	if userID.Valid {
		v := userID.Int64
		c.UserID = &v
	}
	if archivedAt.Valid {
		v := archivedAt.Time
		c.ArchivedAt = &v
	}
	c.IsGlobal = c.UserID == nil
	return &c, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create inserts a personal category, or reactivates the user's archived one with the same name key.
// Returns (nil, false, nil) when an active category with the same name key exists, and
// apperrors.ErrConflict on any other unique violation (not expected).
func (r *PostgresRepository) Create(ctx context.Context, userID int64, req *CreateCategoryRequest) (*Category, bool, error) {
	// `xmax = 0` is true only for a freshly inserted row; a row updated by ON CONFLICT carries the locking
	// transaction id. The ON CONFLICT WHERE skips an active row, so no row comes back.
	const query = `
		INSERT INTO categories (user_id, name, icon, color)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, catalog_name_key(name)) WHERE user_id IS NOT NULL
		DO UPDATE SET icon = EXCLUDED.icon, color = EXCLUDED.color, archived_at = NULL, updated_at = NOW()
		   WHERE categories.archived_at IS NOT NULL
		RETURNING ` + categoryColumns + `, (xmax = 0) AS inserted`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.upsert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "categories")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var inserted bool
	c, err := scanCategory(r.db.Write.QueryRowContext(ctx, query, userID, req.Name, req.Icon, req.Color), &inserted)
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
		return nil, false, fmt.Errorf("categoryPostgresRepository.Create: %w", err)
	}

	span.SetAttributes(attribute.Int64("db.result.id", c.ID), attribute.Bool("db.result.inserted", inserted))
	return c, inserted, nil
}

// GetByID fetches a category by ID, regardless of ownership.
// Returns (nil, nil) when the category does not exist.
func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (*Category, error) {
	const query = `
		SELECT ` + categoryColumns + `
		FROM   categories
		WHERE  id = $1`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.select_by_id")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	c, err := scanCategory(r.db.Read.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.GetByID: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return c, nil
}

// ListForUser returns all categories visible to the user, archived ones included: global ones plus
// the user's own. Globals are listed first (user_id IS NULL), then personal categories alphabetically.
// No limit: these are hand-made lists of one person, tens of items (RN-20 of feature 006).
func (r *PostgresRepository) ListForUser(ctx context.Context, userID int64) ([]Category, error) {
	// ORDER BY (user_id IS NULL) DESC puts globals first; lower(name) and id give a deterministic order.
	const query = `
		SELECT ` + categoryColumns + `
		FROM   categories
		WHERE  user_id IS NULL OR user_id = $1
		ORDER  BY (user_id IS NULL) DESC, lower(name) ASC, id ASC`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.select_list")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "categories")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	rows, err := r.db.Read.QueryContext(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.ListForUser: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("categoryPostgresRepository.ListForUser scan: %w", err)
		}
		items = append(items, *c)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.ListForUser rows: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result.count", len(items)))
	return items, nil
}

// ExistsGlobalByName reports whether a global category has the same name key (case- and
// accent-insensitive).
func (r *PostgresRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM categories
			WHERE  user_id IS NULL AND catalog_name_key(name) = catalog_name_key($1)
		)`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.exists_global_by_name")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "categories")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var exists bool
	if err := r.db.Read.QueryRowContext(ctx, query, name).Scan(&exists); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("categoryPostgresRepository.ExistsGlobalByName: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", exists))
	return exists, nil
}

// FindOwnByName returns the user's own category (active or archived) with the same name key, reading
// from the write database because it runs right after a unique violation. Returns (nil, nil) if none.
func (r *PostgresRepository) FindOwnByName(ctx context.Context, userID int64, name string) (*Category, error) {
	const query = `
		SELECT ` + categoryColumns + `
		FROM   categories
		WHERE  user_id = $1 AND catalog_name_key(name) = catalog_name_key($2)`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.select_own_by_name")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "categories")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	c, err := scanCategory(r.db.Write.QueryRowContext(ctx, query, userID, name))
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.FindOwnByName: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return c, nil
}

// Update replaces the name, icon and color of a personal category scoped to userID.
// Returns (nil, nil) when the category does not exist or belongs to another user.
// Returns apperrors.ErrConflict on name uniqueness violation.
func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error) {
	const query = `
		UPDATE categories
		SET    name = $1, icon = $2, color = $3, updated_at = NOW()
		WHERE  id = $4 AND user_id = $5
		RETURNING ` + categoryColumns

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	c, err := scanCategory(r.db.Write.QueryRowContext(ctx, query, req.Name, req.Icon, req.Color, id, userID))
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
		return nil, fmt.Errorf("categoryPostgresRepository.Update: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return c, nil
}

// SetArchived archives or reactivates a personal category scoped to userID. Archiving keeps the
// original archived_at of an already archived category. A global category never matches
// (user_id IS NULL), and the categories_global_not_archived check also protects it.
// Returns false when the category does not exist or belongs to another user.
func (r *PostgresRepository) SetArchived(ctx context.Context, userID, id int64, archived bool) (bool, error) {
	const archiveQuery = `
		UPDATE categories SET archived_at = COALESCE(archived_at, NOW()), updated_at = NOW()
		WHERE  id = $1 AND user_id = $2`
	const unarchiveQuery = `
		UPDATE categories SET archived_at = NULL, updated_at = NOW()
		WHERE  id = $1 AND user_id = $2`

	query, spanName := unarchiveQuery, "db.categories.unarchive"
	if archived {
		query, spanName = archiveQuery, "db.categories.archive"
	}

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, spanName)
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, id, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("categoryPostgresRepository.SetArchived: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("categoryPostgresRepository.SetArchived RowsAffected: %w", err)
	}

	found := rowsAffected > 0
	span.SetAttributes(attribute.Bool("db.result.found", found))
	return found, nil
}
