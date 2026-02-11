-- Add skip_monitoring field to error_passthrough_rules table
-- When true, errors matching this rule will not be recorded in ops_error_logs
ALTER TABLE error_passthrough_rules
ADD COLUMN IF NOT EXISTS skip_monitoring BOOLEAN NOT NULL DEFAULT false;
