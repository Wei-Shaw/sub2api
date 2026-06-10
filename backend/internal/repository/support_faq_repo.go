// Package repository — support_faq_repo.go
//
// 客服知识库 FAQ Repository 实现。设计要点（见 add-support-knowledge-rag design D1）：
//
//   - 非 embedding 列（question / answer / tags / enabled / sort_order / created_at /
//     updated_at）由 ent 管理；CRUD 走 ent 标准 builder + clientFromContext，享受事务
//     接管 / hooks 等通用能力。
//
//   - embedding 列（PostgreSQL `vector(1536)`）不在 ent schema 中，由 SQL migration
//     直接 DDL；这里通过原生 *sql.DB 的 SetEmbedding / ClearEmbedding 写入。
//     pgvector 的文本字面量格式 `'[v1,v2,...]'::vector` 直接以参数化字符串提交即可，
//     PG 端会自行转换为 vector。
//
//   - List / GetByID 需要返回 Indexed 字段（embedding IS NOT NULL）：用户面 admin UI
//     依赖该字段渲染"未索引"badge。这里采用"ent 拉非 embedding 列 + raw SQL 拉
//     id 列表（filter embedding IS NOT NULL）"的两步法；表预期 < 1 万行，开销可接受。
//
//   - 与既有 repository 一致的 sqlExecutor 抽象：构造接受 *sql.DB，通过 sqlExecutor
//     接口转发，便于在 transactional 测试中注入 *sql.Tx。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/supportfaqitem"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/lib/pq"
)

// supportFaqRepository 是 service.SupportFaqRepository 的实现。
type supportFaqRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

// NewSupportFaqRepository 构造 FAQ Repository。
//
// 与 NewUsageCleanupRepository 的双注入风格一致：ent.Client 处理结构化字段，
// *sql.DB 处理 embedding 列。
func NewSupportFaqRepository(client *dbent.Client, sqlDB *sql.DB) service.SupportFaqRepository {
	return &supportFaqRepository{client: client, sql: sqlDB}
}

// Create 新建 FAQ。tags = nil 时自动归一化为空切片，避免 ent 写 NULL（SQL schema 是
// `NOT NULL DEFAULT '{}'`，ent default 也兜底，但显式归一更清晰）。
func (r *supportFaqRepository) Create(ctx context.Context, item *service.SupportFaqItem) error {
	if item == nil {
		return fmt.Errorf("nil faq item")
	}
	client := clientFromContext(ctx, r.client)

	tags := item.Tags
	if tags == nil {
		tags = []string{}
	}

	builder := client.SupportFaqItem.Create().
		SetQuestion(item.Question).
		SetAnswer(item.Answer).
		SetTags(tags).
		SetEnabled(item.Enabled).
		SetSortOrder(item.SortOrder)

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	item.ID = created.ID
	item.Tags = append([]string(nil), created.Tags...)
	item.CreatedAt = created.CreatedAt
	item.UpdatedAt = created.UpdatedAt
	// 新建时 embedding 一定为 NULL。
	item.Indexed = false
	return nil
}

// GetByID 读取单条；填充 Indexed 字段。
func (r *supportFaqRepository) GetByID(ctx context.Context, id int64) (*service.SupportFaqItem, error) {
	m, err := r.client.SupportFaqItem.Query().
		Where(supportfaqitem.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportFaqNotFound, nil)
	}

	out := supportFaqEntityToService(m)
	indexed, err := r.isIndexed(ctx, id)
	if err != nil {
		return nil, err
	}
	out.Indexed = indexed
	return out, nil
}

// List 返回全部 FAQ；按 sort_order ASC, id ASC。Indexed 字段批量填充。
func (r *supportFaqRepository) List(ctx context.Context, onlyEnabled bool) ([]service.SupportFaqItem, error) {
	q := r.client.SupportFaqItem.Query().
		Order(dbent.Asc(supportfaqitem.FieldSortOrder), dbent.Asc(supportfaqitem.FieldID))
	if onlyEnabled {
		q = q.Where(supportfaqitem.EnabledEQ(true))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []service.SupportFaqItem{}, nil
	}

	// 一次性拉所有"已索引"的 id 集合，避免 N 次回表。
	indexed, err := r.indexedIDsAll(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.SupportFaqItem, 0, len(rows))
	for _, m := range rows {
		s := supportFaqEntityToService(m)
		if _, ok := indexed[m.ID]; ok {
			s.Indexed = true
		}
		out = append(out, *s)
	}
	return out, nil
}

// UpdatePartial 部分更新；nil 字段不修改。
func (r *supportFaqRepository) UpdatePartial(ctx context.Context, id int64, patch service.SupportFaqItemPatch) error {
	client := clientFromContext(ctx, r.client)
	builder := client.SupportFaqItem.UpdateOneID(id)

	hasUpdate := false
	if patch.Question != nil {
		builder.SetQuestion(*patch.Question)
		hasUpdate = true
	}
	if patch.Answer != nil {
		builder.SetAnswer(*patch.Answer)
		hasUpdate = true
	}
	if patch.Tags != nil {
		tags := *patch.Tags
		if tags == nil {
			tags = []string{}
		}
		builder.SetTags(tags)
		hasUpdate = true
	}
	if patch.Enabled != nil {
		builder.SetEnabled(*patch.Enabled)
		hasUpdate = true
	}
	if patch.SortOrder != nil {
		builder.SetSortOrder(*patch.SortOrder)
		hasUpdate = true
	}

	if !hasUpdate {
		// 兼容空 patch 调用：ping ent 确认 row 存在，不做无谓 UPDATE。
		exists, err := r.client.SupportFaqItem.Query().Where(supportfaqitem.IDEQ(id)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrSupportFaqNotFound
		}
		return nil
	}

	if _, err := builder.Save(ctx); err != nil {
		return translatePersistenceError(err, service.ErrSupportFaqNotFound, nil)
	}
	return nil
}

// Delete 物理删除。row 不存在 → ErrSupportFaqNotFound。
func (r *supportFaqRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	err := client.SupportFaqItem.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSupportFaqNotFound, nil)
	}
	return nil
}

// CountAll 全表行数。
func (r *supportFaqRepository) CountAll(ctx context.Context) (int, error) {
	return r.client.SupportFaqItem.Query().Count(ctx)
}

// SetEmbedding 写入 embedding 列。vec=nil 走 ClearEmbedding 等价路径（NULL）。
func (r *supportFaqRepository) SetEmbedding(ctx context.Context, id int64, vec []float32) error {
	if vec == nil {
		return r.ClearEmbedding(ctx, id)
	}
	if len(vec) != service.SupportChatRAGEmbedDimension {
		return fmt.Errorf("support faq embedding: vector dim %d != expected %d",
			len(vec), service.SupportChatRAGEmbedDimension)
	}
	literal := encodePgVector(vec)
	res, err := r.sql.ExecContext(ctx,
		`UPDATE support_faq_items
		   SET embedding = $1::vector,
		       updated_at = NOW()
		 WHERE id = $2`,
		literal, id,
	)
	if err != nil {
		return fmt.Errorf("support faq set embedding: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrSupportFaqNotFound
	}
	return nil
}

// ClearEmbedding 把 embedding 列置 NULL。
func (r *supportFaqRepository) ClearEmbedding(ctx context.Context, id int64) error {
	res, err := r.sql.ExecContext(ctx,
		`UPDATE support_faq_items
		   SET embedding = NULL,
		       updated_at = NOW()
		 WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("support faq clear embedding: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrSupportFaqNotFound
	}
	return nil
}

// ListIDsWithoutEmbedding 返回 enabled = true 且 embedding IS NULL 的 id 列表。
// 启动数据迁移后异步补 embedding 用；admin 触发 reindex 也用。
func (r *supportFaqRepository) ListIDsWithoutEmbedding(ctx context.Context) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx,
		`SELECT id FROM support_faq_items
		  WHERE enabled = TRUE AND embedding IS NULL
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("support faq list ids without embedding: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// isIndexed 检测单条 row 的 embedding 是否非空。
func (r *supportFaqRepository) isIndexed(ctx context.Context, id int64) (bool, error) {
	var hasEmbed bool
	err := scanSingleRow(ctx, r.sql,
		`SELECT embedding IS NOT NULL FROM support_faq_items WHERE id = $1`,
		[]any{id}, &hasEmbed)
	if err != nil {
		// row 不存在时 ent 路径已经处理了，这里出 sql.ErrNoRows 视作"未索引"。
		// 但更稳健的是冒泡，让上层决定。
		return false, fmt.Errorf("support faq is_indexed: %w", err)
	}
	return hasEmbed, nil
}

// indexedIDsAll 返回所有 embedding IS NOT NULL 的 id 集合。
// List 路径用，避免 N+1。
func (r *supportFaqRepository) indexedIDsAll(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := r.sql.QueryContext(ctx,
		`SELECT id FROM support_faq_items WHERE embedding IS NOT NULL`,
	)
	if err != nil {
		return nil, fmt.Errorf("support faq indexed ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]struct{}, 32)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// supportFaqEntityToService 把 ent 实体转成 service 域模型。Indexed 字段由调用方填。
func supportFaqEntityToService(m *dbent.SupportFaqItem) *service.SupportFaqItem {
	if m == nil {
		return nil
	}
	tags := append([]string(nil), m.Tags...)
	return &service.SupportFaqItem{
		ID:        m.ID,
		Question:  m.Question,
		Answer:    m.Answer,
		Tags:      tags,
		Enabled:   m.Enabled,
		SortOrder: m.SortOrder,
		Indexed:   false,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// encodePgVector 把 []float32 编码成 pgvector 接受的文本字面量 `[1.23,-4.5,...]`。
//
// pgvector 没有 binary protocol（lib/pq 的 array 适配器只支持 PG 原生数组类型，
// vector 是扩展类型），所以走文本字面量。我们用 `%g` 通用格式（最少字符表达浮点），
// 配合 `::vector` cast 即可让 PG 端正确解析。
//
// 出于安全考虑：仅出现 `[`, `]`, `,`, 数字、`-`, `.`, `e/E`、`+`，无注入风险。
func encodePgVector(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(vec) * 12)
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		// %g 在精度足够时输出最短表达；对 float32 默认 6 位有效数字够 cosine 相似度用。
		fmt.Fprintf(&b, "%g", v)
	}
	b.WriteByte(']')
	return b.String()
}

// 以下导入仅为避免 "imported and not used" —— pq 在未来扩展批量 SetEmbedding 时
// 会用到 pq.Array；当前编译期保证 import 集合稳定。
var _ = pq.Array
var _ = time.Now
