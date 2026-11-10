-- 撤销 182_prompt_audit_full_prompt.sql：完整提示词原文不得进入 PostgreSQL。
--
-- 规范约束（add-openai-compatible-prompt-audit）：
--   - design.md:39      完整提示词和 Guard 凭据不能进入数据库、日志、API、前端或错误响应
--   - proposal.md:43    完整提示词只允许存在于请求内存和 Redis 短 TTL 载荷
--   - specs/prompt-input-audit/spec.md:81
--                       PostgreSQL、结构化日志、管理 API 和前端 MUST NOT 保存或返回完整原始提示词
--
-- 182 曾把未脱敏的完整提示词写入 prompt_audit_events.full_prompt，并通过管理 API
-- 与前端事件详情回显。本迁移删除该列，事件表只保留 redacted_preview、prompt_hash
-- 等不可逆摘要；复核所需的原文仍只存在于 Redis 短 TTL 载荷中。
--
-- 幂等：重复执行影响 0 列。

ALTER TABLE prompt_audit_events
    DROP COLUMN IF EXISTS full_prompt;
