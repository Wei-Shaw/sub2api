// Package repository — support_doc_chunk_repo.go
//
// 客服知识库 doc chunks 持久化层。表结构由 SQL migration 创建（add-support-knowledge-rag
// 151 号迁移），列定义包含原生 pgvector `embedding vector(1536)`，因此本 repo 完全
// 走 *sql.DB（不经过 ent）：
//
//   - 写入：(source_url, content_hash) 唯一约束 → ON CONFLICT DO NOTHING 保证幂等；
//   - embedding 列：与 FAQ 同款 `'[v1,v2,...]'::vector` 文本字面量绑定；
//   - 清理孤儿：DELETE FROM ... WHERE source_url = $1 AND content_hash <> ALL($2)；
//   - 全表清空（admin purge）：TRUNCATE/DELETE FROM。
//
// 设计参考 design.md D3 / D5：
//   - upsert 不会 UPDATE 已存在的 chunk —— 同样的 (url, hash) 视为"没变"，跳过；
//   - DeleteOrphans 在 pipeline 单一 URL 处理完后调，把"上次抓到但本次没出现"的 chunks
//     物理删除；保留 transaction 边界由 service 层决定（M1 不强制事务以减少长事务）。
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/lib/pq"
)

// supportDocChunkRepository 是 service.SupportDocChunkRepository 的 PG 实现。
type supportDocChunkRepository struct {
	sql sqlExecutor
}

// NewSupportDocChunkRepository 构造 doc chunk repo，仅依赖 *sql.DB。
func NewSupportDocChunkRepository(sqlDB *sql.DB) service.SupportDocChunkRepository {
	return &supportDocChunkRepository{sql: sqlDB}
}

// UpsertChunk 写入一条 chunk；若 (source_url, content_hash) 已存在则视为"没变"跳过。
//
// 返回 inserted=true 表示本次实际新增了一行；false 表示已存在 -> 跳过。
//
// embedding 允许为 nil（embed 失败的兜底路径），写入 NULL；后续 admin 可手动重试。
func (r *supportDocChunkRepository) UpsertChunk(
	ctx context.Context,
	sourceURL, contentHash, chunkText string,
	vec []float32,
) (bool, error) {
	if sourceURL == "" || contentHash == "" {
		return false, fmt.Errorf("support doc chunk upsert: empty source_url or content_hash")
	}

	if len(vec) == 0 {
		// embedding 为空：写 NULL。
		res, err := r.sql.ExecContext(ctx, `
			INSERT INTO support_doc_chunks (source_url, chunk_text, content_hash, embedding, fetched_at)
			VALUES ($1, $2, $3, NULL, NOW())
			ON CONFLICT (source_url, content_hash) DO NOTHING`,
			sourceURL, chunkText, contentHash,
		)
		if err != nil {
			return false, fmt.Errorf("support doc chunk insert (null embed): %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return false, err
		}
		return affected > 0, nil
	}

	if len(vec) != service.SupportChatRAGEmbedDimension {
		return false, fmt.Errorf("support doc chunk: vector dim %d != expected %d",
			len(vec), service.SupportChatRAGEmbedDimension)
	}

	literal := encodePgVector(vec)
	res, err := r.sql.ExecContext(ctx, `
		INSERT INTO support_doc_chunks (source_url, chunk_text, content_hash, embedding, fetched_at)
		VALUES ($1, $2, $3, $4::vector, NOW())
		ON CONFLICT (source_url, content_hash) DO NOTHING`,
		sourceURL, chunkText, contentHash, literal,
	)
	if err != nil {
		return false, fmt.Errorf("support doc chunk insert: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// DeleteOrphans 删除某个 source_url 下不在 keepHashes 集合中的所有 chunks。
//
// 用于 pipeline 处理完一个 URL 后清理"上次抓过但本次不再出现"的 chunks。
// keepHashes 为空切片：删除该 URL 下全部 chunks。
//
// 返回删除条数（用于 status 报告）。
func (r *supportDocChunkRepository) DeleteOrphans(
	ctx context.Context,
	sourceURL string,
	keepHashes []string,
) (int, error) {
	if sourceURL == "" {
		return 0, fmt.Errorf("support doc chunk delete orphans: empty source_url")
	}
	var (
		res sql.Result
		err error
	)
	if len(keepHashes) == 0 {
		res, err = r.sql.ExecContext(ctx,
			`DELETE FROM support_doc_chunks WHERE source_url = $1`,
			sourceURL,
		)
	} else {
		res, err = r.sql.ExecContext(ctx,
			`DELETE FROM support_doc_chunks
			  WHERE source_url = $1
			    AND content_hash <> ALL($2)`,
			sourceURL, pq.Array(keepHashes),
		)
	}
	if err != nil {
		return 0, fmt.Errorf("support doc chunk delete orphans: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// DeleteAll 物理删除全部 doc chunks（admin purge 按钮）。
func (r *supportDocChunkRepository) DeleteAll(ctx context.Context) (int, error) {
	res, err := r.sql.ExecContext(ctx, `DELETE FROM support_doc_chunks`)
	if err != nil {
		return 0, fmt.Errorf("support doc chunk delete all: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// CountAll 全表行数（status 报告 chunks_total 用）。
func (r *supportDocChunkRepository) CountAll(ctx context.Context) (int, error) {
	var n int
	if err := scanSingleRow(ctx, r.sql,
		`SELECT count(*) FROM support_doc_chunks`,
		nil, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// DistinctSourceURLs 返回所有出现过的 source_url（admin UI 可用于"列出已抓站点"）。
func (r *supportDocChunkRepository) DistinctSourceURLs(ctx context.Context) ([]string, error) {
	rows, err := r.sql.QueryContext(ctx,
		`SELECT DISTINCT source_url FROM support_doc_chunks ORDER BY source_url ASC`)
	if err != nil {
		return nil, fmt.Errorf("support doc chunk distinct urls: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]string, 0, 8)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
