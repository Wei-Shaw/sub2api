-- MySQL 方言版本（翻译自 ../233_add_usage_log_upstream_request_id_index_notx.sql）
-- 翻译说明：
--   - _notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
--     不支持，迁移执行器保证单次执行）。
--   - 非唯一部分索引（WHERE upstream_request_id IS NOT NULL）→ 去掉 WHERE 改为普通索引
--     （仅性能差异，语义等价）。
--   - 索引名保留 idx_usage_logs_upstream_request_id（运行器 prepare 阶段按此名预清理）。

-- 只索引带上游请求标识的行；CONCURRENTLY 避免阻塞热表写入（MySQL 无对应机制）。
CREATE INDEX idx_usage_logs_upstream_request_id
    ON usage_logs (upstream_request_id);
