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
	"github.com/jonasjesusamerico/we-sync-api/internal/configs"
)

const (
	dbTimeout      = 10 * time.Second
	dbSystem       = "postgresql"
	dbName         = "finance"
	repoTracerName = "we-sync-api/category/repository"
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

type categoryPostgresRepository struct {
	db *configs.DB
}

// NewCategoryRepository creates a PostgreSQL-backed CategoryRepository.
func NewCategoryRepository(db *configs.DB) CategoryRepository {
	return &categoryPostgresRepository{db: db}
}

// Create inserts a new personal category owned by userID.
// Returns apperrors.ErrConflict on name uniqueness violation (pg error 23505).
func (r *categoryPostgresRepository) Create(ctx context.Context, userID int64, req *CreateCategoryRequest) (*Category, error) {
	const query = `
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at, updated_at`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.insert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "categories")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var c Category
	var userIDVal sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query, userID, req.Name).Scan(
		&c.ID,
		&userIDVal,
		&c.Name,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.Create: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		c.UserID = &v
	}
	c.IsGlobal = c.UserID == nil

	span.SetAttributes(attribute.Int64("db.result.id", c.ID))
	return &c, nil
}

// GetByID fetches a category by ID, regardless of ownership.
// Returns (nil, nil) when the category does not exist.
func (r *categoryPostgresRepository) GetByID(ctx context.Context, id int64) (*Category, error) {
	const query = `
		SELECT id, user_id, name, created_at, updated_at
		FROM   categories
		WHERE  id = $1`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.select_by_id")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var c Category
	var userIDVal sql.NullInt64
	err := r.db.Read.QueryRowContext(ctx, query, id).Scan(
		&c.ID,
		&userIDVal,
		&c.Name,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.GetByID: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		c.UserID = &v
	}
	c.IsGlobal = c.UserID == nil

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &c, nil
}

// ListForUser returns all categories visible to the user: global ones plus the user's own.
// Globals are listed first (user_id IS NULL), then personal categories alphabetically.
func (r *categoryPostgresRepository) ListForUser(ctx context.Context, userID int64) ([]Category, error) {
	// ORDER BY (user_id IS NULL) DESC puts globals first; lower(name) for deterministic order.
	const query = `
		SELECT id, user_id, name, created_at, updated_at
		FROM   categories
		WHERE  user_id IS NULL OR user_id = $1
		ORDER  BY (user_id IS NULL) DESC, lower(name) ASC`

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
	defer rows.Close()

	items := []Category{}
	for rows.Next() {
		var c Category
		var userIDVal sql.NullInt64
		if err := rows.Scan(&c.ID, &userIDVal, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("categoryPostgresRepository.ListForUser scan: %w", err)
		}
		if userIDVal.Valid {
			v := userIDVal.Int64
			c.UserID = &v
		}
		c.IsGlobal = c.UserID == nil
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryPostgresRepository.ListForUser rows: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result.count", len(items)))
	return items, nil
}

// ExistsGlobalByName reports whether a global category named name exists (case-insensitive).
func (r *categoryPostgresRepository) ExistsGlobalByName(ctx context.Context, name string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM categories
			WHERE  user_id IS NULL AND lower(name) = lower($1)
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

// Update renames a personal category scoped to userID.
// Returns (nil, nil) when the category does not exist or belongs to another user.
// Returns apperrors.ErrConflict on name uniqueness violation.
func (r *categoryPostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error) {
	const query = `
		UPDATE categories
		SET    name = $1, updated_at = NOW()
		WHERE  id = $2 AND user_id = $3
		RETURNING id, user_id, name, created_at, updated_at`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var c Category
	var userIDVal sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query, req.Name, id, userID).Scan(
		&c.ID,
		&userIDVal,
		&c.Name,
		&c.CreatedAt,
		&c.UpdatedAt,
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
		return nil, fmt.Errorf("categoryPostgresRepository.Update: %w", err)
	}

	if userIDVal.Valid {
		v := userIDVal.Int64
		c.UserID = &v
	}
	c.IsGlobal = c.UserID == nil

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &c, nil
}

// Delete removes a personal category scoped to userID.
// Returns (false, nil) when the category does not exist or belongs to another user.
// Returns apperrors.ErrConflict when the category is referenced by purchase entries (pg error 23503).
func (r *categoryPostgresRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
	const query = `DELETE FROM categories WHERE id = $1 AND user_id = $2`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.categories.delete")
	defer span.End()
	span.SetAttributes(append(dbAttrs("DELETE", "categories"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, id, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			// Foreign key violation: category is referenced by purchase_entries.
			return false, apperrors.ErrConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("categoryPostgresRepository.Delete: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("categoryPostgresRepository.Delete RowsAffected: %w", err)
	}

	found := rowsAffected > 0
	span.SetAttributes(attribute.Bool("db.result.found", found))
	return found, nil
}
