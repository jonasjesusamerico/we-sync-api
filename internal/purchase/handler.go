package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
	"github.com/jonasjesusamerico/we-sync-api/internal/paymentmethod"
)

// service is the behaviour of *Service the handler depends on. It is declared
// here, at the consumer, so handler tests can substitute a fake.
type service interface {
	Create(ctx context.Context, req *CreatePurchaseRequest) (*Purchase, error)
	GetByID(ctx context.Context, id int64) (*Purchase, error)
	List(ctx context.Context, filter ListFilter) (*PaginatedResult, error)
	Update(ctx context.Context, id int64, req *UpdatePurchaseRequest) (*Purchase, error)
	Delete(ctx context.Context, id int64) error
	Summary(ctx context.Context, year, month int, today time.Time) (*MonthSummary, error)
	Overview(ctx context.Context, year, month int, today time.Time) (*MonthOverview, error)
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
// @Failure     404  {object} httputil.APIError "CATEGORY_NOT_FOUND or PAYMENT_METHOD_NOT_FOUND: category or payment method not found or not accessible"
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
// @Description Returns a paginated and filtered list of purchase entries, newest first, with the count and the
// @Description amount (without CANCELED) of the whole filter.
// @Tags        purchases
// @Produce     json
// @Param       month             query    int    false "Month (1-12)"
// @Param       year              query    int    false "Year (e.g. 2024)"
// @Param       status            query    string false "Status" Enums(PENDING, PAID, CANCELED)
// @Param       category_id       query    int    false "Category ID (exact match)"
// @Param       payment_method_id query    int    false "Payment method ID (exact match)"
// @Param       q                 query    string false "Text searched anywhere in the description, ignoring case and accents (trimmed; at most 255 characters)"
// @Param       page              query    int    false "Page number (default 1)"
// @Param       page_size         query    int    false "Items per page (default 20, max 100)"
// @Success     200               {object} PaginatedResult
// @Failure     400               {object} httputil.APIError "Invalid query parameter"
// @Failure     422               {object} httputil.APIError "page_size above 100 or q above 255 characters"
// @Failure     500               {object} httputil.APIError "Internal server error"
// @Router      /purchases [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// q is not validated here: any text is a valid format, and trimming and the
	// length limit are business rules of the service.
	filter := ListFilter{
		Status: Status(q.Get("status")),
		Search: q.Get("q"),
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

	if v := q.Get("payment_method_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			httputil.WriteBadRequest(w, "payment_method_id must be a positive integer")
			return
		}
		filter.PaymentMethodID = n
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

// Summary handles GET /purchases/summary
//
// @Summary     Month spending summary
// @Description Spent amount of the month (PAID + PENDING, every day of the month) and the comparison with the
// @Description previous month over the same period: up to today's day when the month is today's month, else the
// @Description whole months. today is the client's local date and only decides the period.
// @Description previous_until_day is the last day summed of the previous month (null in FULL_MONTH).
// @Tags        purchases
// @Produce     json
// @Param       year  query    int    true "Year (1-9999)"
// @Param       month query    int    true "Month (1-12)"
// @Param       today query    string true "Client local date (YYYY-MM-DD)"
// @Success     200   {object} MonthSummary
// @Failure     400   {object} httputil.APIError "Missing or invalid year, month or today"
// @Failure     500   {object} httputil.APIError "Internal server error"
// @Router      /purchases/summary [get]
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	year, month, today, ok := parseMonthQuery(w, r)
	if !ok {
		return
	}

	summary, err := h.svc.Summary(r.Context(), year, month, today)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, summary)
}

// Overview handles GET /purchases/overview
//
// @Summary     Month overview of the home page
// @Description KPIs, spending per day, per category and per payment method (PAID + PENDING of the whole month)
// @Description and the 6 latest purchases of any status, all read from one database snapshot. today is the
// @Description client's local date: it decides the current month, the daily average and the forecast days.
// @Tags        purchases
// @Produce     json
// @Param       year  query    int    true "Year (1-9999)"
// @Param       month query    int    true "Month (1-12)"
// @Param       today query    string true "Client local date (YYYY-MM-DD)"
// @Success     200   {object} MonthOverview
// @Failure     400   {object} httputil.APIError "Missing or invalid year, month or today"
// @Failure     401   {object} httputil.APIError "Missing or invalid token"
// @Failure     500   {object} httputil.APIError "Internal server error"
// @Router      /purchases/overview [get]
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	year, month, today, ok := parseMonthQuery(w, r)
	if !ok {
		return
	}

	overview, err := h.svc.Overview(r.Context(), year, month, today)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, overview)
}

// parseMonthQuery reads year, month and today of the summary and the overview, in this order.
// On the first invalid parameter it writes the 400 and returns ok = false.
func parseMonthQuery(w http.ResponseWriter, r *http.Request) (year, month int, today time.Time, ok bool) {
	q := r.URL.Query()

	// A missing parameter is invalid too, with the same message.
	year, err := strconv.Atoi(q.Get("year"))
	if err != nil || year < minSummaryYear || year > maxSummaryYear {
		httputil.WriteBadRequest(w, "year must be an integer between 1 and 9999")
		return 0, 0, time.Time{}, false
	}
	month, err = strconv.Atoi(q.Get("month"))
	if err != nil || month < 1 || month > 12 {
		httputil.WriteBadRequest(w, "month must be an integer between 1 and 12")
		return 0, 0, time.Time{}, false
	}
	today, err = time.Parse(dateLayout, q.Get("today"))
	if err != nil {
		httputil.WriteBadRequest(w, "today must be a date in YYYY-MM-DD format")
		return 0, 0, time.Time{}, false
	}
	return year, month, today, true
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
// @Failure     404  {object} httputil.APIError "NOT_FOUND: purchase not found; CATEGORY_NOT_FOUND or PAYMENT_METHOD_NOT_FOUND: category or payment method not found or not accessible"
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
		httputil.WriteError(w, http.StatusNotFound, httputil.ErrCodeCategoryNotFound, "category not found")
	case errors.Is(err, paymentmethod.ErrPaymentMethodNotFound):
		httputil.WriteError(w, http.StatusNotFound, httputil.ErrCodePaymentMethodNotFound, "payment method not found")
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
