package invoices

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ListRepository interface {
	ListInvoices(context.Context, ListFilters) (InvoiceListResponse, error)
}

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
