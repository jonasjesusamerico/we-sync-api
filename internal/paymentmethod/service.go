package paymentmethod

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
)

const tracerName = "we-sync-api/paymentmethod"

// maxNameRunes is the maximum name length, in characters (runes) after trimming; old names may be longer
// and stay valid until edited (RN-12 of feature 006).
const maxNameRunes = 40

// Service implements the payment method business rules.
type Service struct {
	repo Repository
}

// NewService creates a new Service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// validateName trims the name and checks it in the contract order. It returns the trimmed name or the
// first validation error.
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperrors.NewValidationError("name is required")
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return "", apperrors.NewValidationError("name must be at most %d characters", maxNameRunes)
	}
	return name, nil
}

// Create validates and persists a new personal payment method for the authenticated user, or reactivates the
// user's archived payment method with the same name. The bool is true when a payment method was created and false
// when an archived one was reactivated.
func (s *Service) Create(ctx context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.Create")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, false, apperrors.ErrUnauthorized
	}
	if req == nil {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}
	name, err := validateName(req.Name)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}
	req.Name = name

	// Prevent creating a personal payment method with the same name as a global one.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	pm, inserted, err := s.repo.Create(ctx, userID, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, ErrNameConflict.Error())
			return nil, false, ErrNameConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, fmt.Errorf("paymentMethodService.Create: %w", err)
	}
	if pm == nil {
		// An active own payment method already has this name.
		span.SetStatus(codes.Error, ErrNameConflict.Error())
		return nil, false, ErrNameConflict
	}

	span.SetAttributes(attribute.Int64("payment_method.id", pm.ID), attribute.Bool("payment_method.reactivated", !inserted))
	return pm, inserted, nil
}

// List returns all payment methods visible to the authenticated user, archived ones included.
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

// Update renames a personal payment method (active or archived) owned by the authenticated user.
// Returns apperrors.ErrNotFound if the payment method does not exist or belongs to another user,
// apperrors.ErrForbidden if it is global, and ErrDefaultNameConflict, ErrNameConflict or
// ErrArchivedNameConflict when the name is taken.
func (s *Service) Update(ctx context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "paymentMethodService.Update")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("payment_method.id", id))

	if req == nil {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	name, err := validateName(req.Name)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	req.Name = name

	if err := s.checkOwned(ctx, userID, id); err != nil {
		recordOwnershipError(span, err)
		return nil, err
	}

	// Prevent renaming to a name that collides with a global payment method.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Renaming to its own name (changing only case or accents) does not conflict: the unique index
	// compares with other rows.
	pm, err := s.repo.Update(ctx, userID, id, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			err = s.ownNameConflict(ctx, userID, req.Name)
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

	return pm, nil
}

// Archive archives a personal payment method owned by the authenticated user. Idempotent.
func (s *Service) Archive(ctx context.Context, id int64) error {
	return s.setArchived(ctx, "paymentMethodService.Archive", id, true)
}

// Unarchive reactivates a personal payment method owned by the authenticated user. Idempotent.
func (s *Service) Unarchive(ctx context.Context, id int64) error {
	return s.setArchived(ctx, "paymentMethodService.Unarchive", id, false)
}

// setArchived is the shared flow of Archive and Unarchive.
// Returns apperrors.ErrNotFound if the payment method does not exist or belongs to another user, and
// apperrors.ErrForbidden if it is global.
func (s *Service) setArchived(ctx context.Context, spanName string, id int64, archived bool) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, spanName)
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("payment_method.id", id))

	if err := s.checkOwned(ctx, userID, id); err != nil {
		recordOwnershipError(span, err)
		return err
	}

	found, err := s.repo.SetArchived(ctx, userID, id, archived)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("%s: %w", spanName, err)
	}
	if !found {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}
	return nil
}

// checkOwned verifies the payment method exists and is a personal one of userID.
// Returns apperrors.ErrNotFound if it does not exist or belongs to another user (no existence leak),
// and apperrors.ErrForbidden if it is global.
func (s *Service) checkOwned(ctx context.Context, userID, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("paymentMethodService.checkOwned: %w", err)
	}
	if existing == nil {
		return apperrors.ErrNotFound
	}
	if existing.IsGlobal {
		return apperrors.ErrForbidden
	}
	if !existing.AccessibleBy(userID) {
		return apperrors.ErrNotFound
	}
	return nil
}

// recordOwnershipError marks the span for a checkOwned error; only unexpected errors are recorded as such.
func recordOwnershipError(span trace.Span, err error) {
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrForbidden) {
		span.RecordError(err)
	}
	span.SetStatus(codes.Error, err.Error())
}

// checkGlobalNameCollision returns ErrDefaultNameConflict if a global payment method has the same name key
// (case- and accent-insensitive). Uniqueness among personal payment methods is enforced by the DB.
// It is check-then-act, tolerated because global payment methods only change through migrations.
func (s *Service) checkGlobalNameCollision(ctx context.Context, name string) error {
	exists, err := s.repo.ExistsGlobalByName(ctx, name)
	if err != nil {
		return fmt.Errorf("paymentMethodService.checkGlobalNameCollision: %w", err)
	}
	if exists {
		return ErrDefaultNameConflict
	}
	return nil
}

// ownNameConflict tells, after a unique violation on update, whether the other own payment method with the
// same name is archived (ErrArchivedNameConflict) or active (ErrNameConflict). If that row is gone in
// the meantime, it still answers ErrNameConflict.
func (s *Service) ownNameConflict(ctx context.Context, userID int64, name string) error {
	other, err := s.repo.FindOwnByName(ctx, userID, name)
	if err != nil {
		return fmt.Errorf("paymentMethodService.ownNameConflict: %w", err)
	}
	if other != nil && other.IsArchived() {
		return ErrArchivedNameConflict
	}
	return ErrNameConflict
}
