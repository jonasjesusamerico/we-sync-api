// Package apperrors defines the sentinel errors shared across all application domains.
package apperrors

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers should use errors.Is to check for these.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrValidation is returned when input data fails business rule validation.
	ErrValidation = errors.New("validation error")

	// ErrUnauthorized is returned when authentication is required or invalid.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when an authenticated caller lacks permission.
	ErrForbidden = errors.New("forbidden")

	// ErrConflict is returned when an operation conflicts with existing state.
	ErrConflict = errors.New("conflict")
)

// ValidationError wraps ErrValidation with a human-readable message.
// It satisfies errors.Is(err, ErrValidation) via Unwrap.
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// NewValidationError creates a ValidationError with the given formatted message.
func NewValidationError(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}
