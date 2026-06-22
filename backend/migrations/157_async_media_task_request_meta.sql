-- 为 async_media_tasks 增加请求元信息列（fal 等异步图片平台）。
-- 终态 usage_log 可能由后台 reconciler 或 fal 原生轮询（另一个 HTTP 请求）追加写入，
-- 此时已脱离原始请求上下文，故客户端 IP / User-Agent / 端点需在提交时持久化到任务表，
-- 供终态写 usage_log 时回填（端点、IP 等列此前对 fal 行为空）。
-- 全部可空，兼容存量任务。

-- 客户端 IP（支持 IPv6）
ALTER TABLE async_media_tasks ADD COLUMN IF NOT EXISTS client_ip VARCHAR(45);
-- 客户端 User-Agent
ALTER TABLE async_media_tasks ADD COLUMN IF NOT EXISTS user_agent VARCHAR(512);
-- 对外门面端点（客户端可见路径，如 /v1/images/generations、/fal/{model}）
ALTER TABLE async_media_tasks ADD COLUMN IF NOT EXISTS inbound_endpoint VARCHAR(100);
-- 上游 fal 端点（提交所用 slug 路径）
ALTER TABLE async_media_tasks ADD COLUMN IF NOT EXISTS upstream_endpoint VARCHAR(200);
