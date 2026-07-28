-- Post-rollout finalizer for ingress-rejection log cleanup.
--
-- DO NOT run this during a rolling deployment. Run it only after:
--   1. every application instance is on the release that no longer reads or
--      writes deleted_api_key_audits and the deprecated ops_error_logs columns;
--   2. cleanup-ingress-reject-logs has been dry-run and, if desired, executed;
--   3. a database backup or recovery point has been verified.

BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

DROP TABLE IF EXISTS deleted_api_key_audits;

ALTER TABLE IF EXISTS ops_error_logs
    DROP COLUMN IF EXISTS attempted_key_prefix,
    DROP COLUMN IF EXISTS deleted_key_owner_user_id,
    DROP COLUMN IF EXISTS deleted_key_name;

COMMIT;

-- Run this separately during a normal maintenance window:
-- VACUUM (ANALYZE) ops_error_logs;
