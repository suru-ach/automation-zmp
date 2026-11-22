CREATE TABLE emails (
    id BIGSERIAL PRIMARY KEY,
    gmail_id TEXT UNIQUE NOT NULL,
    thread_id TEXT,
    subject TEXT,
    sender TEXT,
    recipient TEXT,
    email_date TIMESTAMPTZ,
    body TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);


CREATE TABLE extraction_runs (
    id BIGSERIAL PRIMARY KEY,
    email_id BIGINT NOT NULL,

    channel VARCHAR(10) NOT NULL
        CHECK (channel IN ('pdf', 'img')),

    model VARCHAR(255),
    created_at JSONB,
    done BOOLEAN,
    done_reason TEXT,
    response_count INTEGER,
    processed_at TIMESTAMPTZ DEFAULT NOW(),

    FOREIGN KEY (email_id) REFERENCES emails(id)
);


CREATE TABLE invoices (
    id BIGSERIAL PRIMARY KEY,
    extraction_id BIGINT NOT NULL,

    is_invoice BOOLEAN,
    reason TEXT,
    vendor_name TEXT,
    invoice_number TEXT,
    invoice_date DATE,
    po_number TEXT,
    due_date DATE,
    bill_to TEXT,
    currency VARCHAR(10),
    subtotal NUMERIC(18,2),
    tax_amount NUMERIC(18,2),
    grand_total NUMERIC(18,2),
    notes TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    FOREIGN KEY (extraction_id) REFERENCES extraction_runs(id)
);


CREATE TABLE invoice_line_items (
    id BIGSERIAL PRIMARY KEY,
    invoice_id BIGINT NOT NULL,

    description TEXT,
    quantity NUMERIC(18,4),
    unit TEXT,
    unit_price NUMERIC(18,2),
    line_total NUMERIC(18,2),

    FOREIGN KEY (invoice_id) REFERENCES invoices(id)
);
