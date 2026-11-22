package reports

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"invoice-tracker/backend/internal/invoices"
)

type SQLGenerator struct {
	db *sql.DB
}

func NewSQLGenerator(db *sql.DB) *SQLGenerator {
	return &SQLGenerator{db: db}
}

type storedInvoice struct {
	id        int64
	vendor    string
	currency  string
	totalText sql.NullString
}

func (g *SQLGenerator) Generate(ctx context.Context, workbook *WorkbookRequest) ([]byte, error) {
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin report transaction: %w", err)
	}
	defer tx.Rollback()

	poCounts := countPurchaseOrders(workbook)
	for sheetIndex := range workbook.Sheets {
		for rowIndex := range workbook.Sheets[sheetIndex].Rows {
			row := &workbook.Sheets[sheetIndex].Rows[rowIndex]
			row.AutomationStatus = invoices.ProcessingStatusNeedsReview
			poNumber := strings.TrimSpace(row.Values["po_number"])
			if poNumber == "" {
				continue
			}
			stored, err := queryInvoicesForPO(ctx, tx, poNumber)
			if err != nil {
				return nil, err
			}
			if len(stored) == 0 {
				continue
			}
			if poCounts[normalizePO(poNumber)] > 1 {
				if err := updateInvoiceStatuses(ctx, tx, storedInvoiceIDs(stored), row.AutomationStatus); err != nil {
					return nil, err
				}
				continue
			}
			vendor := strings.TrimSpace(row.Values["vendor"])
			currency := strings.TrimSpace(row.Values["currency"])
			poAmount, ok := parseAmount(row.Values["po_amount"])
			if vendor == "" || currency == "" || !ok {
				if err := updateInvoiceStatuses(ctx, tx, storedInvoiceIDs(stored), row.AutomationStatus); err != nil {
					return nil, err
				}
				continue
			}

			status, matchedIDs := evaluatePO(vendor, currency, poAmount, stored)
			row.AutomationStatus = status
			if status == invoices.ProcessingStatusNeedsReview {
				if err := updateInvoiceStatuses(ctx, tx, storedInvoiceIDs(stored), status); err != nil {
					return nil, err
				}
				continue
			}
			if err := updateInvoiceStatuses(ctx, tx, matchedIDs, status); err != nil {
				return nil, err
			}
		}
	}

	data, err := writeWorkbook(workbook)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit report transaction: %w", err)
	}
	return data, nil
}

func countPurchaseOrders(workbook *WorkbookRequest) map[string]int {
	counts := make(map[string]int)
	for _, sheet := range workbook.Sheets {
		for _, row := range sheet.Rows {
			if poNumber := strings.TrimSpace(row.Values["po_number"]); poNumber != "" {
				counts[normalizePO(poNumber)]++
			}
		}
	}
	return counts
}

func normalizePO(poNumber string) string {
	return strings.ToUpper(strings.TrimSpace(poNumber))
}

func queryInvoicesForPO(ctx context.Context, tx *sql.Tx, poNumber string) ([]storedInvoice, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT i.id, COALESCE(i.vendor_name, ''), COALESCE(i.currency, ''), i.grand_total::text
		FROM invoices i
		WHERE i.is_invoice IS TRUE
		  AND LOWER(BTRIM(COALESCE(i.po_number, ''))) = LOWER($1)
		ORDER BY i.id
		FOR UPDATE
	`, poNumber)
	if err != nil {
		return nil, fmt.Errorf("find invoices for PO %q: %w", poNumber, err)
	}
	defer rows.Close()

	var result []storedInvoice
	for rows.Next() {
		var invoice storedInvoice
		if err := rows.Scan(&invoice.id, &invoice.vendor, &invoice.currency, &invoice.totalText); err != nil {
			return nil, fmt.Errorf("read invoice for PO %q: %w", poNumber, err)
		}
		result = append(result, invoice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoices for PO %q: %w", poNumber, err)
	}
	return result, nil
}

func storedInvoiceIDs(stored []storedInvoice) []int64 {
	ids := make([]int64, 0, len(stored))
	for _, invoice := range stored {
		ids = append(ids, invoice.id)
	}
	return ids
}

func updateInvoiceStatuses(ctx context.Context, tx *sql.Tx, ids []int64, status string) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			"UPDATE invoices SET processing_status = $1 WHERE id = $2",
			status, id,
		); err != nil {
			return fmt.Errorf("update invoice %d processing status: %w", id, err)
		}
	}
	return nil
}

func evaluatePO(vendor, currency string, poAmount *big.Rat, stored []storedInvoice) (string, []int64) {
	sum := new(big.Rat)
	matchedIDs := make([]int64, 0, len(stored))
	for _, invoice := range stored {
		if !strings.EqualFold(strings.TrimSpace(invoice.vendor), vendor) ||
			!strings.EqualFold(strings.TrimSpace(invoice.currency), currency) {
			continue
		}
		if !invoice.totalText.Valid {
			return invoices.ProcessingStatusNeedsReview, nil
		}
		total, ok := parseAmount(invoice.totalText.String)
		if !ok {
			return invoices.ProcessingStatusNeedsReview, nil
		}
		sum.Add(sum, total)
		matchedIDs = append(matchedIDs, invoice.id)
	}
	if len(matchedIDs) == 0 {
		return invoices.ProcessingStatusNeedsReview, nil
	}
	if sum.Cmp(poAmount) == 0 {
		return invoices.ProcessingStatusVerified, matchedIDs
	}
	return invoices.ProcessingStatusMismatch, matchedIDs
}

var amountPattern = regexp.MustCompile(`^-?(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$`)

func parseAmount(input string) (*big.Rat, bool) {
	value := strings.TrimSpace(input)
	if !amountPattern.MatchString(value) {
		return nil, false
	}
	value = strings.ReplaceAll(value, ",", "")
	amount, ok := new(big.Rat).SetString(value)
	return amount, ok
}
