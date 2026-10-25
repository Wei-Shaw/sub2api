-- MySQL 方言版本（翻译自 ../194_add_usage_log_upstream_response_model.sql）

ALTER TABLE usage_logs
    ADD COLUMN upstream_response_model VARCHAR(200) NULL,
    ADD COLUMN upstream_model_mismatch BOOLEAN NULL;
