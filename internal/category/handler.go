package category

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// CategoryHandler handles HTTP requests for the category domain.
type CategoryHandler struct {
	svc CategoryService
}

// NewCategoryHandler creates a new CategoryHandler with the given service.
func NewCategoryHandler(svc CategoryService) *CategoryHandler {
	return &CategoryHandler{svc: svc}
}

// Create handles POST /categories
//
// @Summary     Create a personal category
// @Description Creates a new personal category for the authenticated user.
// @Tags        categories
// @Accept      json
// @Produce     json
// @Param       body body     CreateCategoryRequest true "Category data"
// @Success     201  {object} Category
// @Failure     400  {object} httputil.APIError "Invalid request body"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     409  {object} httputil.APIError "Name already exists"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /categories [post]
func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	c, err := h.svc.Create(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, c)
}

// List handles GET /categories
//
// @Summary     List categories
// @Description Returns all categories visible to the authenticated user (global + personal).
// @Tags        categories
// @Produce     json
// @Success     200 {array}  Category
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /categories [get]
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
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
// @Description Renames a personal category owned by the authenticated user.
// @Tags        categories
// @Accept      json
// @Produce     json
// @Param       id   path     int                  true "Category ID"
// @Param       body body     UpdateCategoryRequest true "Updated category data"
// @Success     200  {object} Category
// @Failure     400  {object} httputil.APIError "Invalid request body or ID"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     403  {object} httputil.APIError "Cannot modify a global category"
// @Failure     404  {object} httputil.APIError "Not found"
// @Failure     409  {object} httputil.APIError "Name already exists"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /categories/{id} [put]
func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
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

// Delete handles DELETE /categories/{id}
//
// @Summary     Delete a personal category
// @Description Removes a personal category owned by the authenticated user.
//
//	Fails with 409 if the category is referenced by purchase entries.
//
// @Tags        categories
// @Produce     json
// @Param       id  path int true "Category ID"
// @Success     204 "No Content"
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     403 {object} httputil.APIError "Cannot delete a global category"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     409 {object} httputil.APIError "Category in use by purchase entries"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /categories/{id} [delete]
func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleError maps service-layer errors to standardised HTTP responses.
func (h *CategoryHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrUnauthorized):
		httputil.WriteUnauthorized(w, "authentication required")
	case errors.Is(err, apperrors.ErrForbidden):
		httputil.WriteForbidden(w, "global categories cannot be modified or deleted")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, "category not found")
	case errors.Is(err, apperrors.ErrConflict):
		httputil.WriteConflict(w, "category name already exists or is in use by purchase entries")
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
