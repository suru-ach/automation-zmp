package invoices

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ListRepository interface {
	ListInvoices(context.Context, ListFilters) (InvoiceListResponse, error)
}

type Repository interface {
	ListRepository
	GetInvoice(context.Context, int64) (InvoiceDetailResponse, error)
}

var ErrInvoiceNotFound = errors.New("invoice not found")

type SQLRepository struct {
	db *sql.DB
}

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) ListInvoices(ctx context.Context, filters ListFilters) (InvoiceListResponse, error) {
	where, args := buildWhere(filters)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invoices i"+where, args...).Scan(&total); err != nil {
		return InvoiceListResponse{}, fmt.Errorf("count invoices: %w", err)
	}

	queryArgs := append([]any(nil), args...)
	queryArgs = append(queryArgs, filters.Limit, (filters.Page-1)*filters.Limit)
	query := `
		SELECT i.id, i.vendor_name, i.invoice_number, i.invoice_date,
		       i.po_number, i.grand_total, i.currency, i.is_invoice, i.created_at
		FROM invoices i` + where + `
		ORDER BY i.created_at DESC NULLS LAST, i.id DESC
		LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return InvoiceListResponse{}, fmt.Errorf("query invoices: %w", err)
	}
	defer rows.Close()

	result := InvoiceListResponse{
		Total:    total,
		Page:     filters.Page,
		Limit:    filters.Limit,
		Invoices: make([]InvoiceSummary, 0),
	}
	for rows.Next() {
		var invoice InvoiceSummary
		var vendorName, invoiceNumber, poNumber, currency sql.NullString
		var invoiceDate sql.NullTime
		var grandTotal sql.NullFloat64
		var isInvoice sql.NullBool
		var createdAt sql.NullTime

		if err := rows.Scan(
			&invoice.ID,
			&vendorName,
			&invoiceNumber,
			&invoiceDate,
			&poNumber,
			&grandTotal,
			&currency,
			&isInvoice,
			&createdAt,
		); err != nil {
			return InvoiceListResponse{}, fmt.Errorf("scan invoice: %w", err)
		}

		invoice.VendorName = nullableString(vendorName)
		invoice.InvoiceNumber = nullableString(invoiceNumber)
		if invoiceDate.Valid {
			date := invoiceDate.Time.Format("2006-01-02")
			invoice.InvoiceDate = &date
		}
		invoice.PONumber = nullableString(poNumber)
		invoice.Currency = nullableString(currency)
		if grandTotal.Valid {
			invoice.GrandTotal = &grandTotal.Float64
		}
		if isInvoice.Valid {
			invoice.IsInvoice = &isInvoice.Bool
		}
		if createdAt.Valid {
			invoice.CreatedAt = &createdAt.Time
		}
		result.Invoices = append(result.Invoices, invoice)
	}
	if err := rows.Err(); err != nil {
		return InvoiceListResponse{}, fmt.Errorf("iterate invoices: %w", err)
	}

	return result, nil
}

func (r *SQLRepository) GetInvoice(ctx context.Context, id int64) (InvoiceDetailResponse, error) {
	var result InvoiceDetailResponse
	var vendorName, invoiceNumber, poNumber, currency sql.NullString
	var invoiceDate, createdAt sql.NullTime
	var grandTotal sql.NullFloat64
	var isInvoice sql.NullBool
	var model, extractionCreatedAt, doneReason sql.NullString
	var done sql.NullBool
	var responseCount sql.NullInt64
	var threadID, subject, sender, recipient, body sql.NullString
	var emailDate sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT i.id, i.vendor_name, i.invoice_number, i.invoice_date,
		       i.po_number, i.grand_total, i.currency, i.is_invoice, i.created_at,
		       er.channel, er.model, er.created_at, er.done, er.done_reason, er.response_count,
		       e.gmail_id, e.thread_id, e.subject, e.sender, e.recipient, e.email_date, e.body
		FROM invoices i
		JOIN extraction_runs er ON er.id = i.extraction_id
		JOIN emails e ON e.id = er.email_id
		WHERE i.id = $1`, id,
	).Scan(
		&result.Invoice.ID,
		&vendorName,
		&invoiceNumber,
		&invoiceDate,
		&poNumber,
		&grandTotal,
		&currency,
		&isInvoice,
		&createdAt,
		&result.Extraction.Channel,
		&model,
		&extractionCreatedAt,
		&done,
		&doneReason,
		&responseCount,
		&result.Email.GmailID,
		&threadID,
		&subject,
		&sender,
		&recipient,
		&emailDate,
		&body,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InvoiceDetailResponse{}, ErrInvoiceNotFound
	}
	if err != nil {
		return InvoiceDetailResponse{}, fmt.Errorf("query invoice details: %w", err)
	}

	result.Invoice.VendorName = nullableString(vendorName)
	result.Invoice.InvoiceNumber = nullableString(invoiceNumber)
	if invoiceDate.Valid {
		date := invoiceDate.Time.Format("2006-01-02")
		result.Invoice.InvoiceDate = &date
	}
	result.Invoice.PONumber = nullableString(poNumber)
	result.Invoice.Currency = nullableString(currency)
	if grandTotal.Valid {
		result.Invoice.GrandTotal = &grandTotal.Float64
	}
	if isInvoice.Valid {
		result.Invoice.IsInvoice = &isInvoice.Bool
	}
	if createdAt.Valid {
		result.Invoice.CreatedAt = &createdAt.Time
	}
	result.Extraction.Model = nullableString(model)
	if extractionCreatedAt.Valid {
		result.Extraction.CreatedAt = json.RawMessage(extractionCreatedAt.String)
	}
	if done.Valid {
		result.Extraction.Done = &done.Bool
	}
	result.Extraction.DoneReason = nullableString(doneReason)
	if responseCount.Valid {
		count := int(responseCount.Int64)
		result.Extraction.ResponseCount = &count
	}
	result.Email.ThreadID = nullableString(threadID)
	result.Email.Subject = nullableString(subject)
	result.Email.Sender = nullableString(sender)
	result.Email.Recipient = nullableString(recipient)
	if emailDate.Valid {
		result.Email.EmailDate = &emailDate.Time
	}
	result.Email.Body = nullableString(body)

	result.LineItems, err = r.listLineItems(ctx, id)
	if err != nil {
		return InvoiceDetailResponse{}, err
	}
	return result, nil
}

func (r *SQLRepository) listLineItems(ctx context.Context, invoiceID int64) ([]LineItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT description, quantity, unit, unit_price, line_total
		FROM invoice_line_items
		WHERE invoice_id = $1
		ORDER BY id`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("query invoice line items: %w", err)
	}
	defer rows.Close()

	items := make([]LineItem, 0)
	for rows.Next() {
		var item LineItem
		var description, unit sql.NullString
		var quantity, unitPrice, lineTotal sql.NullFloat64
		if err := rows.Scan(&description, &quantity, &unit, &unitPrice, &lineTotal); err != nil {
			return nil, fmt.Errorf("scan invoice line item: %w", err)
		}
		item.Description = nullableString(description)
		item.Unit = nullableString(unit)
		if quantity.Valid {
			item.Quantity = &quantity.Float64
		}
		if unitPrice.Valid {
			item.UnitPrice = &unitPrice.Float64
		}
		if lineTotal.Valid {
			item.LineTotal = &lineTotal.Float64
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice line items: %w", err)
	}
	return items, nil
}

func buildWhere(filters ListFilters) (string, []any) {
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 6)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}

	if filters.VendorName != "" {
		add("i.vendor_name ILIKE $%d", "%"+filters.VendorName+"%")
	}
	if filters.PONumber != "" {
		add("i.po_number ILIKE $%d", "%"+filters.PONumber+"%")
	}
	if filters.InvoiceNumber != "" {
		add("i.invoice_number ILIKE $%d", "%"+filters.InvoiceNumber+"%")
	}
	if filters.IsInvoice != nil {
		add("i.is_invoice = $%d", *filters.IsInvoice)
	}
	if filters.FromDate != "" {
		add("i.invoice_date >= $%d::date", filters.FromDate)
	}
	if filters.ToDate != "" {
		add("i.invoice_date <= $%d::date", filters.ToDate)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
