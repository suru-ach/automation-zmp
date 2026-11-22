package invoices

import "testing"

func TestInitialProcessingStatus(t *testing.T) {
	invoice := true
	notInvoice := false
	poNumber := "PO-123"
	emptyPONumber := "  "

	tests := []struct {
		name    string
		invoice *CreateInvoiceInput
		want    string
	}{
		{
			name:    "invoice with PO waits for verification",
			invoice: &CreateInvoiceInput{IsInvoice: &invoice, PONumber: &poNumber},
			want:    "awaiting_po",
		},
		{
			name:    "invoice without PO is not applicable",
			invoice: &CreateInvoiceInput{IsInvoice: &invoice},
			want:    "not_applicable",
		},
		{
			name:    "non-invoice is not applicable",
			invoice: &CreateInvoiceInput{IsInvoice: &notInvoice, PONumber: &poNumber},
			want:    "not_applicable",
		},
		{
			name:    "blank PO is not applicable",
			invoice: &CreateInvoiceInput{PONumber: &emptyPONumber},
			want:    "not_applicable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialProcessingStatus(test.invoice); got != test.want {
				t.Fatalf("initialProcessingStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildWhereFiltersByProcessingStatus(t *testing.T) {
	where, args := buildWhere(ListFilters{
		VendorName:       "Acme",
		ProcessingStatus: ProcessingStatusVerified,
	})

	if where != " WHERE i.vendor_name ILIKE $1 AND i.processing_status = $2" {
		t.Fatalf("where = %q", where)
	}
	if len(args) != 2 || args[0] != "%Acme%" || args[1] != ProcessingStatusVerified {
		t.Fatalf("args = %#v", args)
	}
}
