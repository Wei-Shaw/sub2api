-- usage_logs.upstream_request_id 记录成功请求的上游响应请求标识
-- （x-request-id / xai-request-id / x-goog-request-id 等响应头，原样落库）。
-- NULL 表示历史行或该路径无上游请求标识（WS 轮次、本地合成计费 ID 等）。
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);
