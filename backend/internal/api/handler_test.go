package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"invoice-tracker/backend/internal/invoices"
	"invoice-tracker/backend/internal/pdfimages"
	"invoice-tracker/backend/internal/reports"
)

type emptyInvoiceRepository struct{}

func (emptyInvoiceRepository) ListInvoices(context.Context, invoices.ListFilters) (invoices.InvoiceListResponse, error) {
	return invoices.InvoiceListResponse{Page: 1, Limit: 20, Invoices: []invoices.InvoiceSummary{}}, nil
}

func (emptyInvoiceRepository) GetInvoice(context.Context, int64) (invoices.InvoiceDetailResponse, error) {
	return invoices.InvoiceDetailResponse{}, invoices.ErrInvoiceNotFound
}

func (emptyInvoiceRepository) CreateInvoice(context.Context, invoices.CreateInvoiceRequest) (invoices.CreateInvoiceResponse, error) {
	return invoices.CreateInvoiceResponse{Success: true, InvoiceID: 73}, nil
}

type unusedPDFConverter struct{}

func (unusedPDFConverter) PageCount(context.Context, string) (int, error) {
	return 0, nil
}

func (unusedPDFConverter) Render(context.Context, string, string, int) error {
	return nil
}

type emptyReportGenerator struct{}

func (emptyReportGenerator) Generate(context.Context, *reports.WorkbookRequest) ([]byte, error) {
	return []byte("test-workbook"), nil
}

func TestRoutesShareConfiguredOrigin(t *testing.T) {
	handler := NewHandler(
		emptyInvoiceRepository{},
		emptyReportGenerator{},
		unusedPDFConverter{},
		"http://localhost:5173",
		slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	)
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/convert", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("expected CORS header on the shared convert route")
	}
}

func TestInvoiceCreateRouteIsRegistered(t *testing.T) {
	handler := NewHandler(
		emptyInvoiceRepository{},
		emptyReportGenerator{},
		unusedPDFConverter{},
		"http://localhost:5173",
		slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.create", strings.NewReader(`{
		"email":{"gmail_id":"route-test"},
		"extraction":{"channel":"pdf","model":"test"},
		"invoice":{},
		"line_items":[]
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
}

func TestInvoiceBulkCreateRouteIsRegistered(t *testing.T) {
	handler := NewHandler(
		emptyInvoiceRepository{},
		emptyReportGenerator{},
		unusedPDFConverter{},
		"http://localhost:5173",
		slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.bulk-create", strings.NewReader(`{
		"body": {
			"invoices": [{
				"email":{"gmail_id":"route-test"},
				"extraction":{"channel":"img","model":"test"},
				"invoice":{"invoice_number":"BULK-ROUTE-TEST"},
				"line_items":[]
			}]
		}
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
}

func TestReportRouteIsRegistered(t *testing.T) {
	handler := NewHandler(
		emptyInvoiceRepository{},
		emptyReportGenerator{},
		unusedPDFConverter{},
		"http://localhost:5173",
		slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/report.gen", strings.NewReader(`{
		"file_name":"purchase-orders.xlsx",
		"sheets":[{
			"name":"POs",
			"header_row":1,
			"columns":[
				{"key":"po_number","label":"PO Number"},
				{"key":"vendor","label":"Vendor"},
				{"key":"po_amount","label":"PO Amount"},
				{"key":"currency","label":"Currency"}
			],
			"rows":[{"source_row":2,"values":{"po_number":"PO-1","vendor":"Vendor","po_amount":"10","currency":"INR"}}]
		}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if response.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content type = %q, want XLSX", response.Header().Get("Content-Type"))
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(data []byte) (int, error) {
	w.t.Helper()
	return len(data), nil
}

var _ pdfimages.Converter = unusedPDFConverter{}
var _ reports.Generator = emptyReportGenerator{}
