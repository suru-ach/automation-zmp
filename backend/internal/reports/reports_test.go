package reports

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"invoice-tracker/backend/internal/invoices"
)

func TestEvaluatePOUsesExactSumAndVendorCurrency(t *testing.T) {
	stored := []storedInvoice{
		{id: 1, vendor: "Northwind", currency: "INR", totalText: sqlString("100.25")},
		{id: 2, vendor: "Northwind", currency: "INR", totalText: sqlString("20")},
		{id: 3, vendor: "Other Vendor", currency: "INR", totalText: sqlString("500")},
	}
	amount := mustAmount(t, "120.25")

	status, ids := evaluatePO("northwind", "inr", amount, stored)
	if status != invoices.ProcessingStatusVerified {
		t.Fatalf("status = %q, want verified", status)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("matched IDs = %v, want [1 2]", ids)
	}

	status, _ = evaluatePO("Northwind", "INR", mustAmount(t, "120.24"), stored)
	if status != invoices.ProcessingStatusMismatch {
		t.Fatalf("status = %q, want mismatch", status)
	}

	status, ids = evaluatePO("Northwind", "USD", amount, stored)
	if status != invoices.ProcessingStatusNeedsReview || len(ids) != 0 {
		t.Fatalf("unmatched currency status/IDs = %q/%v, want needs_review/no IDs", status, ids)
	}
}

func TestWriteWorkbookAppendsOrUpdatesAutomationStatus(t *testing.T) {
	request := WorkbookRequest{
		Sheets: []Sheet{{
			Name:      "POs",
			HeaderRow: 1,
			Columns: []Column{
				{Key: "po_number", Label: "PO Number"},
				{Key: "automation_status", Label: "Old Status"},
				{Key: "po_amount", Label: "PO Amount"},
			},
			Rows: []Row{{
				SourceRow:        2,
				Values:           map[string]string{"po_number": "PO-1", "automation_status": "old", "po_amount": "120"},
				AutomationStatus: invoices.ProcessingStatusVerified,
			}},
		},
			{
				Name:      "POs",
				HeaderRow: 1,
				Columns:   []Column{{Key: "po_number", Label: "PO Number"}},
				Rows: []Row{{
					SourceRow:        2,
					Values:           map[string]string{"po_number": "PO-2"},
					AutomationStatus: invoices.ProcessingStatusNeedsReview,
				}},
			}},
	}

	data, err := writeWorkbook(&request)
	if err != nil {
		t.Fatalf("writeWorkbook() error = %v", err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer workbook.Close()

	sheetNames := workbook.GetSheetList()
	if len(sheetNames) != 2 || sheetNames[0] != "POs" || sheetNames[1] != "POs (2)" {
		t.Fatalf("sheet names = %v, want unique POs sheets", sheetNames)
	}
	rows, err := workbook.GetRows(sheetNames[0])
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	if rows[0][1] != "automation_status" || rows[1][1] != invoices.ProcessingStatusVerified {
		t.Fatalf("automation status header/value = %v/%v", rows[0][1], rows[1][1])
	}
	rows, err = workbook.GetRows(sheetNames[1])
	if err != nil {
		t.Fatalf("GetRows() error = %v", err)
	}
	if rows[0][1] != "automation_status" || rows[1][1] != invoices.ProcessingStatusNeedsReview {
		t.Fatalf("appended automation status header/value = %v/%v", rows[0][1], rows[1][1])
	}
}

func TestReportHandlerValidatesAndReturnsWorkbook(t *testing.T) {
	handler := NewHandler(testGenerator{}, nil)
	body, err := json.Marshal(WorkbookRequest{
		FileName: "purchase-orders.csv",
		Sheets: []Sheet{{
			Name:      "POs",
			HeaderRow: 1,
			Columns: []Column{
				{Key: "po_number", Label: "PO Number"},
				{Key: "vendor", Label: "Vendor"},
				{Key: "po_amount", Label: "PO Amount"},
				{Key: "currency", Label: "Currency"},
			},
			Rows: []Row{{SourceRow: 2, Values: map[string]string{
				"po_number": "PO-1", "vendor": "Vendor", "po_amount": "10", "currency": "INR",
			}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/report.gen", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if response.Header().Get("Content-Disposition") != `attachment; filename="purchase-orders-reconciliation.xlsx"` {
		t.Fatalf("content disposition = %q", response.Header().Get("Content-Disposition"))
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("response is not a valid XLSX: %v", err)
	}
	defer workbook.Close()
	rows, err := workbook.GetRows("POs")
	if err != nil {
		t.Fatalf("read generated sheet: %v", err)
	}
	if rows[0][4] != "automation_status" {
		t.Fatalf("last header = %q, want automation_status", rows[0][4])
	}
}

func TestReportHandlerRejectsMissingMatchColumns(t *testing.T) {
	handler := NewHandler(testGenerator{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/report.gen", strings.NewReader(`{
		"file_name":"po.csv",
		"sheets":[{"name":"POs","header_row":1,"columns":[{"key":"vendor","label":"Vendor"}],
		"rows":[{"source_row":2,"values":{"vendor":"Vendor"}}]}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
	}
}

type testGenerator struct{}

func (testGenerator) Generate(_ context.Context, workbook *WorkbookRequest) ([]byte, error) {
	return writeWorkbook(workbook)
}

func sqlString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func mustAmount(t *testing.T, value string) *big.Rat {
	t.Helper()
	amount, ok := parseAmount(value)
	if !ok {
		t.Fatalf("parseAmount(%q) failed", value)
	}
	return amount
}
