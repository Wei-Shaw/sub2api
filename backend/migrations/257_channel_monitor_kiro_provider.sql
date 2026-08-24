-- Migration: 257_channel_monitor_kiro_provider (ported from nianzs 229; renumbered for this fork sequence)
-- 把 kiro 加入渠道监控 provider 的 CHECK 约束（channel_monitors 与
-- channel_monitor_request_templates 两张表）。
--
-- 背景：226 号迁移把 provider 扩到 8 平台（新增 antigravity/kimi/zhipu/deepseek）时，
-- 采用 DROP + 重建的写法，重建的列表里没有本 fork 独有的 kiro，与 176 加 grok 时
-- 同型。kiro 缺失导致管理端无法为 kiro 账号建立渠道监控（provider='kiro' 违约）。
--
-- kiro 的支持形态与 antigravity 一致：只允许配额模式（check_mode='quota'）。
-- 探活 adapter 注册表 providerAdapters（internal/service/channel_monitor_checker.go）
-- 未注册 kiro —— kiro 走 AWS CodeWhisperer event-stream 协议，与适配器假定的
-- 「JSON POST + gjson 取文本」形态不兼容。配额侧则可用：
-- getUsageForAccount 对 direct mode 账号（OAuth，或 APIKey 且 base_url 为空）
-- 走 getKiroUsage 取真实额度。
--
-- 幂等守卫的探测键必须是 'kiro'：226 已把 'kimi' 写进约束定义，沿用 kimi 作键
-- 会让本迁移在存量库上直接 no-op。
-- 新约束是旧约束的超集，存量行瞬时校验通过。

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

    IF monitor_constraint_def IS NULL OR position('kiro' IN monitor_constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek'));
    END IF;

    SELECT pg_get_constraintdef(c.oid)
      INTO template_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitor_request_templates'
       AND c.conname = 'channel_monitor_request_templates_provider_check';

    IF template_constraint_def IS NULL OR position('kiro' IN template_constraint_def) = 0 THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek'));
    END IF;
END $$;
