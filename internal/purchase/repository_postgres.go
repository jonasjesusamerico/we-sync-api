package purchase

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jonasjesusamerico/we-sync-api/internal/configs"
)

type purchasePostgresRepository struct {
	db *configs.DB
}

// NewPurchaseRepository creates a PostgreSQL-backed PurchaseRepository.
func NewPurchaseRepository(db *configs.DB) PurchaseRepository {
	return &purchasePostgresRepository{db: db}
}

// Create inserts a new purchase entry using the write DB and returns the created record.
func (r *purchasePostgresRepository) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	const query = `
		INSERT INTO purchase_entries
			(purchase_date, description, sector, payment_method, amount, status, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, purchase_date, description, sector, payment_method, amount, status, created_at, updated_at`

	var p Purchase
	err := r.db.Write.QueryRowContext(ctx, query,
		req.PurchaseDate,
		req.Description,
		req.Sector,
		req.PaymentMethod,
		req.Amount,
		req.Status,
	).Scan(
		&p.ID,
		&p.PurchaseDate,
		&p.Description,
		&p.Sector,
		&p.PaymentMethod,
		&p.Amount,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.Create: %w", err)
	}

	return &p, nil
}

// GetByID fetches a single purchase entry by ID using the read DB.
// Returns (nil, nil) when the record is not found.
func (r *purchasePostgresRepository) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	const query = `
		SELECT id, purchase_date, description, sector, payment_method, amount, status, created_at, updated_at
		FROM   purchase_entries
		WHERE  id = $1`

	var p Purchase
	err := r.db.Read.QueryRowContext(ctx, query, id).Scan(
		&p.ID,
		&p.PurchaseDate,
		&p.Description,
		&p.Sector,
		&p.PaymentMethod,
		&p.Amount,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.GetByID: %w", err)
	}

	return &p, nil
}

// List retrieves a paginated and filtered list of purchase entries from the read DB.
func (r *purchasePostgresRepository) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	args := []any{}
	conditions := []string{}
	argIdx := 1

	if filter.Month > 0 {
		conditions = append(conditions, fmt.Sprintf("EXTRACT(MONTH FROM purchase_date) = $%d", argIdx))
		args = append(args, filter.Month)
		argIdx++
	}
	if filter.Year > 0 {
		conditions = append(conditions, fmt.Sprintf("EXTRACT(YEAR FROM purchase_date) = $%d", argIdx))
		args = append(args, filter.Year)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Sector != "" {
		conditions = append(conditions, fmt.Sprintf("sector ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Sector+"%")
		argIdx++
	}
	if filter.PaymentMethod != "" {
		conditions = append(conditions, fmt.Sprintf("payment_method = $%d", argIdx))
		args = append(args, filter.PaymentMethod)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows before applying pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM purchase_entries %s", where)
	var total int
	if err := r.db.Read.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.List count: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	dataQuery := fmt.Sprintf(`
		SELECT id, purchase_date, description, sector, payment_method, amount, status, created_at, updated_at
		FROM   purchase_entries
		%s
		ORDER  BY purchase_date DESC, id DESC
		LIMIT  $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Read.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.List query: %w", err)
	}
	defer rows.Close()

	items := []Purchase{}
	for rows.Next() {
		var p Purchase
		if err := rows.Scan(
			&p.ID,
			&p.PurchaseDate,
			&p.Description,
			&p.Sector,
			&p.PaymentMethod,
			&p.Amount,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("purchasePostgresRepository.List scan: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.List rows: %w", err)
	}

	totalPages := total / filter.PageSize
	if total%filter.PageSize != 0 {
		totalPages++
	}

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
func (r *purchasePostgresRepository) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	const query = `
		UPDATE purchase_entries
		SET
			purchase_date  = $1,
			description    = $2,
			sector         = $3,
			payment_method = $4,
			amount         = $5,
			status         = $6,
			updated_at     = NOW()
		WHERE id = $7
		RETURNING id, purchase_date, description, sector, payment_method, amount, status, created_at, updated_at`

	var p Purchase
	err := r.db.Write.QueryRowContext(ctx, query,
		req.PurchaseDate,
		req.Description,
		req.Sector,
		req.PaymentMethod,
		req.Amount,
		req.Status,
		id,
	).Scan(
		&p.ID,
		&p.PurchaseDate,
		&p.Description,
		&p.Sector,
		&p.PaymentMethod,
		&p.Amount,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("purchasePostgresRepository.Update: %w", err)
	}

	return &p, nil
}

// Delete removes a purchase entry by ID using the write DB.
// Returns (false, nil) when no row with the given ID exists.
func (r *purchasePostgresRepository) Delete(ctx context.Context, id int64) (bool, error) {
	const query = `DELETE FROM purchase_entries WHERE id = $1`

	result, err := r.db.Write.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("purchasePostgresRepository.Delete: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("purchasePostgresRepository.Delete RowsAffected: %w", err)
	}

	return rows > 0, nil
}
