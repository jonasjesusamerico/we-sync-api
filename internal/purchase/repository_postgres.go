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
	"github.com/jonasjesusamerico/we-sync-api/internal/database"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
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

// Create inserts a new purchase entry using the write DB and returns the created record.
func (r *PostgresRepository) Create(ctx context.Context, userID int64, req *CreatePurchaseRequest) (*Purchase, error) {
	// Use a CTE to INSERT and then JOIN with categories and payment methods in one round-trip.
	const query = `
		WITH inserted AS (
			INSERT INTO purchase_entries
				(user_id, category_id, purchase_date, description, payment_method_id, amount, status, created_at, updated_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
			RETURNING id, user_id, category_id, purchase_date, description, payment_method_id, amount, status, created_at, updated_at
		)
		SELECT
			i.id, i.user_id, i.purchase_date, i.description,
			i.amount, i.status, i.created_at, i.updated_at,
			c.id, c.user_id, c.name, c.icon, c.color,
			pm.id, pm.user_id, pm.name
		FROM inserted i
		JOIN categories c ON c.id = i.category_id
		JOIN payment_methods pm ON pm.id = i.payment_method_id`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.insert")
	defer span.End()
	span.SetAttributes(dbAttrs("INSERT", "purchase_entries")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var p Purchase
	var amountStr string
	var catUserID, pmUserID sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query,
		userID,
		req.CategoryID,
		req.PurchaseDate.Time,
		req.Description,
		req.PaymentMethodID,
		req.Amount.String(),
		req.Status,
	).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
		&p.Category.Icon,
		&p.Category.Color,
		&p.PaymentMethod.ID,
		&pmUserID,
		&p.PaymentMethod.Name,
	)
	if err != nil {
		if notFound := referenceNotFound(err); notFound != nil {
			return nil, notFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Create: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.PaymentMethod.IsGlobal = !pmUserID.Valid
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
func (r *PostgresRepository) GetByID(ctx context.Context, userID, id int64) (*Purchase, error) {
	const query = `
		SELECT
			pe.id, pe.user_id, pe.purchase_date, pe.description,
			pe.amount, pe.status, pe.created_at, pe.updated_at,
			c.id, c.user_id, c.name, c.icon, c.color,
			pm.id, pm.user_id, pm.name
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		JOIN   payment_methods pm ON pm.id = pe.payment_method_id
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
	var catUserID, pmUserID sql.NullInt64
	err := r.db.Read.QueryRowContext(ctx, query, id, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
		&p.Category.Icon,
		&p.Category.Color,
		&p.PaymentMethod.ID,
		&pmUserID,
		&p.PaymentMethod.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.GetByID: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.PaymentMethod.IsGlobal = !pmUserID.Valid
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
func (r *PostgresRepository) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.select_list")
	defer span.End()
	span.SetAttributes(append(dbAttrs("SELECT", "purchase_entries"),
		attribute.Int("db.query.page", filter.Page),
		attribute.Int("db.query.page_size", filter.PageSize),
	)...)

	where, args := listWhere(filter)
	argIdx := len(args) + 1

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	// No JOIN: category_id is a NOT NULL foreign key, so joining cannot change the count.
	// The amount reads the same rows, so the count and the sum always agree on the filter.
	countQuery := `SELECT COUNT(*), COALESCE(SUM(pe.amount) FILTER (WHERE pe.status <> 'CANCELED'), 0)
		FROM purchase_entries pe ` + where
	var total int
	var totalAmountStr string
	if err := r.db.Read.QueryRowContext(ctx, countQuery, args...).Scan(&total, &totalAmountStr); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.List count: %w", err)
	}
	totalAmount, err := decimal.NewFromString(totalAmountStr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.List parse total amount: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	dataQuery := fmt.Sprintf(`
		SELECT
			pe.id, pe.user_id, pe.purchase_date, pe.description,
			pe.amount, pe.status, pe.created_at, pe.updated_at,
			c.id, c.user_id, c.name, c.icon, c.color,
			pm.id, pm.user_id, pm.name
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		JOIN   payment_methods pm ON pm.id = pe.payment_method_id
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
	defer func() { _ = rows.Close() }()

	items := []Purchase{}
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("purchasePostgresRepository.List scan: %w", err)
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
		Items:       items,
		Total:       total,
		TotalAmount: totalAmount,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		TotalPages:  totalPages,
	}, nil
}

// Summarize sums the user's purchases of the month and of the compared periods in one query.
// All bounds are inclusive dates, so December of 9999 never needs the year 10000. The
// previous month is read with purchase_date < MonthStart; without a previous month the
// scan starts at MonthStart and that sum is always 0, with no date of year 0 sent.
func (r *PostgresRepository) Summarize(ctx context.Context, userID int64, ranges SummaryRanges) (*SummarySums, error) {
	const query = `
		SELECT
			COUNT(*) FILTER (WHERE pe.purchase_date >= $3),
			COALESCE(SUM(pe.amount) FILTER (WHERE pe.purchase_date >= $3 AND pe.status <> 'CANCELED'), 0),
			COALESCE(SUM(pe.amount) FILTER (WHERE pe.purchase_date >= $3 AND pe.purchase_date <= $5
			                                  AND pe.status <> 'CANCELED'), 0),
			COALESCE(SUM(pe.amount) FILTER (WHERE pe.purchase_date < $3 AND pe.purchase_date <= $6
			                                  AND pe.status <> 'CANCELED'), 0)
		FROM  purchase_entries pe
		WHERE pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date <= $4`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.select_summary")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "purchase_entries")...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	scanStart, previousEnd := ranges.MonthStart, ranges.MonthStart
	if ranges.HasPrevious {
		scanStart, previousEnd = ranges.PreviousStart, ranges.PreviousEnd
	}

	var sums SummarySums
	var spentStr, currentStr, previousStr string
	err := r.db.Read.QueryRowContext(ctx, query,
		userID, scanStart, ranges.MonthStart, ranges.MonthEnd, ranges.CurrentEnd, previousEnd,
	).Scan(&sums.MonthCount, &spentStr, &currentStr, &previousStr)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Summarize: %w", err)
	}

	for _, field := range []struct {
		raw string
		dst *decimal.Decimal
	}{
		{spentStr, &sums.MonthSpent},
		{currentStr, &sums.CurrentAmount},
		{previousStr, &sums.PreviousAmount},
	} {
		*field.dst, err = decimal.NewFromString(field.raw)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("purchasePostgresRepository.Summarize parse amount: %w", err)
		}
	}

	return &sums, nil
}

// rowScanner is satisfied by *sql.Rows and *sql.Row.
type rowScanner interface{ Scan(dest ...any) error }

// scanPurchase reads one purchase in the column order of the List SELECT:
// pe.id, pe.user_id, pe.purchase_date, pe.description, pe.amount, pe.status, pe.created_at,
// pe.updated_at, c.id, c.user_id, c.name, c.icon, c.color, pm.id, pm.user_id, pm.name.
// The caller wraps the error with its own prefix.
func scanPurchase(row rowScanner) (Purchase, error) {
	var p Purchase
	var amountStr string
	var catUserID, pmUserID sql.NullInt64
	if err := row.Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
		&p.Category.Icon,
		&p.Category.Color,
		&p.PaymentMethod.ID,
		&pmUserID,
		&p.PaymentMethod.Name,
	); err != nil {
		return Purchase{}, err
	}
	p.Category.IsGlobal = !catUserID.Valid
	p.PaymentMethod.IsGlobal = !pmUserID.Valid
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return Purchase{}, fmt.Errorf("parse amount: %w", err)
	}
	p.Amount = amount
	return p, nil
}

// Overview reads the blocks of the month in one read-only REPEATABLE READ transaction (ADR-03 of
// feature 007): the four reads see the same snapshot, so the blocks always add up to each other.
// All bounds are inclusive dates, like Summarize: December of 9999 never needs the year 10000.
func (r *PostgresRepository) Overview(ctx context.Context, userID int64, monthStart, monthEnd time.Time) (*OverviewData, error) {
	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.select_overview")
	defer span.End()
	span.SetAttributes(dbAttrs("SELECT", "purchase_entries")...)

	// One timeout for the whole transaction.
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	fail := func(step string, err error) (*OverviewData, error) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Overview %s: %w", step, err)
	}

	tx, err := r.db.Read.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fail("begin", err)
	}
	// No-op after Commit (sql.ErrTxDone); read-only, so nothing to undo.
	defer func() { _ = tx.Rollback() }()

	days, err := overviewDays(ctx, tx, userID, monthStart, monthEnd)
	if err != nil {
		return fail("days", err)
	}
	categories, err := overviewCategories(ctx, tx, userID, monthStart, monthEnd)
	if err != nil {
		return fail("categories", err)
	}
	paymentMethods, err := overviewPaymentMethods(ctx, tx, userID, monthStart, monthEnd)
	if err != nil {
		return fail("payment methods", err)
	}
	latest, err := overviewLatest(ctx, tx, userID, monthStart, monthEnd)
	if err != nil {
		return fail("latest", err)
	}
	if err := tx.Commit(); err != nil {
		return fail("commit", err)
	}

	span.SetAttributes(
		attribute.Int("db.result.days", len(days)),
		attribute.Int("db.result.categories", len(categories)),
		attribute.Int("db.result.payment_methods", len(paymentMethods)),
		attribute.Int("db.result.latest", len(latest)),
	)
	return &OverviewData{Days: days, Categories: categories, PaymentMethods: paymentMethods, Latest: latest}, nil
}

// overviewDays sums PAID and PENDING per day of the month (at most 31 rows).
func overviewDays(ctx context.Context, tx *sql.Tx, userID int64, monthStart, monthEnd time.Time) ([]DayTotals, error) {
	const query = `
		SELECT pe.purchase_date,
		       COALESCE(SUM(pe.amount) FILTER (WHERE pe.status = 'PAID'), 0),
		       COUNT(*)                FILTER (WHERE pe.status = 'PAID'),
		       COALESCE(SUM(pe.amount) FILTER (WHERE pe.status = 'PENDING'), 0),
		       COUNT(*)                FILTER (WHERE pe.status = 'PENDING')
		FROM   purchase_entries pe
		WHERE  pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date <= $3
		  AND  pe.status <> 'CANCELED'
		GROUP  BY pe.purchase_date
		ORDER  BY pe.purchase_date`

	rows, err := tx.QueryContext(ctx, query, userID, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	days := []DayTotals{}
	for rows.Next() {
		var d DayTotals
		var paidStr, pendingStr string
		if err := rows.Scan(&d.Date, &paidStr, &d.PaidCount, &pendingStr, &d.PendingCount); err != nil {
			return nil, err
		}
		if d.PaidAmount, err = decimal.NewFromString(paidStr); err != nil {
			return nil, fmt.Errorf("parse amount: %w", err)
		}
		if d.PendingAmount, err = decimal.NewFromString(pendingStr); err != nil {
			return nil, fmt.Errorf("parse amount: %w", err)
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// overviewCategories sums PAID and PENDING per category, archived ones included (RN-10 of feature 007).
// GROUP BY c.id is enough: it is the primary key, and the other columns depend on it.
func overviewCategories(ctx context.Context, tx *sql.Tx, userID int64, monthStart, monthEnd time.Time) ([]CategoryTotal, error) {
	const query = `
		SELECT c.id, c.user_id, c.name, c.icon, c.color, SUM(pe.amount)
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		WHERE  pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date <= $3
		  AND  pe.status <> 'CANCELED'
		GROUP  BY c.id
		ORDER  BY SUM(pe.amount) DESC, catalog_name_key(c.name), c.id`

	rows, err := tx.QueryContext(ctx, query, userID, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	totals := []CategoryTotal{}
	for rows.Next() {
		var ct CategoryTotal
		var catUserID sql.NullInt64
		var amountStr string
		if err := rows.Scan(&ct.Category.ID, &catUserID, &ct.Category.Name, &ct.Category.Icon,
			&ct.Category.Color, &amountStr); err != nil {
			return nil, err
		}
		ct.Category.IsGlobal = !catUserID.Valid
		if ct.Amount, err = decimal.NewFromString(amountStr); err != nil {
			return nil, fmt.Errorf("parse amount: %w", err)
		}
		totals = append(totals, ct)
	}
	return totals, rows.Err()
}

// overviewPaymentMethods sums PAID and PENDING per payment method, archived ones included.
func overviewPaymentMethods(ctx context.Context, tx *sql.Tx, userID int64, monthStart, monthEnd time.Time) ([]PaymentMethodTotal, error) {
	const query = `
		SELECT pm.id, pm.user_id, pm.name, SUM(pe.amount)
		FROM   purchase_entries pe
		JOIN   payment_methods pm ON pm.id = pe.payment_method_id
		WHERE  pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date <= $3
		  AND  pe.status <> 'CANCELED'
		GROUP  BY pm.id
		ORDER  BY SUM(pe.amount) DESC, catalog_name_key(pm.name), pm.id`

	rows, err := tx.QueryContext(ctx, query, userID, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	totals := []PaymentMethodTotal{}
	for rows.Next() {
		var pt PaymentMethodTotal
		var pmUserID sql.NullInt64
		var amountStr string
		if err := rows.Scan(&pt.PaymentMethod.ID, &pmUserID, &pt.PaymentMethod.Name, &amountStr); err != nil {
			return nil, err
		}
		pt.PaymentMethod.IsGlobal = !pmUserID.Valid
		if pt.Amount, err = decimal.NewFromString(amountStr); err != nil {
			return nil, fmt.Errorf("parse amount: %w", err)
		}
		totals = append(totals, pt)
	}
	return totals, rows.Err()
}

// overviewLatest reads the month's newest purchases of every status (RN-07 of feature 007), in the
// order of the purchase list.
func overviewLatest(ctx context.Context, tx *sql.Tx, userID int64, monthStart, monthEnd time.Time) ([]Purchase, error) {
	const query = `
		SELECT pe.id, pe.user_id, pe.purchase_date, pe.description,
		       pe.amount, pe.status, pe.created_at, pe.updated_at,
		       c.id, c.user_id, c.name, c.icon, c.color,
		       pm.id, pm.user_id, pm.name
		FROM   purchase_entries pe
		JOIN   categories c ON c.id = pe.category_id
		JOIN   payment_methods pm ON pm.id = pe.payment_method_id
		WHERE  pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date <= $3
		ORDER  BY pe.purchase_date DESC, pe.id DESC
		LIMIT  $4`

	rows, err := tx.QueryContext(ctx, query, userID, monthStart, monthEnd, overviewLatestLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	latest := []Purchase{}
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		latest = append(latest, p)
	}
	return latest, rows.Err()
}

// listWhere builds the WHERE clause and its positional arguments for List.
// The year (and month, when both are given) filter is expressed as a
// purchase_date range so it can use the (user_id, purchase_date) index;
// a month without a year matches that month in any year.
func listWhere(filter ListFilter) (string, []any) {
	args := []any{filter.UserID}
	conditions := []string{"pe.user_id = $1"}
	add := func(condition string, arg any) {
		args = append(args, arg)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}

	switch {
	case filter.Year > 0 && filter.Month > 0:
		from := time.Date(filter.Year, time.Month(filter.Month), 1, 0, 0, 0, 0, time.UTC)
		add("pe.purchase_date >= $%d", from)
		add("pe.purchase_date < $%d", from.AddDate(0, 1, 0))
	case filter.Year > 0:
		from := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
		add("pe.purchase_date >= $%d", from)
		add("pe.purchase_date < $%d", from.AddDate(1, 0, 0))
	case filter.Month > 0:
		add("EXTRACT(MONTH FROM pe.purchase_date) = $%d", filter.Month)
	}
	if filter.Status != "" {
		add("pe.status = $%d", string(filter.Status))
	}
	if filter.CategoryID > 0 {
		add("pe.category_id = $%d", filter.CategoryID)
	}
	if filter.PaymentMethodID > 0 {
		add("pe.payment_method_id = $%d", filter.PaymentMethodID)
	}
	if filter.Search != "" {
		// strpos has no wildcards, so %, _ and \ in the search are literal. LIKE with an
		// escape would not be enough: unaccent.rules maps fullwidth forms such as ％ to %.
		add("strpos(lower(unaccent(pe.description)), lower(unaccent($%d))) > 0", filter.Search)
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

// Update modifies an existing purchase entry using the write DB and returns the updated record.
// Returns (nil, nil) when no row with the given ID exists.
func (r *PostgresRepository) Update(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	const query = `
		WITH updated AS (
			UPDATE purchase_entries
			SET
				category_id    = $1,
				purchase_date  = $2,
				description    = $3,
				payment_method_id = $4,
				amount         = $5,
				status         = $6,
				updated_at     = NOW()
			WHERE id = $7 AND user_id = $8
			RETURNING id, user_id, category_id, purchase_date, description, payment_method_id, amount, status, created_at, updated_at
		)
		SELECT
			u.id, u.user_id, u.purchase_date, u.description,
			u.amount, u.status, u.created_at, u.updated_at,
			c.id, c.user_id, c.name, c.icon, c.color,
			pm.id, pm.user_id, pm.name
		FROM updated u
		JOIN categories c ON c.id = u.category_id
		JOIN payment_methods pm ON pm.id = u.payment_method_id`

	ctx, span := otel.Tracer(repoTracerName).Start(ctx, "db.purchase_entries.update")
	defer span.End()
	span.SetAttributes(append(dbAttrs("UPDATE", "purchase_entries"),
		attribute.Int64("db.query.id", id),
	)...)

	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var p Purchase
	var amountStr string
	var catUserID, pmUserID sql.NullInt64
	err := r.db.Write.QueryRowContext(ctx, query,
		req.CategoryID,
		req.PurchaseDate.Time,
		req.Description,
		req.PaymentMethodID,
		req.Amount.String(),
		req.Status,
		id,
		userID,
	).Scan(
		&p.ID,
		&p.UserID,
		&p.PurchaseDate.Time,
		&p.Description,
		&amountStr,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Category.ID,
		&catUserID,
		&p.Category.Name,
		&p.Category.Icon,
		&p.Category.Color,
		&p.PaymentMethod.ID,
		&pmUserID,
		&p.PaymentMethod.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		span.SetAttributes(attribute.Bool("db.result.found", false))
		return nil, nil
	}
	if err != nil {
		if notFound := referenceNotFound(err); notFound != nil {
			return nil, notFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchasePostgresRepository.Update: %w", err)
	}

	p.Category.IsGlobal = !catUserID.Valid
	p.PaymentMethod.IsGlobal = !pmUserID.Valid
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
func (r *PostgresRepository) Delete(ctx context.Context, userID, id int64) (bool, error) {
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

// paymentMethodFKConstraint is the payment_method_id FK name set in 000006_create_payment_methods.
const paymentMethodFKConstraint = "fk_purchase_entries_payment_method"

// referenceNotFound maps a PostgreSQL foreign key violation (23503) to the not-found
// error of the referenced domain: the category or payment method was removed after
// the service validated it. Returns nil when err is not a foreign key violation.
func referenceNotFound(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return nil
	}
	if pgErr.ConstraintName == paymentMethodFKConstraint {
		return paymentmethod.ErrPaymentMethodNotFound
	}
	return category.ErrCategoryNotFound
}
