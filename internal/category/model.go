package category

import (
	"fmt"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
)

// ErrCategoryNotFound is returned when a category does not exist or is not accessible
// by the caller. It wraps apperrors.ErrNotFound so generic not-found handling still applies.
var ErrCategoryNotFound = fmt.Errorf("category %w", apperrors.ErrNotFound)

// Name conflicts (RN-13 of feature 006). They wrap apperrors.ErrConflict, so generic handling still applies;
// the handler maps each one to its own code.
var (
	ErrDefaultNameConflict  = fmt.Errorf("name of a default category: %w", apperrors.ErrConflict)
	ErrNameConflict         = fmt.Errorf("name of another category: %w", apperrors.ErrConflict)
	ErrArchivedNameConflict = fmt.Errorf("name of an archived category: %w", apperrors.ErrConflict)
)

// Icon is the icon of a category, chosen from the prototype's list (ADR-06 of feature 006).
type Icon string

const (
	IconTag     Icon = "TAG"
	IconHeart   Icon = "HEART"
	IconSmile   Icon = "SMILE"
	IconBook    Icon = "BOOK"
	IconCar     Icon = "CAR"
	IconHouse   Icon = "HOUSE"
	IconFood    Icon = "FOOD"
	IconCard    Icon = "CARD"
	IconGift    Icon = "GIFT"
	IconSparkle Icon = "SPARKLE"
)

// IsValid reports whether i is one of the ten icons.
func (i Icon) IsValid() bool {
	switch i {
	case IconTag, IconHeart, IconSmile, IconBook, IconCar, IconHouse, IconFood, IconCard, IconGift, IconSparkle:
		return true
	}
	return false
}

// Color is the color of a category: the name of a --ws-cat-* token of the front.
type Color string

const (
	ColorFood      Color = "FOOD"
	ColorTransport Color = "TRANSPORT"
	ColorHome      Color = "HOME"
	ColorHealth    Color = "HEALTH"
	ColorEducation Color = "EDUCATION"
	ColorLeisure   Color = "LEISURE"
	// ColorNeutral is the stored default of the own categories created before feature 006.
	// It is never accepted as input: IsValid returns false for it.
	ColorNeutral Color = "NEUTRAL"
)

// IsValid reports whether c is one of the six colors a user may choose (NEUTRAL is not).
func (c Color) IsValid() bool {
	switch c {
	case ColorFood, ColorTransport, ColorHome, ColorHealth, ColorEducation, ColorLeisure:
		return true
	}
	return false
}

// Category represents a classification label used to tag purchase entries.
// Global categories have UserID == nil, are never archived and cannot be changed by users.
// Personal categories belong to a specific user (UserID != nil).
type Category struct {
	ID         int64      `json:"id"`
	UserID     *int64     `json:"user_id,omitempty"`
	Name       string     `json:"name"`
	IsGlobal   bool       `json:"is_global"`
	Icon       Icon       `json:"icon"`
	Color      Color      `json:"color"`
	ArchivedAt *time.Time `json:"archived_at"` // null = active
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AccessibleBy reports whether the category is global or owned by userID.
func (c *Category) AccessibleBy(userID int64) bool {
	return c.IsGlobal || (c.UserID != nil && *c.UserID == userID)
}

// IsArchived reports whether the category is archived.
func (c *Category) IsArchived() bool { return c.ArchivedAt != nil }

// CategorySummary is the embedded representation of a category inside a Purchase response.
type CategorySummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsGlobal bool   `json:"is_global"`
	Icon     Icon   `json:"icon"`
	Color    Color  `json:"color"`
}

// CreateCategoryRequest is the input of POST /categories. All fields are required.
type CreateCategoryRequest struct {
	Name  string `json:"name"`
	Icon  Icon   `json:"icon"`
	Color Color  `json:"color"`
}

// UpdateCategoryRequest is the input of PUT /categories/{id}: it replaces name, icon and color.
type UpdateCategoryRequest struct {
	Name  string `json:"name"`
	Icon  Icon   `json:"icon"`
	Color Color  `json:"color"`
}
