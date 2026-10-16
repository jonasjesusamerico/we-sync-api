package paymentmethod

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
	Create(ctx context.Context, req *CreatePaymentMethodRequest) (*PaymentMethod, error)
	List(ctx context.Context) ([]PaymentMethod, error)
	Update(ctx context.Context, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error)
	Delete(ctx context.Context, id int64) error
}

// Handler handles HTTP requests for the payment method domain.
type Handler struct {
	svc service
}

// NewHandler creates a new Handler with the given service.
func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /payment-methods
//
// @Summary     Create a personal payment method
// @Description Creates a new personal payment method for the authenticated user.
// @Tags        payment-methods
// @Accept      json
// @Produce     json
// @Param       body body     CreatePaymentMethodRequest true "PaymentMethod data"
// @Success     201  {object} PaymentMethod
// @Failure     400  {object} httputil.APIError "Invalid request body"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     409  {object} httputil.APIError "Name already exists"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /payment-methods [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	pm, err := h.svc.Create(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, pm)
}

// List handles GET /payment-methods
//
// @Summary     List payment methods
// @Description Returns all payment methods visible to the authenticated user (global + personal).
// @Tags        payment-methods
// @Produce     json
// @Success     200 {array}  PaymentMethod
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /payment-methods [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

// Update handles PUT /payment-methods/{id}
//
// @Summary     Update a personal payment method
// @Description Renames a personal payment method owned by the authenticated user.
// @Tags        payment-methods
// @Accept      json
// @Produce     json
// @Param       id   path     int                  true "Payment method ID"
// @Param       body body     UpdatePaymentMethodRequest true "Updated payment method data"
// @Success     200  {object} PaymentMethod
// @Failure     400  {object} httputil.APIError "Invalid request body or ID"
// @Failure     401  {object} httputil.APIError "Unauthorized"
// @Failure     403  {object} httputil.APIError "Cannot modify a global payment method"
// @Failure     404  {object} httputil.APIError "Not found"
// @Failure     409  {object} httputil.APIError "Name already exists"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /payment-methods/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req UpdatePaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	pm, err := h.svc.Update(r.Context(), id, &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, pm)
}

// Delete handles DELETE /payment-methods/{id}
//
// @Summary     Delete a personal payment method
// @Description Removes a personal payment method owned by the authenticated user.
//
//	Fails with 409 if the payment method is referenced by purchase entries.
//
// @Tags        payment-methods
// @Produce     json
// @Param       id  path int true "Payment method ID"
// @Success     204 "No Content"
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     401 {object} httputil.APIError "Unauthorized"
// @Failure     403 {object} httputil.APIError "Cannot delete a global payment method"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     409 {object} httputil.APIError "PaymentMethod in use by purchase entries"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /payment-methods/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
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
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrUnauthorized):
		httputil.WriteUnauthorized(w, "authentication required")
	case errors.Is(err, apperrors.ErrForbidden):
		httputil.WriteForbidden(w, "global payment methods cannot be modified or deleted")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, "payment method not found")
	case errors.Is(err, apperrors.ErrConflict):
		httputil.WriteConflict(w, "payment method name already exists or is in use by purchase entries")
	case errors.Is(err, apperrors.ErrValidation):
		httputil.WriteValidationError(w, err.Error())
	default:
		log := logger.FromContext(r.Context())
		log.Error("internal error in payment method handler", "error", err)
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
