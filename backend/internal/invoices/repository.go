package invoices

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type ListRepository interface {
	ListInvoices(context.Context, ListFilters) (InvoiceListResponse, error)
}

type Repository interface {
	ListRepository
	GetInvoice(context.Context, int64) (InvoiceDetailResponse, error)
	CreateInvoice(context.Context, CreateInvoiceRequest) (CreateInvoiceResponse, error)
}

var ErrInvoiceNotFound = errors.New("invoice not found")
var ErrEmailAlreadyExists = errors.New("email already exists without an invoice")

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
		       i.po_number, i.grand_total, i.currency, i.is_invoice,
		       i.processing_status, i.created_at
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
			&invoice.ProcessingStatus,
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
	var processingStatus string
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
		       i.processing_status,
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
		&processingStatus,
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
	result.Invoice.ProcessingStatus = processingStatus
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
	if threadID.Valid && threadID.String != "" {
		gmailURL := gmailThreadURL(threadID.String)
		result.Email.GmailURL = &gmailURL
	}
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

func (r *SQLRepository) CreateInvoice(ctx context.Context, request CreateInvoiceRequest) (CreateInvoiceResponse, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateInvoiceResponse{}, fmt.Errorf("begin invoice transaction: %w", err)
	}
	defer tx.Rollback()

	var emailID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO emails (gmail_id, thread_id, subject, sender, recipient, email_date, body)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (gmail_id) DO NOTHING
		RETURNING id`,
		request.Email.GmailID,
		request.Email.ThreadID,
		request.Email.Subject,
		request.Email.Sender,
		request.Email.Recipient,
		request.Email.EmailDate,
		request.Email.Body,
	).Scan(&emailID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `
			SELECT id
			FROM emails
			WHERE gmail_id = $1
			FOR UPDATE`, request.Email.GmailID).Scan(&emailID)
		if errors.Is(err, sql.ErrNoRows) {
			return CreateInvoiceResponse{}, ErrEmailAlreadyExists
		}
		if err != nil {
			return CreateInvoiceResponse{}, fmt.Errorf("lock existing email: %w", err)
		}
	} else if err != nil {
		return CreateInvoiceResponse{}, fmt.Errorf("insert email: %w", err)
	}

	var existingInvoiceID int64
	invoiceNumber := ""
	if request.Invoice.InvoiceNumber != nil {
		invoiceNumber = strings.TrimSpace(*request.Invoice.InvoiceNumber)
	}
	if invoiceNumber != "" {
		err = tx.QueryRowContext(ctx, `
			SELECT i.id
			FROM invoices i
			JOIN extraction_runs er ON er.id = i.extraction_id
			WHERE er.email_id = $1 AND BTRIM(i.invoice_number) = $2
			ORDER BY i.id
			LIMIT 1`, emailID, invoiceNumber).Scan(&existingInvoiceID)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT i.id
			FROM invoices i
			JOIN extraction_runs er ON er.id = i.extraction_id
			WHERE er.email_id = $1
			ORDER BY i.id
			LIMIT 1`, emailID).Scan(&existingInvoiceID)
	}
	if err == nil {
		if err := tx.Commit(); err != nil {
			return CreateInvoiceResponse{}, fmt.Errorf("commit duplicate invoice lookup: %w", err)
		}
		return CreateInvoiceResponse{
			Success:       true,
			InvoiceID:     existingInvoiceID,
			Message:       "Invoice already exists.",
			AlreadyExists: true,
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CreateInvoiceResponse{}, fmt.Errorf("find existing invoice by email and number: %w", err)
	}

	extraction := request.Extraction
	invoice := request.Invoice
	processingStatus := initialProcessingStatus(invoice)
	var extractionCreatedAt any
	if len(extraction.CreatedAt) > 0 {
		extractionCreatedAt = string(extraction.CreatedAt)
	}
	var extractionID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO extraction_runs (
			email_id, channel, model, created_at, done, done_reason, response_count
		)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		RETURNING id`,
		emailID,
		extraction.Channel,
		extraction.Model,
		extractionCreatedAt,
		extraction.Done,
		extraction.DoneReason,
		extraction.ResponseCount,
	).Scan(&extractionID)
	if err != nil {
		return CreateInvoiceResponse{}, fmt.Errorf("insert extraction run: %w", err)
	}

	var invoiceID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO invoices (
			extraction_id, is_invoice, processing_status, reason, vendor_name, invoice_number,
			invoice_date, po_number, due_date, bill_to, currency, subtotal,
			tax_amount, grand_total, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8, $9::date, $10, $11, $12, $13, $14, $15)
		RETURNING id`,
		extractionID,
		invoice.IsInvoice,
		processingStatus,
		invoice.Reason,
		invoice.VendorName,
		invoice.InvoiceNumber,
		invoice.InvoiceDate,
		invoice.PONumber,
		invoice.DueDate,
		invoice.BillTo,
		invoice.Currency,
		invoice.Subtotal,
		invoice.TaxAmount,
		invoice.GrandTotal,
		invoice.Notes,
	).Scan(&invoiceID)
	if err != nil {
		return CreateInvoiceResponse{}, fmt.Errorf("insert invoice: %w", err)
	}

	for _, item := range *request.LineItems {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoice_line_items (
				invoice_id, description, quantity, unit, unit_price, line_total
			)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			invoiceID,
			item.Description,
			item.Quantity,
			item.Unit,
			item.UnitPrice,
			item.LineTotal,
		); err != nil {
			return CreateInvoiceResponse{}, fmt.Errorf("insert invoice line item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return CreateInvoiceResponse{}, fmt.Errorf("commit invoice transaction: %w", err)
	}
	return CreateInvoiceResponse{
		Success:   true,
		InvoiceID: invoiceID,
		Message:   "Invoice created.",
	}, nil
}

func initialProcessingStatus(invoice *CreateInvoiceInput) string {
	if invoice.IsInvoice != nil && !*invoice.IsInvoice {
		return ProcessingStatusNotApplicable
	}
	if invoice.PONumber == nil || strings.TrimSpace(*invoice.PONumber) == "" {
		return ProcessingStatusNotApplicable
	}
	return ProcessingStatusAwaitingPO
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
	conditions := make([]string, 0, 7)
	args := make([]any, 0, 7)
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
	if filters.ProcessingStatus != "" {
		add("i.processing_status = $%d", filters.ProcessingStatus)
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

func gmailThreadURL(threadID string) string {
	return "https://mail.google.com/mail/u/0/#all/" + url.PathEscape(threadID)
}
