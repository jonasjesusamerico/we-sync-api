package paymentmethod

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
)

const tracerName = "we-sync-api/paymentmethod"

// Service implements the payment method business rules.
type Service struct {
	repo Repository
}

// NewService creates a new Service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and persists a new personal payment method for the authenticated user.
func (s *Service) Create(ctx context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.Create")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	if req == nil || strings.TrimSpace(req.Name) == "" {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)

	// Prevent creating a personal payment method with the same name as a global one.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	pm, err := s.repo.Create(ctx, userID, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodService.Create: %w", err)
	}

	span.SetAttributes(attribute.Int64("payment_method.id", pm.ID))
	return pm, nil
}

// List returns all payment methods visible to the authenticated user.
func (s *Service) List(ctx context.Context) ([]PaymentMethod, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.List")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}

	items, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodService.List: %w", err)
	}

	span.SetAttributes(attribute.Int("payment_method.result.count", len(items)))
	return items, nil
}

// Update renames a personal payment method owned by the authenticated user.
// Returns apperrors.ErrNotFound if the payment method does not exist or belongs to another user.
// Returns apperrors.ErrForbidden if the payment method is global.
func (s *Service) Update(ctx context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.Update")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("payment_method.id", id))

	if req == nil || strings.TrimSpace(req.Name) == "" {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)

	// Verify the payment method exists and is not global before attempting the update.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodService.Update: %w", err)
	}
	if existing == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}
	if existing.IsGlobal {
		span.SetStatus(codes.Error, "forbidden: global payment method")
		return nil, apperrors.ErrForbidden
	}
	if !existing.AccessibleBy(userID) {
		// PaymentMethod belongs to another user — treat as not found to avoid leaking existence.
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	// Prevent renaming to a name that collides with a global payment method.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	pm, err := s.repo.Update(ctx, userID, id, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("paymentMethodService.Update: %w", err)
	}
	if pm == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	span.SetAttributes(attribute.String("payment_method.name", pm.Name))
	return pm, nil
}

// Delete removes a personal payment method owned by the authenticated user.
// Returns apperrors.ErrNotFound if the payment method does not exist or belongs to another user.
// Returns apperrors.ErrForbidden if the payment method is global.
// Returns apperrors.ErrConflict if the payment method is referenced by purchase entries.
func (s *Service) Delete(ctx context.Context, id int64) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.Delete")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("payment_method.id", id))

	// Verify existence and global status before attempting delete.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("paymentMethodService.Delete: %w", err)
	}
	if existing == nil {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}
	if existing.IsGlobal {
		span.SetStatus(codes.Error, "forbidden: global payment method")
		return apperrors.ErrForbidden
	}
	if !existing.AccessibleBy(userID) {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}

	found, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("paymentMethodService.Delete: %w", err)
	}
	if !found {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}

	return nil
}

// checkGlobalNameCollision returns ErrConflict if a global payment method has the same
// name (case-insensitive). Uniqueness among personal payment methods is enforced by the DB.
func (s *Service) checkGlobalNameCollision(ctx context.Context, name string) error {
	exists, err := s.repo.ExistsGlobalByName(ctx, name)
	if err != nil {
		return fmt.Errorf("paymentMethodService.checkGlobalNameCollision: %w", err)
	}
	if exists {
		return apperrors.ErrConflict
	}
	return nil
}
