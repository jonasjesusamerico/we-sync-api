package category

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

const tracerName = "we-sync-api/category"

// maxNameRunes is the maximum name length, in characters (runes) after trimming; old names may be longer
// and stay valid until edited (RN-12 of feature 006).
const maxNameRunes = 40

// Service implements the category business rules.
type Service struct {
	repo Repository
}

// NewService creates a new Service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// validateInput checks name, icon and color in the contract order and returns the trimmed name or the
// first validation error.
func validateInput(name string, icon Icon, color Color) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperrors.NewValidationError("name is required")
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return "", apperrors.NewValidationError("name must be at most %d characters", maxNameRunes)
	}
	if icon == "" {
		return "", apperrors.NewValidationError("icon is required")
	}
	if !icon.IsValid() {
		return "", apperrors.NewValidationError(
			"icon %q is not valid; allowed values: TAG, HEART, SMILE, BOOK, CAR, HOUSE, FOOD, CARD, GIFT, SPARKLE", icon)
	}
	if color == "" {
		return "", apperrors.NewValidationError("color is required")
	}
	if !color.IsValid() {
		return "", apperrors.NewValidationError(
			"color %q is not valid; allowed values: FOOD, TRANSPORT, HOME, HEALTH, EDUCATION, LEISURE", color)
	}
	return name, nil
}

// Create validates and persists a new personal category for the authenticated user, or reactivates the
// user's archived category with the same name. The bool is true when a category was created and false
// when an archived one was reactivated.
func (s *Service) Create(ctx context.Context, req *CreateCategoryRequest) (*Category, bool, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.Create")
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
	name, err := validateInput(req.Name, req.Icon, req.Color)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}
	req.Name = name

	// Prevent creating a personal category with the same name as a global one.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	c, inserted, err := s.repo.Create(ctx, userID, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, ErrNameConflict.Error())
			return nil, false, ErrNameConflict
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, fmt.Errorf("categoryService.Create: %w", err)
	}
	if c == nil {
		// An active own category already has this name.
		span.SetStatus(codes.Error, ErrNameConflict.Error())
		return nil, false, ErrNameConflict
	}

	span.SetAttributes(attribute.Int64("category.id", c.ID), attribute.Bool("category.reactivated", !inserted))
	return c, inserted, nil
}

// List returns all categories visible to the authenticated user, archived ones included.
func (s *Service) List(ctx context.Context) ([]Category, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.List")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}

	items, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryService.List: %w", err)
	}

	span.SetAttributes(attribute.Int("category.result.count", len(items)))
	return items, nil
}

// Update replaces the name, icon and color of a personal category (active or archived) owned by the
// authenticated user.
// Returns apperrors.ErrNotFound if the category does not exist or belongs to another user,
// apperrors.ErrForbidden if it is global, and ErrDefaultNameConflict, ErrNameConflict or
// ErrArchivedNameConflict when the name is taken.
func (s *Service) Update(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.Update")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("category.id", id))

	if req == nil {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	name, err := validateInput(req.Name, req.Icon, req.Color)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	req.Name = name

	if err := s.checkOwned(ctx, userID, id); err != nil {
		recordOwnershipError(span, err)
		return nil, err
	}

	// Prevent renaming to a name that collides with a global category.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Renaming to its own name (changing only case or accents) does not conflict: the unique index
	// compares with other rows.
	c, err := s.repo.Update(ctx, userID, id, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			err = s.ownNameConflict(ctx, userID, req.Name)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryService.Update: %w", err)
	}
	if c == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	return c, nil
}

// Archive archives a personal category owned by the authenticated user. Idempotent.
func (s *Service) Archive(ctx context.Context, id int64) error {
	return s.setArchived(ctx, "categoryService.Archive", id, true)
}

// Unarchive reactivates a personal category owned by the authenticated user. Idempotent.
func (s *Service) Unarchive(ctx context.Context, id int64) error {
	return s.setArchived(ctx, "categoryService.Unarchive", id, false)
}

// setArchived is the shared flow of Archive and Unarchive.
// Returns apperrors.ErrNotFound if the category does not exist or belongs to another user, and
// apperrors.ErrForbidden if it is global.
func (s *Service) setArchived(ctx context.Context, spanName string, id int64, archived bool) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, spanName)
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("category.id", id))

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

// checkOwned verifies the category exists and is a personal one of userID.
// Returns apperrors.ErrNotFound if it does not exist or belongs to another user (no existence leak),
// and apperrors.ErrForbidden if it is global.
func (s *Service) checkOwned(ctx context.Context, userID, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("categoryService.checkOwned: %w", err)
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

// checkGlobalNameCollision returns ErrDefaultNameConflict if a global category has the same name key
// (case- and accent-insensitive). Uniqueness among personal categories is enforced by the DB.
// It is check-then-act, tolerated because global categories only change through migrations.
func (s *Service) checkGlobalNameCollision(ctx context.Context, name string) error {
	exists, err := s.repo.ExistsGlobalByName(ctx, name)
	if err != nil {
		return fmt.Errorf("categoryService.checkGlobalNameCollision: %w", err)
	}
	if exists {
		return ErrDefaultNameConflict
	}
	return nil
}

// ownNameConflict tells, after a unique violation on update, whether the other own category with the
// same name is archived (ErrArchivedNameConflict) or active (ErrNameConflict). If that row is gone in
// the meantime, it still answers ErrNameConflict.
func (s *Service) ownNameConflict(ctx context.Context, userID int64, name string) error {
	other, err := s.repo.FindOwnByName(ctx, userID, name)
	if err != nil {
		return fmt.Errorf("categoryService.ownNameConflict: %w", err)
	}
	if other != nil && other.IsArchived() {
		return ErrArchivedNameConflict
	}
	return ErrNameConflict
}
