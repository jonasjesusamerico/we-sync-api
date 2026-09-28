// Package httputil provides shared HTTP response helpers for all feature handlers.
package httputil

import (
	"encoding/json"
	"net/http"
)

// APIError is the standard error envelope returned by all API endpoints.
//
// Example JSON:
//
//	{"code": "VALIDATION_ERROR", "message": "amount must be greater than zero"}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Common error codes used across the API.
const (
	ErrCodeBadRequest          = "BAD_REQUEST"
	ErrCodeValidation          = "VALIDATION_ERROR"
	ErrCodeNotFound            = "NOT_FOUND"
	ErrCodeInternalServerError = "INTERNAL_SERVER_ERROR"
)

// WriteJSON encodes v as JSON and writes it to w with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a structured APIError JSON response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, APIError{
		Code:    code,
		Message: message,
	})
}

// WriteBadRequest writes a 400 Bad Request error response.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteValidationError writes a 422 Unprocessable Entity error response.
func WriteValidationError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidation, message)
}

// WriteNotFound writes a 404 Not Found error response.
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteInternalError writes a 500 Internal Server Error response.
func WriteInternalError(w http.ResponseWriter) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternalServerError, "internal server error")
}
