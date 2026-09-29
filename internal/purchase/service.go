package purchase

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/shopspring/decimal"
)

const tracerName = "we-sync-api/purchase"
const meterName = "we-sync-api/purchase"

// PurchaseService defines the inbound port (business logic) for purchase entries.
type PurchaseService interface {
	Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, id int64) (*Purchase, error)
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, id int64) error
}

type purchaseService struct {
	repo PurchaseRepository

	// Business metrics — recorded after each successful mutation.
	purchasesCreated metric.Int64Counter
	purchasesUpdated metric.Int64Counter
	purchasesDeleted metric.Int64Counter
}

// NewPurchaseService creates a new PurchaseService backed by the given repository.
// It registers the business metrics against the global MeterProvider.
func NewPurchaseService(repo PurchaseRepository) PurchaseService {
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

	return &purchaseService{
		repo:             repo,
		purchasesCreated: purchasesCreated,
		purchasesUpdated: purchasesUpdated,
		purchasesDeleted: purchasesDeleted,
	}
}

// Create validates and persists a new purchase entry.
func (s *purchaseService) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Create")
	defer span.End()

	if err := validateRequest(&req.PurchaseRequest); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	p, err := s.repo.Create(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("purchaseService.Create: %w", err)
	}

	span.SetAttributes(
		attribute.Int64("purchase.id", p.ID),
		attribute.String("purchase.status", string(p.Status)),
		attribute.String("purchase.payment_method", string(p.PaymentMethod)),
	)

	s.purchasesCreated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", string(p.Status)),
			attribute.String("payment_method", string(p.PaymentMethod)),
		),
	)

	return p, nil
}

// GetByID returns the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *purchaseService) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.GetByID")
	defer span.End()

	span.SetAttributes(attribute.Int64("purchase.id", id))

	p, err := s.repo.GetByID(ctx, id)
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
// Applies default values: Page=1, PageSize=20 when not provided.
// Enforces a maximum of MaxPageSize items per page.
func (s *purchaseService) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.List")
	defer span.End()

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
		attribute.String("purchase.filter.payment_method", string(filter.PaymentMethod)),
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
func (s *purchaseService) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Update")
	defer span.End()

	span.SetAttributes(attribute.Int64("purchase.id", id))

	if err := validateRequest(&req.PurchaseRequest); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	p, err := s.repo.Update(ctx, id, req)
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
		attribute.String("purchase.payment_method", string(p.PaymentMethod)),
	)

	s.purchasesUpdated.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", string(p.Status)),
			attribute.String("payment_method", string(p.PaymentMethod)),
		),
	)

	return p, nil
}

// Delete removes the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *purchaseService) Delete(ctx context.Context, id int64) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "purchaseService.Delete")
	defer span.End()

	span.SetAttributes(attribute.Int64("purchase.id", id))

	found, err := s.repo.Delete(ctx, id)
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
	if req.Sector == "" {
		return apperrors.NewValidationError("sector is required")
	}
	if req.PaymentMethod == "" {
		return apperrors.NewValidationError("payment_method is required")
	}
	if !req.PaymentMethod.IsValid() {
		return apperrors.NewValidationError(
			"payment_method %q is not valid; allowed values: CASH, CREDIT_CARD, DEBIT_CARD, PIX, BANK_TRANSFER, BOLETO",
			req.PaymentMethod,
		)
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
