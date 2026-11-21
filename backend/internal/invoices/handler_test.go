package invoices

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeListRepository struct {
	filters ListFilters
	result  InvoiceListResponse
	err     error
}

func (f *fakeListRepository) ListInvoices(_ context.Context, filters ListFilters) (InvoiceListResponse, error) {
	f.filters = filters
	return f.result, f.err
}

func TestListInvoicesPassesFiltersAndReturnsContract(t *testing.T) {
	repository := &fakeListRepository{
		result: InvoiceListResponse{
			Total: 1, Page: 2, Limit: 5,
			Invoices: []InvoiceSummary{{ID: 42}},
		},
	}
	handler := NewHandler(repository, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/invoice.list?vendor_name=Acme&po_number=PO-1&invoice_number=INV-1&is_invoice=true&from_date=2026-01-01&to_date=2026-01-31&page=2&limit=5",
		nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if repository.filters.VendorName != "Acme" || repository.filters.PONumber != "PO-1" ||
		repository.filters.InvoiceNumber != "INV-1" || repository.filters.IsInvoice == nil ||
		!(*repository.filters.IsInvoice) || repository.filters.Page != 2 || repository.filters.Limit != 5 {
		t.Fatalf("unexpected repository filters: %+v", repository.filters)
	}

	var body InvoiceListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 1 || body.Page != 2 || body.Limit != 5 || len(body.Invoices) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestListInvoicesRejectsInvalidQuery(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "invalid page", path: "/api/v1/invoice.list?page=0"},
		{name: "limit too high", path: "/api/v1/invoice.list?limit=101"},
		{name: "invalid boolean", path: "/api/v1/invoice.list?is_invoice=yes"},
		{name: "invalid date", path: "/api/v1/invoice.list?from_date=not-a-date"},
		{name: "reversed dates", path: "/api/v1/invoice.list?from_date=2026-02-01&to_date=2026-01-01"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(&fakeListRepository{}, nil)
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestListInvoicesReturnsInternalError(t *testing.T) {
	handler := NewHandler(&fakeListRepository{err: errors.New("database unavailable")}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/invoice.list", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
