-- 客服知识库 RAG (add-support-knowledge-rag)：
--   1) 启用 pgvector 扩展（在该 PG 实例不可用时整批迁移会回滚，
--      启动层会捕获并降级为「RAG unavailable」状态，详见 design Risks）。
--   2) FAQ 表：admin 维护的结构化 FAQ。
--   3) 文档片段表：从 doc_url 抓取并切片后的段落。
--   embedding 列允许 NULL（计算失败的 row 仍写入，检索时按 IS NOT NULL 过滤）。

-- 1. pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. FAQ 表
CREATE TABLE IF NOT EXISTS support_faq_items (
    id          BIGSERIAL PRIMARY KEY,
    question    VARCHAR(200) NOT NULL,
    answer      TEXT NOT NULL,
    tags        TEXT[]   NOT NULL DEFAULT '{}',
    enabled     BOOLEAN  NOT NULL DEFAULT TRUE,
    sort_order  INTEGER  NOT NULL DEFAULT 0,
    embedding   vector(1536),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_support_faq_sort
    ON support_faq_items (sort_order, id);

-- IVFFlat 索引（cosine ops）：M1 数据量预计 < 1 万行，lists=100 起步
CREATE INDEX IF NOT EXISTS idx_support_faq_embedding
    ON support_faq_items USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- 3. 文档片段表
CREATE TABLE IF NOT EXISTS support_doc_chunks (
    id            BIGSERIAL PRIMARY KEY,
    source_url    VARCHAR(500) NOT NULL,
    chunk_text    TEXT NOT NULL,
    content_hash  CHAR(64) NOT NULL,
    embedding     vector(1536),
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- 同一 URL 上同一份内容只存一份（节省 embedding 成本 + clean-up 用）
    UNIQUE (source_url, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_support_doc_chunks_url
    ON support_doc_chunks (source_url);

CREATE INDEX IF NOT EXISTS idx_support_doc_chunks_embedding
    ON support_doc_chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- 表 / 列注释
COMMENT ON TABLE  support_faq_items                  IS '客服知识库：admin 维护的结构化 FAQ（含 embedding 向量）';
COMMENT ON COLUMN support_faq_items.embedding       IS 'OpenAI text-embedding-3-small 1536 维向量；NULL=未索引';

COMMENT ON TABLE  support_doc_chunks                IS '客服知识库：从 doc_url 抓取并切片后的文档段落';
COMMENT ON COLUMN support_doc_chunks.content_hash   IS 'sha256(chunk_text)；与 source_url 联合做去重';
COMMENT ON COLUMN support_doc_chunks.embedding      IS 'OpenAI text-embedding-3-small 1536 维向量；NULL=未索引';
