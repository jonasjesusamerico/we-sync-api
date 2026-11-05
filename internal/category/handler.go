package category

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// service is the behaviour of *Service the handler depends on. It is declared
// here, at the consumer, so handler tests can substitute a fake.
type service interface {
	Create(ctx context.Context, req *CreateCategoryRequest) (*Category, bool, error)
	List(ctx context.Context) ([]Category, error)
	Update(ctx context.Context, id int64, req *UpdateCategoryRequest) (*Category, error)
	Archive(ctx context.Context, id int64) error
	Unarchive(ctx context.Context, id int64) error
}

// Handler handles HTTP requests for the category domain.
type Handler struct {
	svc service
}

// NewHandler creates a new Handler with the given service.
func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /categories
//
// @Summary     Create a personal category
// @Description Creates a personal category for the authenticated user, or reactivates the user's archived
// @Description category with the same name (case- and accent-insensitive): the stored name is kept and the
// @Description icon and color sent are applied.
// @Tags        categories
// @Accept      json
// @Produce     json
// @Param       body body     CreateCategoryRequest true "Category data"
// @Success     201  {object} Category
// @Success     200  {object} Category          "Reactivated"
// @Failure     400  {object} httputil.APIError "Invalid request body"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     409  {object} httputil.APIError "DEFAULT_NAME_CONFLICT or NAME_CONFLICT"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /categories [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	c, created, err := h.svc.Create(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httputil.WriteJSON(w, status, c)
}

// List handles GET /categories
//
// @Summary     List categories
// @Description Returns all categories visible to the authenticated user (global + personal), archived ones
// @Description included; archived_at tells them apart.
// @Tags        categories
// @Produce     json
// @Success     200 {array}  Category
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /categories [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

// Update handles PUT /categories/{id}
//
// @Summary     Update a personal category
// @Description Replaces the name, icon and color of a personal category (active or archived) owned by the
// @Description authenticated user.
// @Tags        categories
// @Accept      json
// @Produce     json
// @Param       id   path     int                  true "Category ID"
// @Param       body body     UpdateCategoryRequest true "Updated category data"
// @Success     200  {object} Category
// @Failure     400  {object} httputil.APIError "Invalid request body or ID"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     403  {object} httputil.APIError "Cannot change a default category"
// @Failure     404  {object} httputil.APIError "Not found"
// @Failure     409  {object} httputil.APIError "DEFAULT_NAME_CONFLICT, NAME_CONFLICT or ARCHIVED_NAME_CONFLICT"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /categories/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	c, err := h.svc.Update(r.Context(), id, &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, c)
}

// Archive handles PUT /categories/{id}/archive
//
// @Summary     Archive a personal category
// @Description Archives a personal category. Its purchases do not change. Archiving an archived category
// @Description is a no-op (idempotent).
// @Tags        categories
// @Produce     json
// @Param       id  path int true "Category ID"
// @Success     204 "No Content"
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     403 {object} httputil.APIError "Cannot change a default category"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /categories/{id}/archive [put]
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, h.svc.Archive)
}

// Unarchive handles DELETE /categories/{id}/archive
//
// @Summary     Reactivate a personal category
// @Description Reactivates an archived personal category. Reactivating an active category is a no-op
// @Description (idempotent).
// @Tags        categories
// @Produce     json
// @Param       id  path int true "Category ID"
// @Success     204 "No Content"
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     403 {object} httputil.APIError "Cannot change a default category"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /categories/{id}/archive [delete]
func (h *Handler) Unarchive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, h.svc.Unarchive)
}

// setArchived is the shared flow of Archive and Unarchive: parse the id, call op, answer 204.
func (h *Handler) setArchived(w http.ResponseWriter, r *http.Request, op func(context.Context, int64) error) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := op(r.Context(), id); err != nil {
		h.handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleError maps service-layer errors to standardised HTTP responses.
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrUnauthorized):
		httputil.WriteUnauthorized(w, "authentication required")
	case errors.Is(err, apperrors.ErrForbidden):
		httputil.WriteForbidden(w, "default categories cannot be changed")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, "category not found")
	case errors.Is(err, ErrDefaultNameConflict):
		httputil.WriteError(w, http.StatusConflict, httputil.ErrCodeDefaultNameConflict,
			"name already used by a default category")
	case errors.Is(err, ErrArchivedNameConflict):
		httputil.WriteError(w, http.StatusConflict, httputil.ErrCodeArchivedNameConflict,
			"name already used by an archived category")
	case errors.Is(err, apperrors.ErrConflict):
		httputil.WriteError(w, http.StatusConflict, httputil.ErrCodeNameConflict,
			"name already used by another category")
	case errors.Is(err, apperrors.ErrValidation):
		httputil.WriteValidationError(w, err.Error())
	default:
		log := logger.FromContext(r.Context())
		log.Error("internal error in category handler", "error", err)
		httputil.WriteInternalError(w)
	}
}

// parseID extracts and validates the {id} URL parameter from the request.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httputil.WriteBadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}
