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
	Me(ctx context.Context) (*UserResponse, error)
	UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*UserResponse, error)
	UpdatePreferences(ctx context.Context, req *UpdatePreferencesRequest) (*UserResponse, error)
	ChangePassword(ctx context.Context, req *ChangePasswordRequest) (*TokenResponse, error)
	DeleteAccount(ctx context.Context, req *DeleteAccountRequest) error
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// Register creates a new user.
// @Summary Register a user
// @Description Creates a user. name is trimmed and must have 2 to 60 characters; email is trimmed and lowercased.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "Registration data"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} httputil.APIError "Invalid request body"
// @Failure 409 {object} httputil.APIError "Email already registered"
// @Failure 422 {object} httputil.APIError "Validation error (name, email or password)"
// @Failure 500 {object} httputil.APIError "Internal server error"
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

// Me returns the authenticated user.
// @Summary Get the authenticated user
// @Description Returns the id, name, e-mail and theme of the user identified by the access token.
// @Description theme is LIGHT, DARK or null when never chosen in the account.
// @Tags users
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} httputil.APIError "Missing or invalid authentication token"
// @Failure 404 {object} httputil.APIError "User not found"
// @Failure 500 {object} httputil.APIError "Internal server error"
// @Router /users/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	response, err := h.svc.Me(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// UpdateProfile changes the authenticated user's name.
// @Summary Update the profile
// @Description Replaces the name (trimmed, 2 to 60 characters). The e-mail never changes: an email field in the body is ignored.
// @Tags users
// @Accept json
// @Produce json
// @Param body body UpdateProfileRequest true "Profile data"
// @Success 200 {object} UserResponse
// @Failure 400 {object} httputil.APIError "Invalid request body"
// @Failure 401 {object} httputil.APIError "Missing or invalid authentication token"
// @Failure 422 {object} httputil.APIError "Validation error"
// @Failure 500 {object} httputil.APIError "Internal server error"
// @Router /users/me/profile [put]
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	response, err := h.svc.UpdateProfile(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// UpdatePreferences saves the authenticated user's theme.
// @Summary Update the preferences
// @Description Saves the theme in the account (LIGHT or DARK).
// @Tags users
// @Accept json
// @Produce json
// @Param body body UpdatePreferencesRequest true "Preferences"
// @Success 200 {object} UserResponse
// @Failure 400 {object} httputil.APIError "Invalid request body"
// @Failure 401 {object} httputil.APIError "Missing or invalid authentication token"
// @Failure 422 {object} httputil.APIError "Validation error"
// @Failure 500 {object} httputil.APIError "Internal server error"
// @Router /users/me/preferences [put]
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	response, err := h.svc.UpdatePreferences(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// ChangePassword replaces the authenticated user's password.
// @Summary Change the password
// @Description Checks current_password and stores new_password (8 to 72 bytes, never trimmed, different from the current one).
// @Description Every other session ends: previous refresh tokens are revoked and previous access tokens get 401.
// @Description Returns a new token pair for the caller.
// @Tags users
// @Accept json
// @Produce json
// @Param body body ChangePasswordRequest true "Current and new password"
// @Success 200 {object} TokenResponse
// @Failure 400 {object} httputil.APIError "Invalid request body"
// @Failure 401 {object} httputil.APIError "Missing or invalid authentication token"
// @Failure 403 {object} httputil.APIError "INVALID_PASSWORD: the current password does not match"
// @Failure 422 {object} httputil.APIError "Validation error"
// @Failure 500 {object} httputil.APIError "Internal server error"
// @Router /users/me/password [put]
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	response, err := h.svc.ChangePassword(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, response)
}

// DeleteAccount deletes the authenticated user's account.
// @Summary Delete the account
// @Description Deletes the account and all its data (purchases, own categories and payment methods, sessions). It cannot be undone.
// @Description The password confirms the operation and is never trimmed.
// @Tags users
// @Accept json
// @Produce json
// @Param body body DeleteAccountRequest true "Password confirmation"
// @Success 204
// @Failure 400 {object} httputil.APIError "Invalid or missing request body"
// @Failure 401 {object} httputil.APIError "Missing or invalid authentication token"
// @Failure 403 {object} httputil.APIError "INVALID_PASSWORD: the password does not match"
// @Failure 422 {object} httputil.APIError "Validation error"
// @Failure 500 {object} httputil.APIError "Internal server error"
// @Router /users/me [delete]
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req DeleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// An empty body (io.EOF) is also a bad request: the password is required in the body.
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	if err := h.svc.DeleteAccount(r.Context(), &req); err != nil {
		h.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidPassword):
		httputil.WriteError(w, http.StatusForbidden, httputil.ErrCodeInvalidPassword, "invalid password")
	case errors.Is(err, apperrors.ErrValidation):
		httputil.WriteValidationError(w, err.Error())
	case errors.Is(err, apperrors.ErrUnauthorized):
		httputil.WriteUnauthorized(w, err.Error())
	case errors.Is(err, apperrors.ErrConflict):
		httputil.WriteConflict(w, "email already registered")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, "user not found")
	default:
		logger.FromContext(r.Context()).Error("internal error in auth handler", "error", err)
		httputil.WriteInternalError(w)
	}
}
