-- MySQL 方言版本（翻译自 ../226_add_usage_log_effective_model_indexes_notx.sql）
-- 翻译说明：
--   - _notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
--     不支持，迁移执行器保证单次执行）。
--   - 表达式键部分：BTRIM → TRIM；函数键部分按规则文档用双括号包裹
--     （外层为键部分定界，内层为表达式）。
--   - 索引名保留 idx_usage_logs_effective_requested_model_created /
--     idx_usage_logs_effective_upstream_model_created（运行器 prepare 阶段按此名预清理）。

CREATE INDEX idx_usage_logs_effective_requested_model_created
    ON usage_logs (
        ((COALESCE(NULLIF(TRIM(requested_model), ''), model))),
        created_at DESC,
        id DESC
    );

CREATE INDEX idx_usage_logs_effective_upstream_model_created
    ON usage_logs (
        ((COALESCE(NULLIF(TRIM(upstream_model), ''), model))),
        created_at DESC,
        id DESC
    );
