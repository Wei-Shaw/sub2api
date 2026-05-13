-- After this migration the system no longer stores invoice files on disk.
-- Null out file metadata for all records; the columns are kept to avoid
-- Ent schema churn — they will simply never be written for new requests.
-- A follow-up can drop these columns once the change has been in production
-- long enough for everyone to be confident they are permanently unused.
UPDATE invoice_requests
SET invoice_file_path = NULL,
    invoice_file_name = NULL,
    invoice_file_size = NULL,
    invoice_file_mime = NULL
WHERE invoice_file_path IS NOT NULL
   OR invoice_file_name IS NOT NULL
   OR invoice_file_size IS NOT NULL
   OR invoice_file_mime IS NOT NULL;
