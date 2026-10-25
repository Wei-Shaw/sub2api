-- MySQL 方言版本（翻译自 ../174_add_usage_logs_api_key_latest_ip_index_notx.sql）
-- 翻译说明：
--   - _notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
--     不支持，迁移执行器保证单次执行）。
--   - INCLUDE (ip_address)（PG 覆盖列）→ 删除该子句。
--   - 非唯一部分索引（WHERE ip_address IS NOT NULL AND ip_address <> ''）→ 去掉 WHERE
--     改为普通索引（仅性能差异，语义等价）。
--   - 索引名保留 idx_usage_logs_api_key_latest_ip（运行器 prepare 阶段按此名预清理）。

-- Support the per-key latest non-empty source IP lookup without scanning full key history.
CREATE INDEX idx_usage_logs_api_key_latest_ip
    ON usage_logs (api_key_id, created_at DESC, id DESC);
