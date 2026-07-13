// Package repository — support_chat_rag_retriever.go 实现 service.SupportChatRAGRetriever。
//
// 这里直接走 *sql.DB 跑 design D5 的 UNION ALL SQL，因为：
//   - ent 对 pgvector 的 `<=>` 操作符与 `vector` 列读写支持有限；
//   - 检索路径在用户面 chat SSE 热路径上，希望减少 ent reflection 开销；
//   - 一条 UNION ALL 一次 round-trip 即可拉两表 top-K 候选，比 ent two-query 后 merge 简洁。
//
// SQL 形式（与 design.md D5 对齐）：
//
//	WITH embedded AS (SELECT $1::vector AS q)
//	SELECT 'faq' AS source, id, question AS title, answer AS body,
//	       NULL  AS source_url, 1 - (embedding <=> q) AS score
//	  FROM support_faq_items, embedded
//	  WHERE enabled = TRUE AND embedding IS NOT NULL
//	UNION ALL
//	SELECT 'doc' AS source, id, NULL AS title, chunk_text AS body,
//	       source_url, 1 - (embedding <=> q) AS score
//	  FROM support_doc_chunks, embedded
//	  WHERE embedding IS NOT NULL
//	ORDER BY score DESC
//	LIMIT $2;
//
// `<=>` 是 pgvector cosine 距离；similarity = 1 - distance。阈值过滤 / 兜底排序由
// service 层（SupportChatRAGRetrievalService）完成，本层只负责 SQL。
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// supportChatRAGRetriever 是 service.SupportChatRAGRetriever 的 PG/pgvector 实现。
type supportChatRAGRetriever struct {
	sql sqlExecutor
}

// NewSupportChatRAGRetriever 构造检索 repository。
func NewSupportChatRAGRetriever(sqlDB *sql.DB) service.SupportChatRAGRetriever {
	return &supportChatRAGRetriever{sql: sqlDB}
}

// RetrieveTopK 跑 design D5 的 UNION ALL SQL；仅做"喂入向量 + LIMIT"两件事，
// 阈值过滤与排序兜底由 service 层负责。
//
// 入参 qVec 必须长度 == service.SupportChatRAGEmbedDimension；本层不再做长度校验
// （上层 EmbeddingService.Embed 已经在返回前校验维度）。
func (r *supportChatRAGRetriever) RetrieveTopK(
	ctx context.Context,
	qVec []float32,
	limit int,
) ([]service.SupportChatRAGHit, error) {
	if limit <= 0 {
		return []service.SupportChatRAGHit{}, nil
	}
	if len(qVec) == 0 {
		return nil, fmt.Errorf("support chat rag retrieve: query vector empty")
	}
	literal := encodePgVector(qVec)

	const query = `
WITH embedded AS (SELECT $1::vector AS q)
SELECT 'faq'::text AS source, id, question AS title, answer AS body,
       ''::text AS source_url, 1 - (embedding <=> q) AS score
  FROM support_faq_items, embedded
  WHERE enabled = TRUE AND embedding IS NOT NULL
UNION ALL
SELECT 'doc'::text AS source, id, ''::text AS title, chunk_text AS body,
       source_url, 1 - (embedding <=> q) AS score
  FROM support_doc_chunks, embedded
  WHERE embedding IS NOT NULL
ORDER BY score DESC
LIMIT $2`

	rows, err := r.sql.QueryContext(ctx, query, literal, limit)
	if err != nil {
		return nil, fmt.Errorf("support chat rag retrieve query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.SupportChatRAGHit, 0, limit)
	for rows.Next() {
		var h service.SupportChatRAGHit
		if err := rows.Scan(&h.Source, &h.ID, &h.Title, &h.Body, &h.SourceURL, &h.Score); err != nil {
			return nil, fmt.Errorf("support chat rag retrieve scan: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("support chat rag retrieve rows: %w", err)
	}
	return out, nil
}
