-- usage_logs: 调用方提供的请求级元数据（归因标签，如 source/uid/feature）
-- 由客户端通过 X-Usage-Metadata header 传入，用于按业务/用户/功能维度做用量与成本归因。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS metadata jsonb;
