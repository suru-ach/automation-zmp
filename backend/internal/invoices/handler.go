package invoices

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	case "/api/v1/invoice.create":
		h.createInvoice(w, r)
	case "/api/v1/invoice.bulk-create":
		h.bulkCreateInvoices(w, r)
	case "/api/v1/invoice.list":
		h.listInvoices(w, r)
	case "/api/v1/invoice.get":
		h.getInvoice(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint not found.")
	}
}

func (h *Handler) createInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}

	var request CreateInvoiceRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON.")
		return
	}
	var trailingData any
	if err := decoder.Decode(&trailingData); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must contain a single JSON object.")
		return
	}
	if err := validateCreateInvoiceRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INVOICE", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.repository.CreateInvoice(ctx, request)
	if errors.Is(err, ErrEmailAlreadyExists) {
		writeError(w, http.StatusConflict, "CONFLICT", "Email already exists without an invoice.")
		return
	}
	if err != nil {
		h.logger.Error("create invoice failed", "gmail_id", request.Email.GmailID, "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create invoice.")
		return
	}
	status := http.StatusCreated
	if result.AlreadyExists {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func validateCreateInvoiceRequest(request CreateInvoiceRequest) error {
	if request.Email == nil || request.Extraction == nil || request.Invoice == nil || request.LineItems == nil {
		return errors.New("email, extraction, invoice, and line_items are required.")
	}
	if strings.TrimSpace(request.Email.GmailID) == "" {
		return errors.New("email.gmail_id is required.")
	}
	if request.Extraction.Channel != "pdf" && request.Extraction.Channel != "img" {
		return errors.New("extraction.channel must be pdf or img.")
	}
	if strings.TrimSpace(request.Extraction.Model) == "" || len([]rune(request.Extraction.Model)) > 255 {
		return errors.New("extraction.model is required and must be at most 255 characters.")
	}
	for _, date := range []*string{request.Invoice.InvoiceDate, request.Invoice.DueDate} {
		if date != nil && *date != "" {
			if _, err := time.Parse("2006-01-02", *date); err != nil {
				return errors.New("invoice dates must use YYYY-MM-DD format.")
			}
		}
	}
	if request.Invoice.Currency != nil && len([]rune(*request.Invoice.Currency)) > 10 {
		return errors.New("invoice.currency must be at most 10 characters.")
	}
	if request.Extraction.ResponseCount != nil && *request.Extraction.ResponseCount < 0 {
		return errors.New("extraction.response_count must not be negative.")
	}
	return nil
}

func (h *Handler) bulkCreateInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}

	var request BulkCreateInvoicesRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON.")
		return
	}
	var trailingData any
	if err := decoder.Decode(&trailingData); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must contain a single JSON object.")
		return
	}

	invoices, err := request.invoiceRequests()
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BATCH", err.Error())
		return
	}
	if len(invoices) < 1 || len(invoices) > 100 {
		writeError(w, http.StatusBadRequest, "INVALID_BATCH", "invoices must contain between 1 and 100 records.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result := BulkCreateInvoicesResponse{
		Total:   len(invoices),
		Results: make([]BulkInvoiceResult, 0, len(invoices)),
	}
	for _, invoiceRequest := range invoices {
		itemResult := BulkInvoiceResult{Status: "failed"}
		if invoiceRequest.Email != nil {
			itemResult.GmailID = invoiceRequest.Email.GmailID
		}
		if err := validateCreateInvoiceRequest(invoiceRequest); err != nil {
			itemResult.ErrCode = "INVALID_INVOICE"
			itemResult.ErrMsg = err.Error()
			result.Failed++
			result.Results = append(result.Results, itemResult)
			continue
		}

		created, err := h.repository.CreateInvoice(ctx, invoiceRequest)
		if errors.Is(err, ErrEmailAlreadyExists) {
			itemResult.ErrCode = "CONFLICT"
			itemResult.ErrMsg = "Email already exists without an invoice."
			result.Failed++
		} else if err != nil {
			h.logger.Error("bulk create invoice failed", "gmail_id", itemResult.GmailID, "error", err)
			itemResult.ErrCode = "INTERNAL_ERROR"
			itemResult.ErrMsg = "Could not create invoice."
			result.Failed++
		} else {
			itemResult.InvoiceID = &created.InvoiceID
			if created.AlreadyExists {
				itemResult.Status = "already_exists"
				result.AlreadyExists++
			} else {
				itemResult.Status = "created"
				result.Created++
			}
		}
		result.Results = append(result.Results, itemResult)
	}

	writeJSON(w, http.StatusOK, result)
}

func (request BulkCreateInvoicesRequest) invoiceRequests() ([]CreateInvoiceRequest, error) {
	body := strings.TrimSpace(string(request.Body))
	if body == "" || body == "null" {
		return request.Invoices, nil
	}
	if request.Invoices != nil {
		return nil, errors.New("provide invoices either at the top level or inside body, not both.")
	}

	var wrapped struct {
		Invoices []CreateInvoiceRequest `json:"invoices"`
	}
	if err := json.Unmarshal(request.Body, &wrapped); err == nil && wrapped.Invoices != nil {
		return wrapped.Invoices, nil
	}
	var invoices []CreateInvoiceRequest
	if err := json.Unmarshal(request.Body, &invoices); err == nil {
		return invoices, nil
	}
	return nil, errors.New("body must contain an invoices array or be an array of invoice records.")
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
