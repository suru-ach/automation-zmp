package invoices

import (
	"encoding/json"
	"time"
)

type ListFilters struct {
	VendorName    string
	PONumber      string
	InvoiceNumber string
	IsInvoice     *bool
	FromDate      string
	ToDate        string
	Page          int
	Limit         int
}

type InvoiceSummary struct {
	ID            int64      `json:"id"`
	VendorName    *string    `json:"vendor_name,omitempty"`
	InvoiceNumber *string    `json:"invoice_number,omitempty"`
	InvoiceDate   *string    `json:"invoice_date,omitempty"`
	PONumber      *string    `json:"po_number,omitempty"`
	GrandTotal    *float64   `json:"grand_total,omitempty"`
	Currency      *string    `json:"currency,omitempty"`
	IsInvoice     *bool      `json:"is_invoice,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
}

type InvoiceListResponse struct {
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	Limit    int              `json:"limit"`
	Invoices []InvoiceSummary `json:"invoices"`
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
