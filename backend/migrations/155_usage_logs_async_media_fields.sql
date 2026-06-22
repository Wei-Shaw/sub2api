-- 为 usage_logs 增加异步媒体任务关联字段（fal 等异步图片平台）。
-- 全部可空，兼容存量普通日志（NULL 表示非异步任务的历史/普通日志）。
-- usage_logs 仍保持只追加语义：每个异步任务在终态追加写一条日志。

-- 关联的 async_media_tasks.id
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS task_id BIGINT;
-- 成功出图的 fal 原始 url 列表
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_urls JSONB;
-- 转存 COS 后的 url 列表
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cos_url JSONB;
-- 计费状态：charged（已扣费）/ refunded（已退费）；NULL 表示普通日志
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS billing_status VARCHAR(16);

CREATE INDEX IF NOT EXISTS usagelog_task_id ON usage_logs (task_id);
