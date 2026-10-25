-- MySQL 方言版本（翻译自 ../079_ops_error_logs_add_endpoint_fields.sql）
-- Ops error logs: add endpoint, model mapping, and request_type fields
-- to match usage_logs observability coverage.
--
-- All columns are nullable with no default to preserve backward compatibility
-- with existing rows.

-- PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

-- 1) Standardized endpoint paths (analogous to usage_logs.inbound_endpoint / upstream_endpoint)
ALTER TABLE ops_error_logs
    ADD COLUMN inbound_endpoint VARCHAR(256),
    ADD COLUMN upstream_endpoint VARCHAR(256);

-- 2) Model mapping fields (analogous to usage_logs.requested_model / upstream_model)
ALTER TABLE ops_error_logs
    ADD COLUMN requested_model VARCHAR(100),
    ADD COLUMN upstream_model VARCHAR(100);

-- 3) Granular request type enum (analogous to usage_logs.request_type: 0=unknown, 1=sync, 2=stream, 3=ws_v2)
ALTER TABLE ops_error_logs
    ADD COLUMN request_type SMALLINT;

-- (comment skipped) COMMENT ON COLUMN ops_error_logs.inbound_endpoint
-- (comment skipped) COMMENT ON COLUMN ops_error_logs.upstream_endpoint
-- (comment skipped) COMMENT ON COLUMN ops_error_logs.requested_model
-- (comment skipped) COMMENT ON COLUMN ops_error_logs.upstream_model
-- (comment skipped) COMMENT ON COLUMN ops_error_logs.request_type
