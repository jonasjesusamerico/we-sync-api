package purchase

import (
	"context"
	"fmt"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

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
}

// NewPurchaseService creates a new PurchaseService backed by the given repository.
func NewPurchaseService(repo PurchaseRepository) PurchaseService {
	return &purchaseService{repo: repo}
}

// Create validates and persists a new purchase entry.
func (s *purchaseService) Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	p, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("purchaseService.Create: %w", err)
	}

	return p, nil
}

// GetByID returns the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *purchaseService) GetByID(ctx context.Context, id int64) (*Purchase, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("purchaseService.GetByID: %w", err)
	}
	if p == nil {
		return nil, apperrors.ErrNotFound
	}

	return p, nil
}

// List returns a paginated and filtered list of purchase entries.
// Applies default values: Page=1, PageSize=20 when not provided.
func (s *purchaseService) List(ctx context.Context, filter ListFilter) (*PaginatedResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	result, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("purchaseService.List: %w", err)
	}

	return result, nil
}

// Update validates and applies changes to an existing purchase entry.
// Returns apperrors.ErrNotFound if no record with the given ID exists.
func (s *purchaseService) Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	p, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("purchaseService.Update: %w", err)
	}
	if p == nil {
		return nil, apperrors.ErrNotFound
	}

	return p, nil
}

// Delete removes the purchase entry with the given ID.
// Returns apperrors.ErrNotFound if no record exists.
func (s *purchaseService) Delete(ctx context.Context, id int64) error {
	found, err := s.repo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("purchaseService.Delete: %w", err)
	}
	if !found {
		return apperrors.ErrNotFound
	}

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
	if !IsValidPaymentMethod(req.PaymentMethod) {
		return apperrors.NewValidationError("payment_method %q is not valid; allowed values: CASH, CREDIT_CARD, DEBIT_CARD, PIX, BANK_TRANSFER, BOLETO", req.PaymentMethod)
	}
	if req.Amount <= 0 {
		return apperrors.NewValidationError("amount must be greater than zero")
	}
	if req.Status == "" {
		return apperrors.NewValidationError("status is required")
	}
	if !IsValidStatus(req.Status) {
		return apperrors.NewValidationError("status %q is not valid; allowed values: PENDING, PAID, CANCELED", req.Status)
	}

	return nil
}
