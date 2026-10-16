package purchase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/configs"
	"github.com/shopspring/decimal"
)

const (
	dbTimeout      = 10 * time.Second
	dbSystem       = "postgresql"
	dbName         = "finance"
	repoTracerName = "we-sync-api/purchase/repository"
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

type purchasePostgresRepository struct {
	db *configs.DB
}

// NewPurchaseRepository creates a PostgreSQL-backed PurchaseRepository.
func NewPurchaseRepository(db *configs.DB) PurchaseRepository {
	return &purchasePostgresRepository{db: db}
}

// Create inserts a new purchase entry using the write DB and returns the created record.
func (r *purchasePostgresRepository) Create(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error) {
	// Use a CTE to INSERT and then JOIN with categories in one round-trip.
	const query = `
		WITH inserted AS (
			INSERT INTO purchase_entries
				(user_id, category_id, purchase_date, description, payment_method, amount, status, created_at, updated_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
			RETURNING id, user_id, category_id, purchase_date, description, payment_method, amount, status, created_at, updated_at
		)
		SELECT
			i.id, i.user_id, i.purchase_date, i.description,
			i.payment_method, i.amount, i.status, i.created_at, i.updated_at,
			c.id, c.user_id, c.name
		FROM inserted i
		JOIN categories c ON c.id = i.category_id`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.insert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "purchase_entries")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var p Purchase
	var amountStr string
	var catUserID sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query,
		userID,
		req.CategoryID,
		req.PurchaseDate.Time,
		req.Description,
		req.PaymentMethod,
		req.Amount.String(),
		req.Status,
	).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&p.PaymentMethod,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			// category_id was removed after the service validated it.
			return nil, category.ErrCategoryNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Create: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Create parse amount: %w", err)
	}

	span.SetAttributes(attribute.Int64("db.result.id", p.ID))
	return &p, nil
}

// GetByID fetches a single purchase entry by ID using the read DB.
// Returns (nil, nil) when the record is not found.
func (r *purchasePostgresRepository) GetByID(ctx context.Context, userID, id int64) (*Purchase, error) {
	const query = `
		SELECT
			pe.id, pe.user_id, pe.purchase_date, pe.description,
			pe.payment_method, pe.amount, pe.status, pe.created_at, pe.updated_at,
			c.id, c.user_id, c.name
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		WHERE  pe.id = $1 AND pe.user_id = $2`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.select_by_id")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "purchase_entries"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var p Purchase
	var amountStr string
	var catUserID sql.NullInt64
	err := r.db.Read.QueryRowContext(ctx, query, id, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&p.PaymentMethod,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
	)
	if err == sql.ErrNoRows {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.GetByID: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.GetByID parse amount: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &p, nil
}

// List retrieves a paginated and filtered list of purchase entries from the read DB.
func (r *purchasePostgresRepository) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.select_list")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "purchase_entries"),
		attribute.Int("db.query.page", filter.Page),
		attribute.Int("db.query.page_size", filter.PageSize),
	)...)

	args := []any{}
	conditions := []string{"pe.user_id = $1"}
	args = append(args, filter.UserID)
	argIdx := 2

	if filter.Month > 0 {
		conditions = append(conditions, fmt.Sprintf("EXTRACT(MONTH FROM pe.purchase_date) = $%d", argIdx))
		args = append(args, filter.Month)
		argIdx++
	}
	if filter.Year > 0 {
		conditions = append(conditions, fmt.Sprintf("EXTRACT(YEAR FROM pe.purchase_date) = $%d", argIdx))
		args = append(args, filter.Year)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("pe.status = $%d", argIdx))
		args = append(args, string(filter.Status))
		argIdx++
	}
	if filter.CategoryID > 0 {
		conditions = append(conditions, fmt.Sprintf("pe.category_id = $%d", argIdx))
		args = append(args, filter.CategoryID)
		argIdx++
	}
	if filter.PaymentMethod != "" {
		conditions = append(conditions, fmt.Sprintf("pe.payment_method = $%d", argIdx))
		args = append(args, string(filter.PaymentMethod))
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	countQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM purchase_entries pe JOIN categories c ON c.id = pe.category_id %s",
		where,
	)
	var total int
	if err := r.db.Read.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.List count: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	dataQuery := fmt.Sprintf(`
		SELECT
			pe.id, pe.user_id, pe.purchase_date, pe.description,
			pe.payment_method, pe.amount, pe.status, pe.created_at, pe.updated_at,
			c.id, c.user_id, c.name
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		%s
		ORDER  BY pe.purchase_date DESC, pe.id DESC
		LIMIT  $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Read.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.List query: %w", err)
	}
	defer rows.Close()

	items := []Purchase{}
	for rows.Next() {
		var p Purchase
		var amountStr string
		var catUserID sql.NullInt64
		if err := rows.Scan(
			&p.ID,
			&p.UserID,
			&p.PurchaseDate.Time,
			&p.Description,
			&p.PaymentMethod,
			&amountStr,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.Category.ID,
			&catUserID,
			&p.Category.Name,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("purchasePostgresRepository.List scan: %w", err)
		}
		p.Category.IsGlobal = !catUserID.Valid
		p.Amount, err = decimal.NewFromString(amountStr)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("purchasePostgresRepository.List parse amount: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.List rows: %w", err)
	}

	totalPages := total / filter.PageSize
	if total%filter.PageSize != 0 {
		totalPages++
	}

	span.SetAttributes(
		attribute.Int("db.result.total", total),
		attribute.Int("db.result.count", len(items)),
	)

	return &PaginatedResult{
		Items:      items,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// Update modifies an existing purchase entry using the write DB and returns the updated record.
// Returns (nil, nil) when no row with the given ID exists.
func (r *purchasePostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	const query = `
		WITH updated AS (
			UPDATE purchase_entries
			SET
				category_id    = $1,
				purchase_date  = $2,
				description    = $3,
				payment_method = $4,
				amount         = $5,
				status         = $6,
				updated_at     = NOW()
			WHERE id = $7 AND user_id = $8
			RETURNING id, user_id, category_id, purchase_date, description, payment_method, amount, status, created_at, updated_at
		)
		SELECT
			u.id, u.user_id, u.purchase_date, u.description,
			u.payment_method, u.amount, u.status, u.created_at, u.updated_at,
			c.id, c.user_id, c.name
		FROM updated u
		JOIN categories c ON c.id = u.category_id`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "purchase_entries"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var p Purchase
	var amountStr string
	var catUserID sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query,
		req.CategoryID,
		req.PurchaseDate.Time,
		req.Description,
		req.PaymentMethod,
		req.Amount.String(),
		req.Status,
		id,
		userID,
	).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&p.PaymentMethod,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
	)
	if err == sql.ErrNoRows {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			// category_id was removed after the service validated it.
			return nil, category.ErrCategoryNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Update: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Update parse amount: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.result.found", true))
	return &p, nil
}

// Delete removes a purchase entry by ID using the write DB.
// Returns (false, nil) when no row with the given ID exists.
func (r *purchasePostgresRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
	const query = `DELETE FROM purchase_entries WHERE id = $1 AND user_id = $2`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.delete")
	defer span.End()
	span.SetAttributes(append(dbAttrs("DELETE", "purchase_entries"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	result, err := r.db.Write.ExecContext(ctx, query, id, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("purchasePostgresRepository.Delete: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("purchasePostgresRepository.Delete RowsAffected: %w", err)
	}

	found := rowsAffected > 0
	span.SetAttributes(attribute.Bool("db.result.found", found))
	return found, nil
}

// isForeignKeyViolation reports whether err is a PostgreSQL foreign key violation (23503).
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
