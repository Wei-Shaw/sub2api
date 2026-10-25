-- MySQL 方言版本（翻译自 ../195_add_usage_log_upstream_model_mismatch_index_notx.sql）
--
-- 说明：_notx 文件保留文件名，CONCURRENTLY 去除（MySQL 不支持，迁移执行器保证单次执行）；
--       非唯一部分索引（WHERE upstream_model_mismatch IS TRUE）→ 去掉 WHERE，仅性能差异。
CREATE INDEX idx_usage_logs_upstream_model_mismatch_created_at
    ON usage_logs (created_at DESC, id DESC);
