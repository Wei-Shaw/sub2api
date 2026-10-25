-- MySQL 方言版本（翻译自 ../156_content_moderation_matched_keyword.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。

-- 风控中心：记录关键词拦截命中的具体关键词

ALTER TABLE content_moderation_logs ADD COLUMN matched_keyword VARCHAR(255) NOT NULL DEFAULT '';
