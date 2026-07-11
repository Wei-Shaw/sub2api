-- Add group-bound jshandler script IDs for on_before_request.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS jshandler_script_ids JSONB DEFAULT NULL;
COMMENT ON COLUMN groups.jshandler_script_ids IS
    'Ordered JS handler script library IDs for on_before_request (group-bound)';
