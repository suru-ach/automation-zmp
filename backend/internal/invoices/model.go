package invoices

import "time"

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
