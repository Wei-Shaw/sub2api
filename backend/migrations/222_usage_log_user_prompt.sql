-- user_prompt 在 created_at 字段后添加。
-- PostgreSQL 不支持 AFTER 语法，因此物理列会追加到表末尾。
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS user_prompt TEXT;

COMMENT ON COLUMN usage_logs.user_prompt IS
    '本次使用记录中最新一条用户提示词';
