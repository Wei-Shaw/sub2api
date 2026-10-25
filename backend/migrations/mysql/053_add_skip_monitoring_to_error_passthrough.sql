-- MySQL 方言版本（翻译自 ../053_add_skip_monitoring_to_error_passthrough.sql）
-- Add skip_monitoring field to error_passthrough_rules table
-- When true, errors matching this rule will not be recorded in ops_error_logs
ALTER TABLE error_passthrough_rules
ADD COLUMN skip_monitoring BOOLEAN NOT NULL DEFAULT FALSE;
