-- MySQL 方言版本（翻译自 ../046b_add_group_supported_model_scopes.sql）

-- 添加分组支持的模型系列字段
-- （JSONB NOT NULL DEFAULT '...'::jsonb → JSON NOT NULL DEFAULT ('...')，8.0.13+ 表达式默认值；
--   `groups` 为 MySQL 8 保留字）
ALTER TABLE `groups`
ADD COLUMN supported_model_scopes JSON NOT NULL
DEFAULT ('["claude", "gemini_text", "gemini_image"]');

-- (comment skipped)
