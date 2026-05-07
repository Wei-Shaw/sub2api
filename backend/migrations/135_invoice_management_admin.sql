-- Extend invoice_requests with admin processing fields.
ALTER TABLE invoice_requests
    ADD COLUMN IF NOT EXISTS invoice_no VARCHAR(64),
    ADD COLUMN IF NOT EXISTS invoice_file_path VARCHAR(512),
    ADD COLUMN IF NOT EXISTS invoice_file_size BIGINT,
    ADD COLUMN IF NOT EXISTS invoice_file_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS invoice_file_mime VARCHAR(128),
    ADD COLUMN IF NOT EXISTS processed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS invoice_requests_processed_by_idx ON invoice_requests(processed_by);
