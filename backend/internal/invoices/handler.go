package invoices

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	repository Repository
	logger     *slog.Logger
}

func NewHandler(repository Repository, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{
		repository: repository,
		logger:     logger,
	}
	return http.HandlerFunc(handler.serveHTTP)
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/invoice.list":
		h.listInvoices(w, r)
	case "/api/v1/invoice.get":
		h.getInvoice(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint not found.")
	}
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}

	filters, err := parseListFilters(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := h.repository.ListInvoices(ctx, filters)
	if err != nil {
		h.logger.Error("list invoices failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not retrieve invoices.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "id must be a positive integer.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := h.repository.GetInvoice(ctx, id)
	if errors.Is(err, ErrInvoiceNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Invoice not found.")
		return
	}
	if err != nil {
		h.logger.Error("get invoice failed", "invoice_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not retrieve invoice.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseListFilters(r *http.Request) (ListFilters, error) {
	query := r.URL.Query()
	page, err := parsePositiveInt(query.Get("page"), 1, 0)
	if err != nil {
		return ListFilters{}, err
	}
	limit, err := parsePositiveInt(query.Get("limit"), 20, 100)
	if err != nil {
		return ListFilters{}, err
	}

	filters := ListFilters{
		VendorName:    strings.TrimSpace(query.Get("vendor_name")),
		PONumber:      strings.TrimSpace(query.Get("po_number")),
		InvoiceNumber: strings.TrimSpace(query.Get("invoice_number")),
		FromDate:      query.Get("from_date"),
		ToDate:        query.Get("to_date"),
		Page:          page,
		Limit:         limit,
	}

	if query.Has("is_invoice") {
		value, err := strconv.ParseBool(query.Get("is_invoice"))
		if err != nil {
			return ListFilters{}, errInvalidBoolean
		}
		filters.IsInvoice = &value
	}
	for _, date := range []string{filters.FromDate, filters.ToDate} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return ListFilters{}, errInvalidDate
			}
		}
	}
	if filters.FromDate != "" && filters.ToDate != "" && filters.FromDate > filters.ToDate {
		return ListFilters{}, errInvalidDateRange
	}

	return filters, nil
}

func parsePositiveInt(value string, fallback, maximum int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || (maximum > 0 && parsed > maximum) {
		return 0, errInvalidPagination
	}
	return parsed, nil
}

type apiError struct {
	Code    string `json:"err_code"`
	Message string `json:"err_msg"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode API response", "error", err)
	}
}

var (
	errInvalidPagination = &queryError{message: "page must be a positive integer and limit must be between 1 and 100."}
	errInvalidBoolean    = &queryError{message: "is_invoice must be true or false."}
	errInvalidDate       = &queryError{message: "from_date and to_date must be valid dates in YYYY-MM-DD format."}
	errInvalidDateRange  = &queryError{message: "from_date must not be later than to_date."}
)

type queryError struct {
	message string
}

func (e *queryError) Error() string { return e.message }
