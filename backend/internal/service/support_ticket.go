// Package service 内的 support_ticket 文件定义工单系统的领域类型、错误以及
// Repository 接口。Repository 实现位于 internal/repository/support_ticket_repo.go。
//
// 设计要点：
//   - 状态机：open → in_progress → closed，closed 为终态（拒绝任何反向迁移）。
//   - 优先级：low | normal | high，用字符串而非整型，保持 schema 简单；
//     列表页排序由 Repository 层用 SQL `CASE` 表达式映射为权重 1/2/3 后 DESC。
//   - chat_context 仅在 GetByID 路径返回；ListByUser / ListAdmin 永远把它置 nil
//     避免列表请求把不必要的大字段拉满（Spec D1.A 中显式约束）。
//   - 用户与作者关系不在 ent 中建立 edge，仅依赖 SQL FK，避免修改 User schema。
//     repository 内通过普通的 user_id / author_id 字段读写。
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// 工单状态枚举。closed 是终态。
const (
	SupportTicketStatusOpen       = "open"
	SupportTicketStatusInProgress = "in_progress"
	SupportTicketStatusClosed     = "closed"
)

// 工单字段长度限制（与数据库列约束 / spec 一致）。
const (
	// SupportTicketTitleMaxLen 与 SQL `VARCHAR(200)` 对齐。
	SupportTicketTitleMaxLen = 200
	// SupportTicketCategoryMaxLenColumn 与 SQL `VARCHAR(50)` 对齐（与 Settings 端 20 字符上限不同：
	// 列限制宽松便于已存数据回放，校验在 service 层统一收敛到 SupportTicketCategoryMaxLen）。
	SupportTicketCategoryMaxLenColumn = 50
	// SupportTicketContentMaxLen 是工单正文上限（Markdown 文本，~16 KB）。
	SupportTicketContentMaxLen = 16384
	// SupportTicketReplyContentMaxLen 是单条回复上限（与正文一致）。
	SupportTicketReplyContentMaxLen = 16384
	// SupportTicketChatContextMaxLen 是 chat_context 字段上限（spec D4：50000 字符）。
	SupportTicketChatContextMaxLen = 50000
)

// 工单系统业务错误。
//
// 注意 NotFound vs 其他选择：
//   - 功能未启用、工单不存在、非 owner 三种情况一律映射为 404，避免泄露资源存在性
//     与权限边界（spec 5.3 的安全要求）。
//   - 状态机违规（已关闭再追加 / closed → open 等）映射为 409 Conflict。
//   - 内容/分类/优先级/状态值非法映射为 400 BadRequest。
var (
	// ErrSupportFeatureDisabled 表示 settings.support_ticket_enabled = false，
	// handler 层应翻成 404 Not Found，与 ErrSupportTicketNotFound 等同处理。
	ErrSupportFeatureDisabled = infraerrors.NotFound(
		"SUPPORT_FEATURE_DISABLED",
		"support ticket feature is disabled",
	)

	// ErrSupportTicketNotFound：工单不存在或当前调用者不是 owner（用户路径）。
	// admin 路径若工单不存在返回相同错误。
	ErrSupportTicketNotFound = infraerrors.NotFound(
		"SUPPORT_TICKET_NOT_FOUND",
		"support ticket not found",
	)

	// ErrSupportTicketClosed：尝试在已关闭工单上追加回复或重复关闭。
	ErrSupportTicketClosed = infraerrors.Conflict(
		"SUPPORT_TICKET_CLOSED",
		"support ticket is closed",
	)

	// ErrSupportTicketInvalidStatusTransition：尝试把状态从 closed 改回 open / in_progress。
	ErrSupportTicketInvalidStatusTransition = infraerrors.Conflict(
		"SUPPORT_TICKET_STATUS_TRANSITION_INVALID",
		"closed ticket cannot be reopened",
	)

	// ErrSupportTicketTitleRequired：title 空或仅空白。
	ErrSupportTicketTitleRequired = infraerrors.BadRequest(
		"SUPPORT_TICKET_TITLE_REQUIRED",
		"support ticket title is required",
	)
	// ErrSupportTicketTitleTooLong：title 超过列上限。
	ErrSupportTicketTitleTooLong = infraerrors.BadRequest(
		"SUPPORT_TICKET_TITLE_TOO_LONG",
		"support ticket title exceeds maximum length",
	)
	// ErrSupportTicketContentRequired：content 空或仅空白。
	ErrSupportTicketContentRequired = infraerrors.BadRequest(
		"SUPPORT_TICKET_CONTENT_REQUIRED",
		"support ticket content is required",
	)
	// ErrSupportTicketContentTooLong：content 超过 16 KB。
	ErrSupportTicketContentTooLong = infraerrors.BadRequest(
		"SUPPORT_TICKET_CONTENT_TOO_LONG",
		"support ticket content exceeds maximum length",
	)
	// ErrSupportTicketCategoryInvalid：category 不在当前 settings 配置中。
	ErrSupportTicketCategoryInvalid = infraerrors.BadRequest(
		"SUPPORT_TICKET_CATEGORY_INVALID",
		"support ticket category is invalid",
	)
	// ErrSupportTicketStatusInvalid：传入的 status 不在 {open, in_progress, closed}。
	ErrSupportTicketStatusInvalid = infraerrors.BadRequest(
		"SUPPORT_TICKET_STATUS_INVALID",
		"support ticket status is invalid",
	)
	// ErrSupportTicketPriorityInvalid：传入的 priority 不在 {low, normal, high}。
	ErrSupportTicketPriorityInvalid = infraerrors.BadRequest(
		"SUPPORT_TICKET_PRIORITY_INVALID",
		"support ticket priority is invalid",
	)
	// ErrSupportTicketChatContextTooLong：chat_context 超过 50000 字符。
	ErrSupportTicketChatContextTooLong = infraerrors.BadRequest(
		"SUPPORT_TICKET_CHAT_CONTEXT_TOO_LONG",
		"support ticket chat_context exceeds maximum length",
	)
	// ErrSupportTicketReplyContentRequired：回复正文空或仅空白。
	ErrSupportTicketReplyContentRequired = infraerrors.BadRequest(
		"SUPPORT_TICKET_REPLY_CONTENT_REQUIRED",
		"support ticket reply content is required",
	)
	// ErrSupportTicketReplyContentTooLong：回复正文超长。
	ErrSupportTicketReplyContentTooLong = infraerrors.BadRequest(
		"SUPPORT_TICKET_REPLY_CONTENT_TOO_LONG",
		"support ticket reply content exceeds maximum length",
	)
	// ErrSupportTicketNoFieldsToUpdate：admin patch 没有任何可改字段。
	ErrSupportTicketNoFieldsToUpdate = infraerrors.BadRequest(
		"SUPPORT_TICKET_NO_FIELDS_TO_UPDATE",
		"no fields to update",
	)
)

// SupportTicket 是工单领域模型。Repository 与 Service 之间统一用此结构。
//
// 字段对应 SQL 列；ChatContext 与 ClosedAt 是可空字段，用 *string / *time.Time 表达。
type SupportTicket struct {
	ID          int64
	UserID      int64
	Title       string
	Content     string
	Category    string
	Status      string
	Priority    string
	ChatContext *string
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SupportTicketReply 是工单回复领域模型。
//
// AuthorID 可空：若 author 用户被删除，FK ON DELETE SET NULL 会把列置 NULL，
// IsAdmin 字段已在写入时记录角色快照，UI 仍能正确显示"客服"标签。
type SupportTicketReply struct {
	ID        int64
	TicketID  int64
	AuthorID  *int64
	IsAdmin   bool
	Content   string
	CreatedAt time.Time
}

// SupportTicketWithReplies 是 GetByID 返回的聚合结构：工单 + 按 created_at 升序的回复列表。
type SupportTicketWithReplies struct {
	Ticket  SupportTicket
	Replies []SupportTicketReply
}

// SupportTicketListFilters 是 admin 列表查询过滤器。
//
// 用 *int64 表达 UserID 是因为 0 在系统里是合法值（虽然实际不会出现），nil 表示"不过滤"。
// 其余 string 字段空串即"不过滤"。Search 是关键词，Repository 层用 ILIKE 同时匹配 title + content。
type SupportTicketListFilters struct {
	UserID   *int64
	Status   string
	Priority string
	Category string
	Search   string
}

// SupportTicketPatch 是 admin 更新工单的可选字段集合。
// 任何字段为 nil 表示"不修改"，非 nil 字段会被 Repository 写入。
//
// ClosedAt 与 Status 必须保持一致：调用方（service）负责在置 status=closed 时同步设置 ClosedAt=now，
// 在置 status=open/in_progress 时同步把 ClosedAt 置 nil（虽然 service 上层会拒绝该转移）。
type SupportTicketPatch struct {
	Status   *string
	Priority *string
	Category *string
	ClosedAt *time.Time
}

// SupportTicketRepository 是工单系统的存储抽象。
//
// 所有方法应遵守的约定：
//   - 入参 *SupportTicket / *SupportTicketReply 在 Create / AppendReply 成功后会被回填 ID/CreatedAt。
//   - List 路径返回的元素 ChatContext 必须为 nil（不暴露大字段；spec D1）。
//   - GetByID 返回的元素必须包含完整 ChatContext。
//   - UpdateFields 在工单不存在时返回 ErrSupportTicketNotFound。
type SupportTicketRepository interface {
	Create(ctx context.Context, t *SupportTicket) error
	GetByID(ctx context.Context, id int64) (*SupportTicket, error)

	// ListByUser 返回某用户的工单分页列表，按 created_at DESC 排序，不含 chat_context。
	ListByUser(
		ctx context.Context,
		userID int64,
		params pagination.PaginationParams,
	) ([]SupportTicket, *pagination.PaginationResult, error)

	// ListAdmin 返回 admin 视角的工单分页列表，强制按 priority(高>中>低) DESC, created_at DESC 排序。
	// 不含 chat_context。
	ListAdmin(
		ctx context.Context,
		filters SupportTicketListFilters,
		params pagination.PaginationParams,
	) ([]SupportTicket, *pagination.PaginationResult, error)

	// UpdateFields 部分更新工单字段（status / priority / category / closed_at）。
	UpdateFields(ctx context.Context, id int64, patch SupportTicketPatch) error

	// AppendReply 追加一条回复，回填 ID 与 CreatedAt。
	AppendReply(ctx context.Context, reply *SupportTicketReply) error

	// ListReplies 按 created_at ASC 返回某工单的全部回复（不分页：单工单回复数量预期 < 数十条）。
	ListReplies(ctx context.Context, ticketID int64) ([]SupportTicketReply, error)
}

// IsValidSupportTicketStatus 判断给定字符串是否为合法工单状态。
func IsValidSupportTicketStatus(s string) bool {
	switch s {
	case SupportTicketStatusOpen, SupportTicketStatusInProgress, SupportTicketStatusClosed:
		return true
	default:
		return false
	}
}

// IsValidSupportTicketPriority 判断给定字符串是否为合法优先级。
//
// 该函数与 ValidateSupportTicketPriority 不同：后者会做大小写 / 空白归一并返回校验后的值，
// 这里只做布尔判断，用于内部 fast path（如已经经过 normalize 后的二次校验）。
func IsValidSupportTicketPriority(p string) bool {
	switch p {
	case SupportTicketPriorityLow, SupportTicketPriorityNormal, SupportTicketPriorityHigh:
		return true
	default:
		return false
	}
}
