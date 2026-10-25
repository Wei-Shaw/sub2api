-- MySQL 方言版本（翻译自 ../245_gemini_cached_contents_mutation_lease.sql）
-- 翻译说明：
--   - TIMESTAMPTZ NULL → DATETIME(3) NULL；ADD COLUMN IF NOT EXISTS
--     按迁移执行器单次执行语义去掉修饰词；
--   - 部分索引 WHERE mutation_token IS NOT NULL AND deleted_at IS NULL（非唯一）
--     → 直接去掉 WHERE 谓词（普通索引，仅性能差异）。

-- Gemini 显式缓存修改有效期的进行中操作（修改意图）：mutation_token 标识持有者，
-- mutation_lease_until 之前同一缓存的其他修改被拒绝；expire_time 在提交前保持修改前的值，
-- 租约过期仍未清除的意图由恢复流程把上游有效期回滚到 expire_time。
ALTER TABLE gemini_cached_contents
    ADD COLUMN mutation_token       VARCHAR(64) NULL,
    ADD COLUMN mutation_lease_until DATETIME(3) NULL;

CREATE INDEX idx_gemini_cached_contents_pending_mutation
    ON gemini_cached_contents (mutation_lease_until);
