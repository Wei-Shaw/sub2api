// Package repository — support_ticket_repo.go
//
// 工单系统 Repository 实现。设计要点：
//
//   - 列表查询不返回 chat_context（spec D1）：ent 的 ctx.Fields 机制是私有的，没有
//     公开 API 可以"只 SELECT 部分列同时得到完整实体"。这里采取的实用做法是：
//     用普通的 .All() 获取实体，然后在 entity → service 转换层把 ChatContext 强制
//     置 nil。chat_context 列在绝大多数工单上为 NULL（仅浮窗带来的工单非空，
//     且单条上限 50000 字符），List 拉取的 SQL 成本可接受；语义上对调用方完全
//     等价于"列表不暴露 chat_context"。GetByID 单独走 entity → service 的另一条
//     路径，会原样保留 ChatContext。
//
//   - admin 列表的 priority 排序通过自定义 Selector 注入 CASE 表达式：
//     `ORDER BY (CASE priority WHEN 'high' THEN 3 WHEN 'normal' THEN 2 WHEN 'low' THEN 1 END) DESC, created_at DESC`
//     这样让 schema 保持 string + 简单索引，但仍能给 admin 一个"高优先级置顶"
//     的列表视图。
//
//   - admin 关键词 q 走 `title ILIKE %q% OR content ILIKE %q%`（ent 的 ContainsFold
//     生成的就是 ILIKE，参数化拼接，避免 SQL 注入）。q 不裁剪 chat_context 列
//     避免回放浮窗对话内容到搜索结果。
//
//   - Reply 不需要预读 ticket，AppendReply 直接 INSERT；调用方（service）负责
//     先校验 ticket 存在和 owner / status。
package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/supportticket"
	"github.com/Wei-Shaw/sub2api/ent/supportticketreply"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type supportTicketRepository struct {
	client *dbent.Client
}

// NewSupportTicketRepository 构造工单 Repository 实例（接受运行期 *ent.Client，事务由
// clientFromContext 透明切换）。
func NewSupportTicketRepository(client *dbent.Client) service.SupportTicketRepository {
	return &supportTicketRepository{client: client}
}

// Create 新建工单。CreatedAt / UpdatedAt 由 ent schema 默认值填充，回填到入参。
func (r *supportTicketRepository) Create(ctx context.Context, t *service.SupportTicket) error {
	client := clientFromContext(ctx, r.client)
	builder := client.SupportTicket.Create().
		SetUserID(t.UserID).
		SetTitle(t.Title).
		SetContent(t.Content).
		SetCategory(t.Category)

	// status / priority 走 ent default（open / normal）；如果 service 层显式指定就覆盖。
	if t.Status != "" {
		builder.SetStatus(t.Status)
	}
	if t.Priority != "" {
		builder.SetPriority(t.Priority)
	}
	if t.ChatContext != nil {
		builder.SetChatContext(*t.ChatContext)
	}
	if t.ClosedAt != nil {
		builder.SetClosedAt(*t.ClosedAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	// 回填 ID + 时间戳；其余字段调用方已知。
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	t.UpdatedAt = created.UpdatedAt
	t.Status = created.Status
	t.Priority = created.Priority
	return nil
}

// GetByID 返回完整工单（含 chat_context）。未找到返回 ErrSupportTicketNotFound。
func (r *supportTicketRepository) GetByID(ctx context.Context, id int64) (*service.SupportTicket, error) {
	m, err := r.client.SupportTicket.Query().
		Where(supportticket.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSupportTicketNotFound, nil)
	}
	return supportTicketEntityToService(m), nil
}

// ListByUser 返回某用户的工单分页列表（不含 chat_context），按 created_at DESC、tie-break id DESC。
func (r *supportTicketRepository) ListByUser(
	ctx context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
	q := r.client.SupportTicket.Query().
		Where(supportticket.UserIDEQ(userID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Order(dbent.Desc(supportticket.FieldCreatedAt), dbent.Desc(supportticket.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return supportTicketEntitiesToServiceListView(items), paginationResultFromTotal(int64(total), params), nil
}

// ListAdmin 返回 admin 视角分页列表。强制按 priority(高>中>低) DESC、created_at DESC、id DESC 排序。
//
// filters 的所有非空字段都会作为 AND 条件加入 WHERE。Search（q）走 title/content 双字段 ILIKE。
// 列表结果不含 chat_context。
func (r *supportTicketRepository) ListAdmin(
	ctx context.Context,
	filters service.SupportTicketListFilters,
	params pagination.PaginationParams,
) ([]service.SupportTicket, *pagination.PaginationResult, error) {
	q := r.client.SupportTicket.Query()

	if filters.UserID != nil {
		q = q.Where(supportticket.UserIDEQ(*filters.UserID))
	}
	if s := strings.TrimSpace(filters.Status); s != "" {
		q = q.Where(supportticket.StatusEQ(s))
	}
	if p := strings.TrimSpace(filters.Priority); p != "" {
		q = q.Where(supportticket.PriorityEQ(p))
	}
	if c := strings.TrimSpace(filters.Category); c != "" {
		q = q.Where(supportticket.CategoryEQ(c))
	}
	if kw := strings.TrimSpace(filters.Search); kw != "" {
		// ContainsFold 生成 ILIKE %kw%。ent 内部已对 kw 做参数化绑定，无 SQL 注入风险。
		q = q.Where(supportticket.Or(
			supportticket.TitleContainsFold(kw),
			supportticket.ContentContainsFold(kw),
		))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Order(supportTicketAdminOrderOptions()...).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return supportTicketEntitiesToServiceListView(items), paginationResultFromTotal(int64(total), params), nil
}

// supportTicketAdminOrderOptions 返回 admin 列表强制排序：
//
//	ORDER BY (CASE priority WHEN 'high' THEN 3 ... WHEN 'low' THEN 1 ELSE 0 END) DESC,
//	         created_at DESC, id DESC
//
// 用 entsql.Selector.OrderExpr 注入原生 SQL 表达式；CASE 字面量不带用户输入，无注入风险。
// ELSE 0 用于把异常持久值放在最末，避免 NULL/未知值打乱排序。
func supportTicketAdminOrderOptions() []supportticket.OrderOption {
	priorityCase := `(CASE priority ` +
		`WHEN '` + service.SupportTicketPriorityHigh + `' THEN 3 ` +
		`WHEN '` + service.SupportTicketPriorityNormal + `' THEN 2 ` +
		`WHEN '` + service.SupportTicketPriorityLow + `' THEN 1 ` +
		`ELSE 0 END)`

	return []supportticket.OrderOption{
		// 1. priority CASE 权重 DESC
		func(s *entsql.Selector) {
			s.OrderExpr(entsql.Expr(priorityCase + " DESC"))
		},
		// 2. created_at DESC（与 (status, priority, created_at) 索引同向）
		func(s *entsql.Selector) {
			s.OrderBy(entsql.Desc(s.C(supportticket.FieldCreatedAt)))
		},
		// 3. id DESC，稳定排序避免分页跳变
		func(s *entsql.Selector) {
			s.OrderBy(entsql.Desc(s.C(supportticket.FieldID)))
		},
	}
}

// UpdateFields 部分更新工单字段。若工单不存在返回 ErrSupportTicketNotFound。
//
// patch 字段语义：
//   - Status / Priority / Category：非 nil 时覆盖，nil 不动
//   - ClosedAt：非 nil 时 SetClosedAt（即使值是 nil 时间戳也按设置处理；service 层负责
//     传入 valid time）；如需"清除 closed_at"由调用方传 ClosedAt = nil 然后调用方在
//     status 与 ClosedAt 之间维持一致性。本 repo 只透明转发。
func (r *supportTicketRepository) UpdateFields(ctx context.Context, id int64, patch service.SupportTicketPatch) error {
	client := clientFromContext(ctx, r.client)
	builder := client.SupportTicket.UpdateOneID(id)

	hasUpdate := false
	if patch.Status != nil {
		builder.SetStatus(*patch.Status)
		hasUpdate = true
	}
	if patch.Priority != nil {
		builder.SetPriority(*patch.Priority)
		hasUpdate = true
	}
	if patch.Category != nil {
		builder.SetCategory(*patch.Category)
		hasUpdate = true
	}
	if patch.ClosedAt != nil {
		builder.SetClosedAt(*patch.ClosedAt)
		hasUpdate = true
	}

	if !hasUpdate {
		// 仍然 ping 一下确认 ticket 存在；调用方 service 已经做了语义校验，
		// 这里直接返回 nil 也可以——但为兼容"空 patch 表示 touch updated_at"的潜在用法，
		// 这里直接 NOOP 并返回 nil，避免无谓的 UPDATE。
		return nil
	}

	if _, err := builder.Save(ctx); err != nil {
		return translatePersistenceError(err, service.ErrSupportTicketNotFound, nil)
	}
	return nil
}

// AppendReply 追加一条回复。回填 ID 与 CreatedAt。
//
// 调用方应预先校验：
//   - ticket 存在
//   - status != closed
//   - author 与权限边界（user 路径必须是 owner，admin 路径无限制）
//
// 这里不做 cross-row 校验，避免在事务内重复读取，让 service 层在外层事务中编排
// "追加 reply + 把 status open → in_progress" 这种组合操作。
func (r *supportTicketRepository) AppendReply(ctx context.Context, reply *service.SupportTicketReply) error {
	client := clientFromContext(ctx, r.client)
	builder := client.SupportTicketReply.Create().
		SetTicketID(reply.TicketID).
		SetIsAdmin(reply.IsAdmin).
		SetContent(reply.Content)

	if reply.AuthorID != nil {
		builder.SetAuthorID(*reply.AuthorID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	reply.ID = created.ID
	reply.CreatedAt = created.CreatedAt
	return nil
}

// ListReplies 按 created_at ASC 返回某工单的所有回复。
// 不分页：单工单的回复数量预期较小（几条到几十条）。
func (r *supportTicketRepository) ListReplies(ctx context.Context, ticketID int64) ([]service.SupportTicketReply, error) {
	items, err := r.client.SupportTicketReply.Query().
		Where(supportticketreply.TicketIDEQ(ticketID)).
		Order(dbent.Asc(supportticketreply.FieldCreatedAt), dbent.Asc(supportticketreply.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.SupportTicketReply, 0, len(items))
	for _, m := range items {
		if s := supportTicketReplyEntityToService(m); s != nil {
			out = append(out, *s)
		}
	}
	return out, nil
}

// supportTicketEntityToService 把 ent 实体翻译为 service 域模型。**保留 chat_context**。
// 用于 GetByID 路径。
func supportTicketEntityToService(m *dbent.SupportTicket) *service.SupportTicket {
	if m == nil {
		return nil
	}
	return &service.SupportTicket{
		ID:          m.ID,
		UserID:      m.UserID,
		Title:       m.Title,
		Content:     m.Content,
		Category:    m.Category,
		Status:      m.Status,
		Priority:    m.Priority,
		ChatContext: m.ChatContext,
		ClosedAt:    m.ClosedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// supportTicketEntitiesToServiceListView 把 ent 实体列表翻译为 service 列表视图：
// 与 supportTicketEntityToService 一致，但**强制把 ChatContext 置 nil**，确保 List
// 路径永远不会把大字段泄露给上层（spec D1）。
func supportTicketEntitiesToServiceListView(models []*dbent.SupportTicket) []service.SupportTicket {
	out := make([]service.SupportTicket, 0, len(models))
	for _, m := range models {
		s := supportTicketEntityToService(m)
		if s == nil {
			continue
		}
		s.ChatContext = nil // List 视图对外不暴露 chat_context
		out = append(out, *s)
	}
	return out
}

// supportTicketReplyEntityToService 把 reply ent 实体翻译为 service 域模型。
func supportTicketReplyEntityToService(m *dbent.SupportTicketReply) *service.SupportTicketReply {
	if m == nil {
		return nil
	}
	return &service.SupportTicketReply{
		ID:        m.ID,
		TicketID:  m.TicketID,
		AuthorID:  m.AuthorID,
		IsAdmin:   m.IsAdmin,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
}
