package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"invoice-tracker/backend/internal/invoices"
	"invoice-tracker/backend/internal/pdfimages"
)

type emptyInvoiceRepository struct{}

func (emptyInvoiceRepository) ListInvoices(context.Context, invoices.ListFilters) (invoices.InvoiceListResponse, error) {
	return invoices.InvoiceListResponse{Page: 1, Limit: 20, Invoices: []invoices.InvoiceSummary{}}, nil
}

func (emptyInvoiceRepository) GetInvoice(context.Context, int64) (invoices.InvoiceDetailResponse, error) {
	return invoices.InvoiceDetailResponse{}, invoices.ErrInvoiceNotFound
}

type unusedPDFConverter struct{}

func (unusedPDFConverter) PageCount(context.Context, string) (int, error) {
	return 0, nil
}

func (unusedPDFConverter) Render(context.Context, string, string, int) error {
	return nil
}

func TestRoutesShareConfiguredOrigin(t *testing.T) {
	handler := NewHandler(
		emptyInvoiceRepository{},
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

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(data []byte) (int, error) {
	w.t.Helper()
	return len(data), nil
}

var _ pdfimages.Converter = unusedPDFConverter{}
