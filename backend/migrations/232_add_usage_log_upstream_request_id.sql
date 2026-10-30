-- usage_logs.upstream_request_id 记录直接上游在响应头中声明的请求标识，
-- 头名由账户 extra.upstream_request_id_header 指定，未指定时按默认识别链取值。
-- NULL 表示历史行、WS 轮次或该路径没有上游请求标识。
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);
