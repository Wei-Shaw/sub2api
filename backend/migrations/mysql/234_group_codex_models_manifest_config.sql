-- MySQL 方言版本（翻译自 ../234_group_codex_models_manifest_config.sql）
-- 翻译说明：
--   - `groups` 是 MySQL 8 保留字（窗口函数 GROUPS），加反引号；
--   - JSONB NOT NULL DEFAULT '{}'::jsonb → JSON NOT NULL DEFAULT ('{}')
--     （8.0.13+ 表达式默认值）；
--   - ADD COLUMN IF NOT EXISTS 按迁移执行器单次执行语义去掉修饰词；
--   - COMMENT ON COLUMN 整句跳过。

ALTER TABLE `groups`
    ADD COLUMN codex_models_manifest_config JSON NOT NULL DEFAULT ('{}');

-- (comment skipped) COMMENT ON COLUMN groups.codex_models_manifest_config IS
--     'Pinned-accounts Codex models manifest config for OpenAI groups: {"enabled":bool,"account_ids":[int64],"fallback_to_scheduler":bool}; when enabled the Codex /models manifest is fetched only from the pinned accounts'
