-- Cyber-policy blocks are recorded as request_type=4 so they remain visible in
-- usage audits without being confused with legacy request_type=0 rows.
ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3, 4)) NOT VALID;
