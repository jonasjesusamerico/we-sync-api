package category

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

const tracerName = "we-sync-api/category"

// Service implements the category business rules.
type Service struct {
	repo Repository
}

// NewService creates a new Service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and persists a new personal category for the authenticated user.
func (s *Service) Create(ctx context.Context, req *CreateCategoryRequest) (*Category, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.Create")
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

	// Prevent creating a personal category with the same name as a global one.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	c, err := s.repo.Create(ctx, userID, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryService.Create: %w", err)
	}

	span.SetAttributes(attribute.Int64("category.id", c.ID))
	return c, nil
}

// List returns all categories visible to the authenticated user.
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

// Update renames a personal category owned by the authenticated user.
// Returns apperrors.ErrNotFound if the category does not exist or belongs to another user.
// Returns apperrors.ErrForbidden if the category is global.
func (s *Service) Update(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.Update")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("category.id", id))

	if req == nil || strings.TrimSpace(req.Name) == "" {
		err := apperrors.NewValidationError("name is required")
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)

	// Verify the category exists and is not global before attempting the update.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("categoryService.Update: %w", err)
	}
	if existing == nil {
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}
	if existing.IsGlobal {
		span.SetStatus(codes.Error, "forbidden: global category")
		return nil, apperrors.ErrForbidden
	}
	if !existing.AccessibleBy(userID) {
		// Category belongs to another user — treat as not found to avoid leaking existence.
		span.SetStatus(codes.Error, "not found")
		return nil, apperrors.ErrNotFound
	}

	// Prevent renaming to a name that collides with a global category.
	if err := s.checkGlobalNameCollision(ctx, req.Name); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	c, err := s.repo.Update(ctx, userID, id, req)
	if err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
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

	span.SetAttributes(attribute.String("category.name", c.Name))
	return c, nil
}

// Delete removes a personal category owned by the authenticated user.
// Returns apperrors.ErrNotFound if the category does not exist or belongs to another user.
// Returns apperrors.ErrForbidden if the category is global.
// Returns apperrors.ErrConflict if the category is referenced by purchase entries.
func (s *Service) Delete(ctx context.Context, id int64) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "categoryService.Delete")
	defer span.End()

	userID, ok := identity.UserIDFromContext(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	span.SetAttributes(attribute.Int64("category.id", id))

	// Verify existence and global status before attempting delete.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("categoryService.Delete: %w", err)
	}
	if existing == nil {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}
	if existing.IsGlobal {
		span.SetStatus(codes.Error, "forbidden: global category")
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
		return fmt.Errorf("categoryService.Delete: %w", err)
	}
	if !found {
		span.SetStatus(codes.Error, "not found")
		return apperrors.ErrNotFound
	}

	return nil
}

// checkGlobalNameCollision returns ErrConflict if a global category has the same
// name (case-insensitive). Uniqueness among personal categories is enforced by the DB.
func (s *Service) checkGlobalNameCollision(ctx context.Context, name string) error {
	exists, err := s.repo.ExistsGlobalByName(ctx, name)
	if err != nil {
		return fmt.Errorf("categoryService.checkGlobalNameCollision: %w", err)
	}
	if exists {
		return apperrors.ErrConflict
	}
	return nil
}
