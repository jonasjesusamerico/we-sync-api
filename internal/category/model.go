package category

import (
	"fmt"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

// ErrCategoryNotFound is returned when a category does not exist or is not accessible
// by the caller. It wraps apperrors.ErrNotFound so generic not-found handling still applies.
var ErrCategoryNotFound = fmt.Errorf("category %w", apperrors.ErrNotFound)

// Category represents a classification label used to tag purchase entries.
// Global categories have UserID == nil and cannot be modified or deleted by users.
// Personal categories belong to a specific user (UserID != nil).
type Category struct {
	ID        int64     `json:"id"`
	UserID    *int64    `json:"user_id,omitempty"`
	Name      string    `json:"name"`
	IsGlobal  bool      `json:"is_global"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccessibleBy reports whether the category is global or owned by userID.
func (c *Category) AccessibleBy(userID int64) bool {
	return c.IsGlobal || (c.UserID != nil && *c.UserID == userID)
}

// CategorySummary is the embedded representation of a category inside a Purchase response.
type CategorySummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsGlobal bool   `json:"is_global"`
}

// CreateCategoryRequest is the input type for creating a personal category.
type CreateCategoryRequest struct {
	Name string `json:"name"`
}

// UpdateCategoryRequest is the input type for renaming a personal category.
type UpdateCategoryRequest struct {
	Name string `json:"name"`
}
