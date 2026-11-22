ALTER TABLE invoices
ADD COLUMN processing_status VARCHAR(32) NOT NULL DEFAULT 'awaiting_po'
CHECK (processing_status IN (
    'awaiting_po',
    'processing',
    'verified',
    'mismatch',
    'needs_review',
    'error',
    'not_applicable'
));
