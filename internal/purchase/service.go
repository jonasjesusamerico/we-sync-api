package purchase

import (
	"context"
	"fmt"

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
	if err := s.validateCategoryAccess(ctx, req.CategoryID, userID); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := s.validatePaymentMethodAccess(ctx, req.PaymentMethodID, userID); err != nil {
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

	span.SetAttributes(
		attribute.Int("purchase.filter.page", filter.Page),
		attribute.Int("purchase.filter.page_size", filter.PageSize),
		attribute.Int("purchase.filter.month", filter.Month),
		attribute.Int("purchase.filter.year", filter.Year),
		attribute.String("purchase.filter.status", string(filter.Status)),
		attribute.Int64("purchase.filter.payment_method_id", filter.PaymentMethodID),
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
	if err := s.validateCategoryAccess(ctx, req.CategoryID, userID); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := s.validatePaymentMethodAccess(ctx, req.PaymentMethodID, userID); err != nil {
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
func validateRequest(req *PurchaseRequest) error {
	if req.PurchaseDate.IsZero() {
		return apperrors.NewValidationError("purchase_date is required")
	}
	if req.Description == "" {
		return apperrors.NewValidationError("description is required")
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

// validateCategoryAccess ensures the category exists and is accessible by userID.
// Returns category.ErrCategoryNotFound when the category does not exist or belongs to
// another user, so existence of other users' categories is never leaked.
func (s *Service) validateCategoryAccess(ctx context.Context, categoryID, userID int64) error {
	cat, err := s.categories.GetByID(ctx, categoryID)
	if err != nil {
		return fmt.Errorf("purchaseService.validateCategoryAccess: %w", err)
	}
	if cat == nil || !cat.AccessibleBy(userID) {
		return category.ErrCategoryNotFound
	}
	return nil
}

// validatePaymentMethodAccess ensures the payment method exists and is accessible by userID.
// Returns paymentmethod.ErrPaymentMethodNotFound when it does not exist or belongs to
// another user, so existence of other users' payment methods is never leaked.
func (s *Service) validatePaymentMethodAccess(ctx context.Context, paymentMethodID, userID int64) error {
	pm, err := s.paymentMethods.GetByID(ctx, paymentMethodID)
	if err != nil {
		return fmt.Errorf("purchaseService.validatePaymentMethodAccess: %w", err)
	}
	if pm == nil || !pm.AccessibleBy(userID) {
		return paymentmethod.ErrPaymentMethodNotFound
	}
	return nil
}
