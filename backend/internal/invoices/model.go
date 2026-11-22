package invoices

import (
	"encoding/json"
	"time"
)

const (
	ProcessingStatusAwaitingPO    = "awaiting_po"
	ProcessingStatusProcessing    = "processing"
	ProcessingStatusVerified      = "verified"
	ProcessingStatusMismatch      = "mismatch"
	ProcessingStatusNeedsReview   = "needs_review"
	ProcessingStatusError         = "error"
	ProcessingStatusNotApplicable = "not_applicable"
)

func isValidProcessingStatus(status string) bool {
	switch status {
	case ProcessingStatusAwaitingPO, ProcessingStatusProcessing,
		ProcessingStatusVerified, ProcessingStatusMismatch,
		ProcessingStatusNeedsReview, ProcessingStatusError,
		ProcessingStatusNotApplicable:
		return true
	default:
		return false
	}
}

type ListFilters struct {
	VendorName       string
	PONumber         string
	InvoiceNumber    string
	IsInvoice        *bool
	ProcessingStatus string
	FromDate         string
	ToDate           string
	Page             int
	Limit            int
}

type InvoiceSummary struct {
	ID               int64      `json:"id"`
	VendorName       *string    `json:"vendor_name,omitempty"`
	InvoiceNumber    *string    `json:"invoice_number,omitempty"`
	InvoiceDate      *string    `json:"invoice_date,omitempty"`
	PONumber         *string    `json:"po_number,omitempty"`
	GrandTotal       *float64   `json:"grand_total,omitempty"`
	Currency         *string    `json:"currency,omitempty"`
	IsInvoice        *bool      `json:"is_invoice,omitempty"`
	ProcessingStatus string     `json:"processing_status"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
}

type InvoiceListResponse struct {
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	Limit    int              `json:"limit"`
	Invoices []InvoiceSummary `json:"invoices"`
}

type CreateInvoiceRequest struct {
	Email      *CreateEmailInput      `json:"email"`
	Extraction *CreateExtractionInput `json:"extraction"`
	Invoice    *CreateInvoiceInput    `json:"invoice"`
	LineItems  *[]LineItem            `json:"line_items"`
}

type CreateEmailInput struct {
	GmailID   string     `json:"gmail_id"`
	ThreadID  *string    `json:"thread_id,omitempty"`
	Subject   *string    `json:"subject,omitempty"`
	Sender    *string    `json:"sender,omitempty"`
	Recipient *string    `json:"recipient,omitempty"`
	EmailDate *time.Time `json:"email_date,omitempty"`
	Body      *string    `json:"body,omitempty"`
}

type CreateExtractionInput struct {
	Channel       string          `json:"channel"`
	Model         string          `json:"model"`
	CreatedAt     json.RawMessage `json:"created_at,omitempty"`
	Done          *bool           `json:"done,omitempty"`
	DoneReason    *string         `json:"done_reason,omitempty"`
	ResponseCount *int            `json:"response_count,omitempty"`
}

type CreateInvoiceInput struct {
	IsInvoice     *bool    `json:"is_invoice,omitempty"`
	Reason        *string  `json:"reason,omitempty"`
	VendorName    *string  `json:"vendor_name,omitempty"`
	InvoiceNumber *string  `json:"invoice_number,omitempty"`
	InvoiceDate   *string  `json:"invoice_date,omitempty"`
	PONumber      *string  `json:"po_number,omitempty"`
	DueDate       *string  `json:"due_date,omitempty"`
	BillTo        *string  `json:"bill_to,omitempty"`
	Currency      *string  `json:"currency,omitempty"`
	Subtotal      *float64 `json:"subtotal,omitempty"`
	TaxAmount     *float64 `json:"tax_amount,omitempty"`
	GrandTotal    *float64 `json:"grand_total,omitempty"`
	Notes         *string  `json:"notes,omitempty"`
}

type CreateInvoiceResponse struct {
	Success       bool   `json:"success"`
	InvoiceID     int64  `json:"invoice_id"`
	Message       string `json:"message,omitempty"`
	AlreadyExists bool   `json:"already_exists"`
}

type BulkCreateInvoicesRequest struct {
	Invoices []CreateInvoiceRequest `json:"invoices"`
	Body     json.RawMessage        `json:"body,omitempty"`
}

type BulkInvoiceResult struct {
	GmailID   string `json:"gmail_id"`
	Status    string `json:"status"`
	InvoiceID *int64 `json:"invoice_id,omitempty"`
	ErrCode   string `json:"err_code,omitempty"`
	ErrMsg    string `json:"err_msg,omitempty"`
}

type BulkCreateInvoicesResponse struct {
	Total         int                 `json:"total"`
	Created       int                 `json:"created"`
	AlreadyExists int                 `json:"already_exists"`
	Failed        int                 `json:"failed"`
	Results       []BulkInvoiceResult `json:"results"`
}

type InvoiceDetailResponse struct {
	Invoice    InvoiceSummary `json:"invoice"`
	Extraction ExtractionData `json:"extraction"`
	Email      EmailData      `json:"email"`
	LineItems  []LineItem     `json:"line_items"`
}

type ExtractionData struct {
	Channel       string          `json:"channel"`
	Model         *string         `json:"model,omitempty"`
	CreatedAt     json.RawMessage `json:"created_at,omitempty"`
	Done          *bool           `json:"done,omitempty"`
	DoneReason    *string         `json:"done_reason,omitempty"`
	ResponseCount *int            `json:"response_count,omitempty"`
}

type EmailData struct {
	GmailID   string     `json:"gmail_id"`
	ThreadID  *string    `json:"thread_id,omitempty"`
	GmailURL  *string    `json:"gmail_url,omitempty"`
	Subject   *string    `json:"subject,omitempty"`
	Sender    *string    `json:"sender,omitempty"`
	Recipient *string    `json:"recipient,omitempty"`
	EmailDate *time.Time `json:"email_date,omitempty"`
	Body      *string    `json:"body,omitempty"`
}

type LineItem struct {
	Description *string  `json:"description,omitempty"`
	Quantity    *float64 `json:"quantity,omitempty"`
	Unit        *string  `json:"unit,omitempty"`
	UnitPrice   *float64 `json:"unit_price,omitempty"`
	LineTotal   *float64 `json:"line_total,omitempty"`
}
