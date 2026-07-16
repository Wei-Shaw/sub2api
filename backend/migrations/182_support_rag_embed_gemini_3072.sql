-- 客服知识库 RAG：把 embedding 维度从 OpenAI text-embedding-3-small (1536)
-- 切换到 Gemini gemini-embedding-001 (3072)。
--
-- 语义：
--   1) 清空已有 embedding 列（旧 1536 维向量与新 3072 维不能共存/比较）。
--      业务代码把 embedding IS NULL 视为"未索引"，admin 触发 reindex 或
--      doc pipeline 下一次抓取会重新生成。FAQ / doc chunk 的其它字段保留。
--   2) 把列类型 vector(1536) 改成 vector(3072)。
--   3) 删除旧 ivfflat 近似索引（pgvector ivfflat / hnsw 要求 ≤ 2000 维，
--      3072 维无法建立近似索引）。M1 数据量 < 1 万行，顺序扫描可接受；
--      如后期规模扩大，可考虑改用 halfvec(3072) + hnsw 或做 pgvector >= 0.7 的
--      binary quantization / product quantization。
--   4) 更新表 / 列注释以反映新 embedding 语义。
--
-- 该迁移不可逆（旧 1536 维数据在 (1) 之后即丢失）。

-- 1. 清空旧 embedding（新旧维度不同，无法直接强转）。
UPDATE support_faq_items  SET embedding = NULL WHERE embedding IS NOT NULL;
UPDATE support_doc_chunks SET embedding = NULL WHERE embedding IS NOT NULL;

-- 2. 删除旧的 ivfflat 索引（新维度 3072 超过 ivfflat 2000 维上限）。
DROP INDEX IF EXISTS idx_support_faq_embedding;
DROP INDEX IF EXISTS idx_support_doc_chunks_embedding;

-- 3. 修改列类型到 vector(3072)。
ALTER TABLE support_faq_items  ALTER COLUMN embedding TYPE vector(3072);
ALTER TABLE support_doc_chunks ALTER COLUMN embedding TYPE vector(3072);

-- 4. 更新列注释。
COMMENT ON COLUMN support_faq_items.embedding
    IS 'Gemini gemini-embedding-001 3072 维向量（可切换 provider）；NULL=未索引';
COMMENT ON COLUMN support_doc_chunks.embedding
    IS 'Gemini gemini-embedding-001 3072 维向量（可切换 provider）；NULL=未索引';
