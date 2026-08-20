-- Migration: 244_channel_monitor_quota_mode
-- 渠道监控配额模式。编号从上游 226 顺延，避免和 fork 已有 226 冲突。
--   1. provider 扩容到全部 8 平台（antigravity/kimi/zhipu/deepseek）
--   2. check_mode：probe / quota / quota_probe
--   3. account_id 关联已有账号
--   4. channel_monitor_histories.quota 配额快照
--   5. 公开设置 channel_monitor_show_quota（默认关闭）

DO $$
DECLARE
    monitor_constraint_def TEXT;
    template_constraint_def TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO monitor_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitors'
       AND c.conname = 'channel_monitors_provider_check';

    IF monitor_constraint_def IS NULL
       OR position('antigravity' IN monitor_constraint_def) = 0
       OR position('kimi' IN monitor_constraint_def) = 0
       OR position('zhipu' IN monitor_constraint_def) = 0
       OR position('deepseek' IN monitor_constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek'));
    END IF;

    SELECT pg_get_constraintdef(c.oid)
      INTO template_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitor_request_templates'
       AND c.conname = 'channel_monitor_request_templates_provider_check';

    IF template_constraint_def IS NULL
       OR position('antigravity' IN template_constraint_def) = 0
       OR position('kimi' IN template_constraint_def) = 0
       OR position('zhipu' IN template_constraint_def) = 0
       OR position('deepseek' IN template_constraint_def) = 0 THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek'));
    END IF;
END $$;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32) NOT NULL DEFAULT 'probe';

-- sub2api-managed-update: reviewed-compatible
DO $$
DECLARE
    check_mode_constraint_def TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO check_mode_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitors'
       AND c.conname = 'channel_monitors_check_mode_check';

    IF check_mode_constraint_def IS NULL
       OR position('''probe''' IN check_mode_constraint_def) = 0
       OR position('''quota''' IN check_mode_constraint_def) = 0
       OR position('''quota_probe''' IN check_mode_constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_check_mode_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_check_mode_check
            CHECK (check_mode IN ('probe', 'quota', 'quota_probe'));
    END IF;
END $$;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_account_id ON channel_monitors(account_id);

COMMENT ON COLUMN channel_monitors.check_mode IS
    'probe = LLM 探活（默认）；quota = 仅查关联账号用量；quota_probe = 探活 + 配额';
COMMENT ON COLUMN channel_monitors.account_id IS
    '配额模式关联的账号 ID（数据源）；账号删除时置空';

ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS quota JSONB;

COMMENT ON COLUMN channel_monitor_histories.quota IS
    '配额模式监控的归一化配额快照（domain.MonitorQuotaSnapshot）；探活模式为 NULL';

INSERT INTO settings (key, value)
VALUES ('channel_monitor_show_quota', 'false')
ON CONFLICT (key) DO NOTHING;
