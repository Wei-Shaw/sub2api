-- 恢复 Ollama Cloud 平台约束。
--
-- 背景：241_add_typesafe_platform.sql 把 user_platform_quotas.platform 与
-- composite_model_routes.target_platform 两个 CHECK 覆写为不含 'ollama_cloud'
-- 的集合，会连带剔除 Ollama Cloud。本迁移在 241 之后重建这两个约束，集合为
-- 241 的全部平台并集 'ollama_cloud'。
--
-- 顺序：必须晚于 241（912 按文件名排序在其后）。新集合是 241 的超集，存量行
-- 瞬时通过校验，因此是默认的 validated 约束。
-- 适用 fresh 建库、空表升级与已应用 241 的环境；channel_monitors /
-- channel_monitor_request_templates 的约束不在本次范围内，保持原状。
--
-- 仅放宽约束，无任何 DML 与数据回填。若 241 尚未应用而库中已存在
-- platform='ollama_cloud' 的行，说明其写入发生在更宽松的旧约束下，需在启动前
-- 停写核对；本迁移不做数据修复，也不能替代该前置处置。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe',
                        'ollama_cloud'));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe',
                               'ollama_cloud'));
