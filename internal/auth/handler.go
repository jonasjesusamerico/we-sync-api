package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// service is the behaviour of *Service the handler depends on. It is declared
// here, at the consumer, so handler tests can substitute a fake.
type service interface {
	Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error)
	Refresh(ctx context.Context, req *RefreshRequest) (*TokenResponse, error)
	Logout(ctx context.Context, req *RefreshRequest) error
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// Register creates a new user.
// @Summary Register a user
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "Registration data"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} httputil.APIError
// @Failure 409 {object} httputil.APIError
// @Failure 422 {object} httputil.APIError
// @Router /auth/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	response, err := h.svc.Register(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, response)
}

// Login authenticates a user and returns an access and refresh token.
// @Summary Authenticate a user
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "Login credentials"
// @Success 200 {object} TokenResponse
// @Failure 400 {object} httputil.APIError
// @Failure 401 {object} httputil.APIError
// @Failure 422 {object} httputil.APIError
// @Router /auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httputil.WriteValidationError(w, "email and password are required")
		return
	}

	response, err := h.svc.Login(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// Refresh rotates a refresh token and returns a new token pair.
// @Summary Refresh authentication tokens
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RefreshRequest true "Refresh token"
// @Success 200 {object} TokenResponse
// @Failure 400 {object} httputil.APIError
// @Failure 401 {object} httputil.APIError
// @Router /auth/refresh [post]
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		httputil.WriteBadRequest(w, "refresh_token is required")
		return
	}

	response, err := h.svc.Refresh(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// Logout revokes a refresh token.
// @Summary Revoke a refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RefreshRequest true "Refresh token"
// @Success 204
// @Failure 400 {object} httputil.APIError
// @Router /auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}
	if err := h.svc.Logout(r.Context(), &req); err != nil {
		h.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrValidation):
		httputil.WriteValidationError(w, err.Error())
	case errors.Is(err, apperrors.ErrUnauthorized):
		httputil.WriteUnauthorized(w, err.Error())
	case errors.Is(err, apperrors.ErrConflict):
		httputil.WriteConflict(w, "email already registered")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, err.Error())
	default:
		logger.FromContext(r.Context()).Error("internal error in auth handler", "error", err)
		httputil.WriteInternalError(w)
	}
}
