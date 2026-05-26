-- Migration: 115_cleanup_claude_code_mimicry_fields
-- 清理 "Claude Code CLI 模拟套件 (A)" + "Signature Pool (B)" 回滚后遗留的 DB 状态。
--
-- 涉及回滚的功能：
--   - 6d0e0562 feat(fingerprint): Claude Code CLI fingerprint mimicry suite
--   - cfd95669 feat(tls-fingerprint): show binding count + fix randomized fingerprint visibility
--   - 2df77c16/78de54b6/89d14a2 等 Signature Pool 相关 commits
--
-- 需要清理的字段：
--   1. accounts.extra->>'tls_fingerprint_randomized' — cfd95669 引入的随机指纹标记
--   2. accounts.extra->>'metadata' (内含 user_id) — sticky session UUID per Claude OAuth account
--   3. accounts.extra->>'sticky_session_user_id' — sticky session 备用键名（保险）
--
-- 需要清理的索引：
--   - idx_accounts_tls_fp_profile_id — 来自 migration 108，加速绑定数聚合查询。
--     回滚后绑定数 UI 已移除，索引不再被任何查询使用，删除以释放空间。
--
-- 注意：上游已存在的 tls_fingerprint_profile_id / enable_tls_fingerprint 字段保留，
-- 这些是上游 TLS fingerprint profile 功能本身的一部分，不在回滚范围内。

-- 1) 删除 cfd95669 引入的索引
DROP INDEX IF EXISTS idx_accounts_tls_fp_profile_id;

-- 2) 清理 sticky session UUID（仅 Claude/Anthropic OAuth/SetupToken 账号会写入此字段）
UPDATE accounts
SET extra = extra - 'metadata'
WHERE deleted_at IS NULL
  AND extra ? 'metadata';

-- 3) 清理随机指纹标记
UPDATE accounts
SET extra = extra - 'tls_fingerprint_randomized'
WHERE deleted_at IS NULL
  AND extra ? 'tls_fingerprint_randomized';

-- 4) 清理可能残留的 sticky session 备用字段
UPDATE accounts
SET extra = extra - 'sticky_session_user_id'
WHERE deleted_at IS NULL
  AND extra ? 'sticky_session_user_id';
