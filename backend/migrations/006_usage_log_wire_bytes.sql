-- usage_logs: 线上字节账三列。
-- in_bytes / out_bytes：网关边界，客户端请求体 / 回给客户端响应体的线上字节。
-- upstream_out_bytes：发往上游的请求体字节合计，含失败重试与 failover 的每一次尝试。
-- 三列均为 NOT NULL DEFAULT 0，对旧数据与现有写入路径零影响。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS in_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS out_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_out_bytes BIGINT NOT NULL DEFAULT 0;
