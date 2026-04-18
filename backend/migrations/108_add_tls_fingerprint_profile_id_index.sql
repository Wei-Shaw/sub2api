-- Migration: 108_add_tls_fingerprint_profile_id_index
-- 为 accounts.extra->>'tls_fingerprint_profile_id' 添加表达式 + 部分索引，
-- 加速 TLS 指纹模板列表 API 中按 profile_id 聚合统计「绑定账号数」的查询。
--
-- 设计说明：
--   - B-tree 表达式索引：物化 (extra->>'tls_fingerprint_profile_id') 为索引键，
--     避免聚合时逐行解析 JSON。GROUP BY 该表达式可走 Index Only Scan。
--   - 部分索引（WHERE 子句）：仅索引绑定了指纹的账号，索引体积最小、
--     查询命中率最高。
--   - 与 045 的 GIN(extra) 索引互不冲突，二者面向不同查询模式。
--
-- 性能预期：1k 账号 <3ms，10k 账号 <10ms，100k 账号 <30ms。
--
-- 兼容性：
--   - 表达式与字段命名沿用现有 service 层约定（"tls_fingerprint_profile_id"）。
--   - 旧账号 extra 中无该字段时不会被索引，写入 / 读取性能零影响。

CREATE INDEX IF NOT EXISTS idx_accounts_tls_fp_profile_id
ON accounts ((extra->>'tls_fingerprint_profile_id'))
WHERE deleted_at IS NULL
  AND extra ? 'tls_fingerprint_profile_id';
