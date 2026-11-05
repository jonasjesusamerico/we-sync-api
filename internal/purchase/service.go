package purchase

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
	"github.com/shopspring/decimal"
)

const tracerName = "we-sync-api/purchase"
const meterName = "we-sync-api/purchase"

// categoryLookup is the only behaviour the purchase service needs from the
// category domain. Declaring it here, at the consumer, keeps the dependency
// minimal: *category.PostgresRepository satisfies it implicitly.
type categoryLookup interface {
	GetByID(ctx context.Context, id int64) (*category.Category, error)
}

// paymentMethodLookup is the only behaviour the purchase service needs from the
// payment method domain; *paymentmethod.PostgresRepository satisfies it implicitly.
type paymentMethodLookup interface {
	GetByID(ctx context.Context, id int64) (*paymentmethod.PaymentMethod, error)
}

// Service implements the purchase-entry business rules.
type Service struct {
	repo           Repository
	categories     categoryLookup
	paymentMethods paymentMethodLookup

	// Business metrics — recorded after each successful mutation.
	purchasesCreated metric.Int64Counter
	purchasesUpdated metric.Int64Counter
	purchasesDeleted metric.Int64Counter
}

// NewService creates a new Service backed by the given repositories.
// categories and paymentMethods are used to validate that category_id and
// payment_method_id are accessible to the user.
func NewService(repo Repository, categories categoryLookup, paymentMethods paymentMethodLookup) *Service {
	meter := otel.Meter(meterName)

	purchasesCreated, _ := meter.Int64Counter(
		"purchase.created.total",
		metric.WithDescription("Total number of purchase entries successfully created."),
		metric.WithUnit("{purchase}"),
	)
	purchasesUpdated, _ := meter.Int64Counter(
		"purchase.updated.total",
		metric.WithDescription("Total number of purchase entries successfully updated."),
		metric.WithUnit("{purchase}"),
	)
	purchasesDeleted, _ := meter.Int64Counter(
		"purchase.deleted.total",
		metric.WithDescription("Total number of purchase entries successfully deleted."),
		metric.WithUnit("{purchase}"),
	)

	return &Service{
		repo:             repo,
		categories:       categories,
		paymentMethods:   paymentMethods,
		purchasesCreated: purchasesCreated,
		purchasesUpdated: purchasesUpdated,
		purchasesDeleted: purchasesDeleted,
	}
}

// Create validates and persists a new purchase entry.
func (s *Service) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Create")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	if req == nil {
		return nil, apperrors.NewValidationError("request is required")
	}
	if err := validateRequest(&req.PurchaseRequest); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	// A new purchase never takes an archived category or payment method (ADR-11 of feature 006).
	catArchived, err := s.validateCategoryAccess(ctx, req.CategoryID, userID)
	if err == nil && catArchived {
		err = category.ErrCategoryNotFound
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	pmArchived, err := s.validatePaymentMethodAccess(ctx, req.PaymentMethodID, userID)
	if err == nil && pmArchived {
		err = paymentmethod.ErrPaymentMethodNotFound
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	p, err := s.repo.Create(ctx, userID, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.Create: %w", err)
	}

	span.SetAttributes(
		attribute.Int64("purchase.id", p.ID),
		attribute.String("purchase.status", string(p.Status)),
		attribute.Int64("purchase.payment_method_id", p.PaymentMethod.ID),
	)

	s.purchasesCreated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", string(p.Status)),
		),
	)

	return p, nil
}

// GetByID returns the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *Service) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.GetByID")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("purchase.id", id))

	p, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.GetByID: %w", err)
	}
	if p == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	return p, nil
}

// List returns a paginated and filtered list of purchase entries.
func (s *Service) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.List")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	filter.UserID = userID
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > MaxPageSize {
		err := apperrors.NewValidationError("page_size must be at most %d", MaxPageSize)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > maxSearchLength {
		err := apperrors.NewValidationError("q must be at most %d characters", maxSearchLength)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("purchase.filter.page", filter.Page),
		attribute.Int("purchase.filter.page_size", filter.PageSize),
		attribute.Int("purchase.filter.month", filter.Month),
		attribute.Int("purchase.filter.year", filter.Year),
		attribute.String("purchase.filter.status", string(filter.Status)),
		attribute.Int64("purchase.filter.payment_method_id", filter.PaymentMethodID),
		// Only whether there is a search: the text is free input and never goes to telemetry.
		attribute.Bool("purchase.filter.has_search", filter.Search != ""),
	)

	result, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.List: %w", err)
	}

	span.SetAttributes(attribute.Int("purchase.result.total", result.Total))

	return result, nil
}

// summaryPlan is what Summary decides before reading the database.
type summaryPlan struct {
	ranges        SummaryRanges
	period        ComparisonPeriod
	untilDay      *int
	previousYear  int
	previousMonth int
}

// summaryRanges plans the month (year, month) given the client's local date today.
// year and month must already be in range. Pure: no clock, no database.
func summaryRanges(year, month int, today time.Time) summaryPlan {
	monthStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	// AddDate(0, 1, -1) is the last day of the month; December of 9999 stays in 9999.
	monthEnd := monthStart.AddDate(0, 1, -1)

	plan := summaryPlan{previousYear: year, previousMonth: month - 1}
	if month == 1 {
		plan.previousYear, plan.previousMonth = year-1, 12
	}
	hasPrevious := year > minSummaryYear || month > 1

	var previousStart, previousMonthEnd time.Time
	if hasPrevious {
		previousStart = time.Date(plan.previousYear, time.Month(plan.previousMonth), 1, 0, 0, 0, 0, time.UTC)
		previousMonthEnd = monthStart.AddDate(0, 0, -1)
	}

	currentEnd, previousEnd := monthEnd, previousMonthEnd
	if today.Year() == year && int(today.Month()) == month {
		day := today.Day()
		plan.period = PeriodUntilDay
		plan.untilDay = &day
		currentEnd = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if hasPrevious {
			// RN-02: when the previous month has no such day, it counts whole.
			previousEnd = time.Date(plan.previousYear, time.Month(plan.previousMonth),
				min(day, previousMonthEnd.Day()), 0, 0, 0, 0, time.UTC)
		}
	} else {
		plan.period = PeriodFullMonth
	}

	plan.ranges = SummaryRanges{
		MonthStart:  monthStart,
		MonthEnd:    monthEnd,
		CurrentEnd:  currentEnd,
		HasPrevious: hasPrevious,
	}
	if hasPrevious {
		plan.ranges.PreviousStart = previousStart
		plan.ranges.PreviousEnd = previousEnd
	}
	return plan
}

var hundred = decimal.NewFromInt(100)

// compare returns the result and the rounded percent of current against previous.
// percent is nil only for NO_BASE. Both amounts are >= 0.
func compare(current, previous decimal.Decimal) (ComparisonResult, *int64) {
	if previous.Sign() <= 0 {
		return ResultNoBase, nil
	}
	// Round(0) rounds half away from zero, which for a value >= 0 is "0.5 up" (RN-02).
	// An exact k.5 has a finite representation, so Div (16 digits) keeps it exact.
	n := current.Sub(previous).Abs().Mul(hundred).Div(previous).Round(0).IntPart()
	switch {
	case n == 0:
		return ResultEqual, &n
	case current.GreaterThan(previous):
		return ResultMore, &n
	default:
		return ResultLess, &n
	}
}

// Summary returns the month's spending and its comparison with the previous month (RN-01..RN-03).
// today is the client's local date and only decides the period; the data is always the
// context user's, so it is not checked against the server clock.
func (s *Service) Summary(ctx context.Context, year, month int, today time.Time) (*MonthSummary, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Summary")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	// The handler already answers 400; this guards other callers, since time.Date would
	// normalize month 13 into January of the next year and sum the wrong month.
	if year < minSummaryYear || year > maxSummaryYear {
		err := apperrors.NewValidationError("year must be an integer between %d and %d", minSummaryYear, maxSummaryYear)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if month < 1 || month > 12 {
		err := apperrors.NewValidationError("month must be an integer between 1 and 12")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	plan := summaryRanges(year, month, today)
	span.SetAttributes(
		attribute.Int("purchase.summary.year", year),
		attribute.Int("purchase.summary.month", month),
		attribute.String("purchase.summary.period", string(plan.period)),
	)

	sums, err := s.repo.Summarize(ctx, userID, plan.ranges)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.Summary: %w", err)
	}
	if sums == nil {
		err := fmt.Errorf("purchaseService.Summary: repository returned no sums")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	previous := sums.PreviousAmount
	if !plan.ranges.HasPrevious {
		previous = decimal.Zero
	}
	result, percent := compare(sums.CurrentAmount, previous)

	return &MonthSummary{
		Year:          year,
		Month:         month,
		PurchaseCount: sums.MonthCount,
		Spent:         sums.MonthSpent,
		Comparison: Comparison{
			Period:         plan.period,
			UntilDay:       plan.untilDay,
			PreviousYear:   plan.previousYear,
			PreviousMonth:  plan.previousMonth,
			CurrentAmount:  sums.CurrentAmount,
			PreviousAmount: previous,
			Result:         result,
			Percent:        percent,
		},
	}, nil
}

// Update validates and applies changes to an existing purchase entry.
// Returns apperrors.ErrNotFound if no record with the given ID exists.
func (s *Service) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Update")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("purchase.id", id))

	if req == nil {
		return nil, apperrors.NewValidationError("request is required")
	}
	if err := validateRequest(&req.PurchaseRequest); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := s.checkUpdateReferences(ctx, userID, id, req); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	p, err := s.repo.Update(ctx, userID, id, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.Update: %w", err)
	}
	if p == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	span.SetAttributes(
		attribute.String("purchase.status", string(p.Status)),
		attribute.Int64("purchase.payment_method_id", p.PaymentMethod.ID),
	)

	s.purchasesUpdated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", string(p.Status)),
		),
	)

	return p, nil
}

// Delete removes the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *Service) Delete(ctx context.Context, id int64) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Delete")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("purchase.id", id))

	found, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("purchaseService.Delete: %w", err)
	}
	if !found {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}

	s.purchasesDeleted.Add(ctx, 1)

	return nil
}

// validateRequest checks all required fields for create and update operations.
// maxAmount is the largest value the amount column holds (NUMERIC(10, 2), migration 000005). Larger values
// would fail in PostgreSQL with a numeric overflow (500) instead of a validation error.
var maxAmount = decimal.RequireFromString("99999999.99")

// maxDescriptionLength is counted in characters (runes), after trimming.
const maxDescriptionLength = 255

// validateRequest checks the input and normalizes it in place: the description is trimmed, as category and
// payment method names are.
func validateRequest(req *PurchaseRequest) error {
	if req.PurchaseDate.IsZero() {
		return apperrors.NewValidationError("purchase_date is required")
	}
	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" {
		return apperrors.NewValidationError("description is required")
	}
	if utf8.RuneCountInString(req.Description) > maxDescriptionLength {
		return apperrors.NewValidationError("description must be at most %d characters", maxDescriptionLength)
	}
	if req.CategoryID <= 0 {
		return apperrors.NewValidationError("category_id must be a positive integer")
	}
	if req.PaymentMethodID <= 0 {
		return apperrors.NewValidationError("payment_method_id must be a positive integer")
	}
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return apperrors.NewValidationError("amount must be greater than zero")
	}
	// The column would round extra decimals silently.
	if !req.Amount.Equal(req.Amount.Round(2)) {
		return apperrors.NewValidationError("amount must have at most 2 decimal places")
	}
	if req.Amount.GreaterThan(maxAmount) {
		return apperrors.NewValidationError("amount must be at most %s", maxAmount.StringFixed(2))
	}
	if req.Status == "" {
		return apperrors.NewValidationError("status is required")
	}
	if !req.Status.IsValid() {
		return apperrors.NewValidationError(
			"status %q is not valid; allowed values: PENDING, PAID, CANCELED",
			req.Status,
		)
	}
	return nil
}

// checkUpdateReferences checks the category and payment method of an update, in the contract order
// (CATEGORY_NOT_FOUND before PAYMENT_METHOD_NOT_FOUND). An archived reference is accepted only when it
// is the one the purchase already has (ADR-11 of feature 006). The current purchase is read lazily, at
// most once, and only when a reference is archived; between that read and the UPDATE the purchase may
// change in another tab, and the last write wins.
func (s *Service) checkUpdateReferences(ctx context.Context, userID, id int64, req *UpdatePurchaseRequest) error {
	var current *Purchase
	loaded := false
	loadCurrent := func() error {
		if loaded {
			return nil
		}
		p, err := s.repo.GetByID(ctx, userID, id)
		if err != nil {
			return fmt.Errorf("purchaseService.Update: %w", err)
		}
		current, loaded = p, true
		return nil
	}

	catArchived, err := s.validateCategoryAccess(ctx, req.CategoryID, userID)
	if err != nil {
		return err
	}
	if catArchived {
		if err := loadCurrent(); err != nil {
			return err
		}
		if current == nil || current.Category.ID != req.CategoryID {
			return category.ErrCategoryNotFound
		}
	}

	pmArchived, err := s.validatePaymentMethodAccess(ctx, req.PaymentMethodID, userID)
	if err != nil {
		return err
	}
	if pmArchived {
		if err := loadCurrent(); err != nil {
			return err
		}
		if current == nil || current.PaymentMethod.ID != req.PaymentMethodID {
			return paymentmethod.ErrPaymentMethodNotFound
		}
	}
	return nil
}

// validateCategoryAccess ensures the category exists and is accessible by userID, and reports whether it
// is archived. Returns category.ErrCategoryNotFound when the category does not exist or belongs to
// another user, so existence of other users' categories is never leaked. "archived" is accepted only by
// Update, when it is the category the purchase already has (ADR-11 of feature 006).
func (s *Service) validateCategoryAccess(ctx context.Context, categoryID, userID int64) (archived bool, err error) {
	cat, err := s.categories.GetByID(ctx, categoryID)
	if err != nil {
		return false, fmt.Errorf("purchaseService.validateCategoryAccess: %w", err)
	}
	if cat == nil || !cat.AccessibleBy(userID) {
		return false, category.ErrCategoryNotFound
	}
	return cat.IsArchived(), nil
}

// validatePaymentMethodAccess ensures the payment method exists and is accessible by userID, and reports
// whether it is archived. Returns paymentmethod.ErrPaymentMethodNotFound when it does not exist or belongs
// to another user, so existence of other users' payment methods is never leaked. "archived" is accepted
// only by Update, when it is the payment method the purchase already has (ADR-11 of feature 006).
func (s *Service) validatePaymentMethodAccess(ctx context.Context, paymentMethodID, userID int64) (archived bool, err error) {
	pm, err := s.paymentMethods.GetByID(ctx, paymentMethodID)
	if err != nil {
		return false, fmt.Errorf("purchaseService.validatePaymentMethodAccess: %w", err)
	}
	if pm == nil || !pm.AccessibleBy(userID) {
		return false, paymentmethod.ErrPaymentMethodNotFound
	}
	return pm.IsArchived(), nil
}
