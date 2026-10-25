-- MySQL 方言版本（翻译自 ../226_channel_monitor_quota_mode.sql）
-- 翻译说明：
--   - PG DO 块（读 pg_get_constraintdef 判断两个 provider_check 约束定义是否已含
--     'kimi'，缺失才重建）→ 迁移执行器保证单次执行，存在性分支去掉，直接重建两个
--     CHECK 约束（与 176 号翻译同型处理；CHECK 约束需 MySQL 8.0.16+ 才强制执行）。
--   - ADD COLUMN ... REFERENCES（内联外键）→ 显式命名 FK 约束
--     fk_channel_monitors_account_id（MySQL 会忽略列内联 REFERENCES）。
--   - JSONB → JSON；`key` 为 MySQL 保留字需反引号；
--     ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id；
--     COMMENT ON COLUMN 整句跳过。

-- Migration: 226_channel_monitor_quota_mode
-- 渠道监控配额模式：
--   1. provider 扩容到全部 8 平台（antigravity/kimi/zhipu/deepseek）
--      （antigravity 仅支持配额模式，无探活 adapter；国产 3 家复用 OpenAI 兼容探活）
--   2. check_mode：probe（默认，现状探活）/ quota（仅查关联账号用量，零 LLM 成本）
--      / quota_probe（探活 + 配额并存）
--   3. account_id 关联已有账号（配额模式的数据源，复用账号侧用量服务）；
--      账号删除时置空，监控保留并报「账号未关联」
--   4. channel_monitor_histories.quota 持久化归一化配额快照（JSONB）
--   5. 新增公开设置 channel_monitor_show_quota（默认关闭）：
--      控制用户端监控页是否展示配额/余额；管理端始终可见

ALTER TABLE channel_monitors
    DROP CHECK channel_monitors_provider_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kimi', 'zhipu', 'deepseek'));

ALTER TABLE channel_monitor_request_templates
    DROP CHECK channel_monitor_request_templates_provider_check;

ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kimi', 'zhipu', 'deepseek'));

ALTER TABLE channel_monitors
    ADD COLUMN check_mode VARCHAR(32) NOT NULL DEFAULT 'probe';

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_check_mode_check
    CHECK (check_mode IN ('probe', 'quota', 'quota_probe'));

ALTER TABLE channel_monitors
    ADD COLUMN account_id BIGINT NULL,
    ADD CONSTRAINT fk_channel_monitors_account_id FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX idx_channel_monitors_account_id ON channel_monitors(account_id);

-- (comment skipped)
-- (comment skipped)

ALTER TABLE channel_monitor_histories
    ADD COLUMN quota JSON NULL;

-- (comment skipped)

-- 用户端是否展示配额/余额（默认关闭，fail-closed 解析：仅 "true" 视为开启）。
-- 管理端不受此开关影响。
INSERT INTO settings (`key`, value)
VALUES ('channel_monitor_show_quota', 'false')
ON DUPLICATE KEY UPDATE id = id;
