-- This only widens the accepted request-type range. The previous application
-- normalizes the newly introduced value 5 to "unknown", so image rollback stays
-- operational after Live usage rows have been written.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE ONLY usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;

-- NOT VALID avoids scanning the existing usage_logs table while still enforcing
-- the widened constraint for all new rows.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type >= 0 AND request_type <= 5) NOT VALID;
