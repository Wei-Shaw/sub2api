-- Gemini 显式缓存修改有效期的进行中操作（修改意图）：mutation_token 标识持有者，
-- mutation_lease_until 之前同一缓存的其他修改被拒绝；expire_time 在提交前保持修改前的值，
-- 租约过期仍未清除的意图由恢复流程把上游有效期回滚到 expire_time。
ALTER TABLE gemini_cached_contents
    ADD COLUMN IF NOT EXISTS mutation_token       VARCHAR(64) NULL,
    ADD COLUMN IF NOT EXISTS mutation_lease_until TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_pending_mutation
    ON gemini_cached_contents (mutation_lease_until)
    WHERE mutation_token IS NOT NULL AND deleted_at IS NULL;
