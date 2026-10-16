package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// service is the behaviour of *Service the handler depends on. It is declared
// here, at the consumer, so handler tests can substitute a fake.
type service interface {
	Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, id int64) (*Purchase, error)
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, id int64) error
}

// Handler handles HTTP requests for purchase entries.
type Handler struct {
	svc service
}

// NewHandler creates a new Handler with the given service.
func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /purchases
//
// @Summary     Create a purchase entry
// @Description Creates a new monthly purchase entry.
// @Tags        purchases
// @Accept      json
// @Produce     json
// @Param       body body     CreatePurchaseRequest true "Purchase entry data"
// @Success     201  {object} Purchase
// @Failure     400  {object} httputil.APIError "Invalid request body"
// @Failure     404  {object} httputil.APIError "Category not found"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /purchases [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreatePurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	purchase, err := h.svc.Create(r.Context(), &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, purchase)
}

// List handles GET /purchases
//
// @Summary     List purchase entries
// @Description Returns a paginated and filtered list of purchase entries.
// @Tags        purchases
// @Produce     json
// @Param       month          query    int    false "Month (1-12)"
// @Param       year           query    int    false "Year (e.g. 2024)"
// @Param       status         query    string false "Status" Enums(PENDING, PAID, CANCELED)
// @Param       category_id    query    int    false "Category ID (exact match)"
// @Param       payment_method query    string false "Payment method" Enums(CASH, CREDIT_CARD, DEBIT_CARD, PIX, BANK_TRANSFER, BOLETO)
// @Param       page           query    int    false "Page number (default 1)"
// @Param       page_size      query    int    false "Items per page (default 20, max 100)"
// @Success     200            {object} PaginatedResult
// @Failure     400            {object} httputil.APIError "Invalid query parameter"
// @Failure     500            {object} httputil.APIError "Internal server error"
// @Router      /purchases [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := ListFilter{
		Status:        Status(q.Get("status")),
		PaymentMethod: PaymentMethod(q.Get("payment_method")),
	}

	if v := q.Get("month"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 12 {
			httputil.WriteBadRequest(w, "month must be an integer between 1 and 12")
			return
		}
		filter.Month = n
	}

	if v := q.Get("year"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httputil.WriteBadRequest(w, "year must be a positive integer")
			return
		}
		filter.Year = n
	}

	if v := q.Get("category_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			httputil.WriteBadRequest(w, "category_id must be a positive integer")
			return
		}
		filter.CategoryID = n
	}

	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httputil.WriteBadRequest(w, "page must be a positive integer")
			return
		}
		filter.Page = n
	}

	if v := q.Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httputil.WriteBadRequest(w, "page_size must be a positive integer")
			return
		}
		filter.PageSize = n
	}

	result, err := h.svc.List(r.Context(), filter)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// GetByID handles GET /purchases/{id}
//
// @Summary     Get a purchase entry by ID
// @Description Returns a single purchase entry identified by its ID.
// @Tags        purchases
// @Produce     json
// @Param       id  path     int true "Purchase entry ID"
// @Success     200 {object} Purchase
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /purchases/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	purchase, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, purchase)
}

// Update handles PUT /purchases/{id}
//
// @Summary     Update a purchase entry
// @Description Replaces all fields of an existing purchase entry.
// @Tags        purchases
// @Accept      json
// @Produce     json
// @Param       id   path     int                  true "Purchase entry ID"
// @Param       body body     UpdatePurchaseRequest true "Updated purchase data"
// @Success     200  {object} Purchase
// @Failure     400  {object} httputil.APIError "Invalid request body or ID"
// @Failure     404  {object} httputil.APIError "Purchase or category not found"
// @Failure     422  {object} httputil.APIError "Validation error"
// @Failure     500  {object} httputil.APIError "Internal server error"
// @Router      /purchases/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req UpdatePurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteBadRequest(w, "invalid request body")
		return
	}

	purchase, err := h.svc.Update(r.Context(), id, &req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, purchase)
}

// Delete handles DELETE /purchases/{id}
//
// @Summary     Delete a purchase entry
// @Description Removes a purchase entry by its ID.
// @Tags        purchases
// @Produce     json
// @Param       id  path int true "Purchase entry ID"
// @Success     204 "No Content"
// @Failure     400 {object} httputil.APIError "Invalid ID"
// @Failure     404 {object} httputil.APIError "Not found"
// @Failure     500 {object} httputil.APIError "Internal server error"
// @Router      /purchases/{id} [delete]
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
	case errors.Is(err, category.ErrCategoryNotFound):
		httputil.WriteNotFound(w, "category not found")
	case errors.Is(err, apperrors.ErrNotFound):
		httputil.WriteNotFound(w, "purchase not found")
	case errors.Is(err, apperrors.ErrValidation):
		httputil.WriteValidationError(w, err.Error())
	default:
		log := logger.FromContext(r.Context())
		log.Error("internal error in purchase handler", "error", err)
		httputil.WriteInternalError(w)
	}
}

// parseID extracts and validates the {id} URL parameter from the request.
// Writes an error response and returns false if the parameter is invalid.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httputil.WriteBadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}
