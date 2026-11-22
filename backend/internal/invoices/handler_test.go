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
	filters       ListFilters
	result        InvoiceListResponse
	detailResult  InvoiceDetailResponse
	createRequest CreateInvoiceRequest
	createCalls   []CreateInvoiceRequest
	createResult  CreateInvoiceResponse
	createResults []CreateInvoiceResponse
	err           error
}

func (f *fakeListRepository) ListInvoices(_ context.Context, filters ListFilters) (InvoiceListResponse, error) {
	f.filters = filters
	return f.result, f.err
}

func (f *fakeListRepository) GetInvoice(context.Context, int64) (InvoiceDetailResponse, error) {
	return f.detailResult, f.err
}

func (f *fakeListRepository) CreateInvoice(_ context.Context, request CreateInvoiceRequest) (CreateInvoiceResponse, error) {
	f.createRequest = request
	f.createCalls = append(f.createCalls, request)
	if len(f.createResults) >= len(f.createCalls) {
		return f.createResults[len(f.createCalls)-1], f.err
	}
	return f.createResult, f.err
}

func TestListInvoicesPassesFiltersAndReturnsContract(t *testing.T) {
	repository := &fakeListRepository{
		result: InvoiceListResponse{
			Total: 1, Page: 2, Limit: 5,
			Invoices: []InvoiceSummary{{ID: 42, ProcessingStatus: "awaiting_po"}},
		},
	}
	handler := NewHandler(repository, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/invoice.list?vendor_name=Acme&po_number=PO-1&invoice_number=INV-1&is_invoice=true&processing_status=awaiting_po&from_date=2026-01-01&to_date=2026-01-31&page=2&limit=5",
		nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if repository.filters.VendorName != "Acme" || repository.filters.PONumber != "PO-1" ||
		repository.filters.InvoiceNumber != "INV-1" || repository.filters.IsInvoice == nil ||
		!(*repository.filters.IsInvoice) || repository.filters.ProcessingStatus != "awaiting_po" ||
		repository.filters.Page != 2 || repository.filters.Limit != 5 {
		t.Fatalf("unexpected repository filters: %+v", repository.filters)
	}

	var body InvoiceListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Total != 1 || body.Page != 2 || body.Limit != 5 || len(body.Invoices) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
	if body.Invoices[0].ProcessingStatus != "awaiting_po" {
		t.Fatalf("processing_status = %q, want awaiting_po", body.Invoices[0].ProcessingStatus)
	}
}

func TestGetInvoiceReturnsEmailMetadata(t *testing.T) {
	repository := &fakeListRepository{
		detailResult: InvoiceDetailResponse{
			Invoice: InvoiceSummary{ID: 42, ProcessingStatus: "verified"},
			Email: EmailData{
				GmailID: "demo-gmail-001",
				Sender:  stringPointer("billing@example.test"),
				Subject: stringPointer("Sample invoice"),
				GmailURL: stringPointer(
					"https://mail.google.com/mail/u/0/#all/demo-thread-001",
				),
			},
			LineItems: []LineItem{},
		},
	}
	handler := NewHandler(repository, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/invoice.get?id=42", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var result InvoiceDetailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Email.GmailID != "demo-gmail-001" || result.Email.Sender == nil ||
		*result.Email.Sender != "billing@example.test" || result.Email.Subject == nil {
		t.Fatalf("email metadata missing from response: %+v", result.Email)
	}
	if result.Email.GmailURL == nil ||
		*result.Email.GmailURL != "https://mail.google.com/mail/u/0/#all/demo-thread-001" {
		t.Fatalf("Gmail URL missing from response: %+v", result.Email)
	}
	if result.Invoice.ProcessingStatus != "verified" {
		t.Fatalf("processing_status = %q, want verified", result.Invoice.ProcessingStatus)
	}
}

func TestGetInvoiceRejectsInvalidIDAndReturnsNotFound(t *testing.T) {
	handler := NewHandler(&fakeListRepository{err: ErrInvoiceNotFound}, nil)
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/v1/invoice.get?id=0", wantStatus: http.StatusBadRequest},
		{path: "/api/v1/invoice.get?id=missing", wantStatus: http.StatusBadRequest},
		{path: "/api/v1/invoice.get?id=99", wantStatus: http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.wantStatus {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.wantStatus)
		}
	}
}

func TestCreateInvoiceReturnsCreatedResponse(t *testing.T) {
	repository := &fakeListRepository{
		createResult: CreateInvoiceResponse{
			Success:   true,
			InvoiceID: 73,
			Message:   "Invoice created.",
		},
	}
	handler := NewHandler(repository, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.create", strings.NewReader(`{
		"email":{"gmail_id":"new-message-73","subject":"Invoice"},
		"extraction":{"channel":"pdf","model":"test-model"},
		"invoice":{"vendor_name":"Example Vendor","grand_total":118},
		"line_items":[{"description":"Test item","quantity":1,"unit":"each","unit_price":118,"line_total":118}]
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	var result CreateInvoiceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Success || result.InvoiceID != 73 || result.AlreadyExists {
		t.Fatalf("unexpected create response: %+v", result)
	}
	if repository.createRequest.Email == nil ||
		repository.createRequest.Email.GmailID != "new-message-73" ||
		repository.createRequest.Invoice == nil ||
		repository.createRequest.Invoice.VendorName == nil ||
		*repository.createRequest.Invoice.VendorName != "Example Vendor" ||
		repository.createRequest.LineItems == nil ||
		len(*repository.createRequest.LineItems) != 1 {
		t.Fatalf("request was not decoded correctly: %+v", repository.createRequest)
	}
}

func TestCreateInvoiceReturnsOKForDuplicateEmail(t *testing.T) {
	handler := NewHandler(&fakeListRepository{
		createResult: CreateInvoiceResponse{
			Success:       true,
			InvoiceID:     73,
			Message:       "Invoice already exists.",
			AlreadyExists: true,
		},
	}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.create", strings.NewReader(`{
		"email":{"gmail_id":"existing-message"},
		"extraction":{"channel":"pdf","model":"test-model"},
		"invoice":{},
		"line_items":[]
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestCreateInvoiceRejectsMalformedAndInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"email":`},
		{name: "missing fields", body: `{}`},
		{name: "invalid channel", body: `{"email":{"gmail_id":"id"},"extraction":{"channel":"unknown","model":"test"},"invoice":{},"line_items":[]}`},
		{name: "invalid invoice date", body: `{"email":{"gmail_id":"id"},"extraction":{"channel":"pdf","model":"test"},"invoice":{"invoice_date":"yesterday"},"line_items":[]}`},
		{name: "trailing JSON", body: `{"email":{"gmail_id":"id"},"extraction":{"channel":"pdf","model":"test"},"invoice":{},"line_items":[]} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeListRepository{}
			handler := NewHandler(repository, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.create", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if repository.createRequest.Email != nil {
				t.Fatal("repository called for invalid request")
			}
		})
	}
}

func TestBulkCreateInvoicesAcceptsBodyWrapperAndPartialFailures(t *testing.T) {
	repository := &fakeListRepository{
		createResults: []CreateInvoiceResponse{
			{Success: true, InvoiceID: 73},
			{Success: true, InvoiceID: 74},
		},
	}
	handler := NewHandler(repository, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.bulk-create", strings.NewReader(`{
		"body": {
			"invoices": [
				{
					"email":{"gmail_id":"same-email"},
					"extraction":{"channel":"pdf","model":"test"},
					"invoice":{"invoice_number":"INV-1"},
					"line_items":[]
				},
				{
					"email":{"gmail_id":"same-email"},
					"extraction":{"channel":"img","model":"test"},
					"invoice":{"invoice_number":"INV-2"},
					"line_items":[]
				},
				{
					"email":{"gmail_id":"invalid-item"},
					"extraction":{"channel":"unknown","model":"test"},
					"invoice":{},
					"line_items":[]
				}
			]
		}
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	var result BulkCreateInvoicesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Total != 3 || result.Created != 2 || result.AlreadyExists != 0 || result.Failed != 1 {
		t.Fatalf("unexpected bulk counts: %+v", result)
	}
	if len(repository.createCalls) != 2 ||
		repository.createCalls[0].Email.GmailID != "same-email" ||
		*repository.createCalls[0].Invoice.InvoiceNumber != "INV-1" ||
		*repository.createCalls[1].Invoice.InvoiceNumber != "INV-2" {
		t.Fatalf("valid invoices were not individually submitted: %+v", repository.createCalls)
	}
	if result.Results[2].ErrCode != "INVALID_INVOICE" {
		t.Fatalf("invalid item did not report validation error: %+v", result.Results[2])
	}
}

func TestBulkCreateInvoicesRejectsInvalidBatchShape(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty batch", body: `{"invoices":[]}`},
		{name: "more than one source", body: `{"invoices":[],"body":{"invoices":[]}}`},
		{name: "unrecognized body wrapper", body: `{"body":{"records":[]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(&fakeListRepository{}, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/invoice.bulk-create", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
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
		{name: "invalid processing status", path: "/api/v1/invoice.list?processing_status=unknown"},
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

func stringPointer(value string) *string {
	return &value
}

func TestGmailThreadURLPathEscapesThreadID(t *testing.T) {
	got := gmailThreadURL("thread/123")
	if got != "https://mail.google.com/mail/u/0/#all/thread%2F123" {
		t.Fatalf("Gmail URL = %q", got)
	}
}
