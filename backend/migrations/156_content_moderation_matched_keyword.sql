-- 风控中心：记录关键词拦截命中的具体关键词

ALTER TABLE content_moderation_logs ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(255) NOT NULL DEFAULT '';
