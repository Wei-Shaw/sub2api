// Package service 内的 support_ticket_notification 文件定义工单站内通知与
// 已读游标（read cursor）的领域类型 + Repository 接口。
//
// 两条并行的"已读"轨道，各司其职（见 openspec/changes/ticket-notifications/specs）：
//
//  1. 【per-工单读游标】support_ticket_reads.last_read_at
//     - 用户或管理员 GET 工单详情 / admin 回复自己工单时 upsert；
//     - 用于"未读工单数"聚合（侧栏红点、`GET /unread-count` 端点）；
//     - 用户视角未读工单 = 存在 admin 回复且 created_at > last_read_at；
//     - 管理员视角未读工单 = 工单创建 or 用户回复晚于 last_read_at；
//     - 没有显式"标记已读"端点，读游标由详情端点副作用维护。
//
//  2. 【per-通知已读位】support_ticket_notification.is_read / read_at
//     - 铃铛面板通知列表数据源，每个 recipient 每次事件一行；
//     - 显式 mark-one-read / mark-all-read 端点驱动；
//     - 用于铃铛红点数字（未读通知数 = COUNT(is_read=false)）。
//
// 两者独立，铃铛红点数字和"未读工单数"是不同语义的聚合，不共享 SQL。
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SupportTicketNotification 是铃铛面板一条通知记录的领域模型，与
// support_ticket_notification 表一一对应。
//
// 字段语义：
//   - RecipientUserID：谁收到这条通知（管理员或工单 owner）；
//   - TicketID：跳转目标；
//   - EventType：ticket_created | user_replied | admin_replied（见 domain 常量）；
//   - ActorUserID：触发者（*int64 因为 FK ON DELETE SET NULL，用户注销后置 NULL）；
//   - TitleSnapshot：写入时的工单标题快照，工单标题后续被改动时铃铛列表仍显示原值；
//   - Excerpt：正文摘要（<= 500 rune，写入前必须用 domain.TruncateSupportTicketNotificationExcerpt 截断）。
//     Optional：ticket_created 事件通常带工单正文摘要；如果调用方不填则为空串。
//   - IsRead / ReadAt：per-通知已读位；用户显式标记时更新。
type SupportTicketNotification struct {
	ID              int64
	RecipientUserID int64
	TicketID        int64
	EventType       string
	TitleSnapshot   string
	Excerpt         string
	ActorUserID     *int64
	IsRead          bool
	CreatedAt       time.Time
	ReadAt          *time.Time
}

// SupportTicketReadState 是"某用户对某工单的已读游标"领域模型，与
// support_ticket_reads 表一一对应。
//
// 写入是 upsert 语义：(ticket_id, user_id) 冲突时 UPDATE last_read_at + updated_at。
// 缺失一行视为"从未读过"（等价于 last_read_at = '1970-01-01'）。
type SupportTicketReadState struct {
	TicketID   int64
	UserID     int64
	LastReadAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SupportTicketNotificationListParams 是铃铛面板通知列表查询参数。
//
// - RecipientUserID：必填，只返回该收件人自己的通知（handler 层从 auth ctx 注入）；
// - OnlyUnread：true 时仅返回 is_read=false 的记录；
// - Params：分页信息（默认 PageSize=20，与 announcement 保持一致）。
type SupportTicketNotificationListParams struct {
	RecipientUserID int64
	OnlyUnread      bool
	Params          pagination.PaginationParams
}

// SupportTicketNotificationRepository 是铃铛面板通知记录的存储抽象。
//
// 契约：
//   - Insert 成功后回填 ID / CreatedAt / IsRead(false) / ReadAt(nil)。
//   - ListByRecipient 排序固定 created_at DESC；OnlyUnread=true 在 SQL 层过滤。
//   - MarkOneRead 只在 (id, recipient_user_id) 匹配时更新 is_read=true / read_at=now；
//     未匹配返回 ErrSupportTicketNotificationNotFound（handler 翻 404，防泄漏 recipient 存在性）。
//   - MarkAllRead 把该 recipient 的所有 is_read=false 通知置为 true / read_at=now，
//     返回受影响行数（用于 audit + 减少不必要的写）。
//   - CountUnreadByRecipient 直接查 COUNT(recipient, is_read=false)，走复合索引。
type SupportTicketNotificationRepository interface {
	// Insert 写入一条通知。excerpt / title_snapshot 需调用方在传入前做 rune 级截断。
	Insert(ctx context.Context, n *SupportTicketNotification) error

	// ListByRecipient 返回分页后的通知列表。
	ListByRecipient(
		ctx context.Context,
		params SupportTicketNotificationListParams,
	) ([]SupportTicketNotification, *pagination.PaginationResult, error)

	// CountUnreadByRecipient 返回 recipient 视角的未读通知条数（is_read=false）。
	CountUnreadByRecipient(ctx context.Context, recipientUserID int64) (int64, error)

	// MarkOneRead 把 (id, recipientUserID) 的 is_read 置 true、read_at 置 readAt。
	// 已读记录重复调用是幂等 no-op；(id, recipient) 不匹配返回 ErrSupportTicketNotificationNotFound。
	MarkOneRead(ctx context.Context, id int64, recipientUserID int64, readAt time.Time) error

	// MarkAllRead 把 recipient 名下所有 is_read=false 的通知一次性置为已读，
	// 返回受影响行数。幂等（无未读时返回 0）。
	MarkAllRead(ctx context.Context, recipientUserID int64, readAt time.Time) (int64, error)
}

// SupportTicketReadRepository 是"per-工单读游标"存储抽象。
//
// 与通知表并行独立：这里维护的是 support_ticket_reads.last_read_at，
// 用于未读工单数聚合与详情自动清红点。
//
// 契约：
//   - MarkTicketRead 是 upsert 语义（(ticket_id, user_id) 冲突时更新 last_read_at），
//     必须幂等；调用方通常传 time.Now()。
//   - CountUnreadForUser 返回该用户 owner 的工单中"有 admin 回复且晚于 last_read_at"的工单数量；
//     缺 read 行视为 last_read_at = '1970-01-01'（即从未读过）。
//   - CountUnreadForAdmin 返回该 admin 视角下"工单创建 or 存在用户回复晚于 last_read_at"的工单数量。
type SupportTicketReadRepository interface {
	// MarkTicketRead 把 (ticketID, userID) 的 last_read_at 置为 readAt（upsert）。
	// 该操作是详情端点的副作用；失败时端点不应返回错误，但仍应把错误上抛供 handler 记 warn。
	MarkTicketRead(ctx context.Context, ticketID, userID int64, readAt time.Time) error

	// CountUnreadForUser 返回该用户 owner 的工单中未读工单条数。
	// 未读定义（用户视角）：存在 admin reply.created_at > coalesce(last_read_at, epoch)。
	CountUnreadForUser(ctx context.Context, userID int64) (int64, error)

	// CountUnreadForAdmin 返回该 admin 视角下未读工单条数。
	// 未读定义（admin 视角）：工单 created_at > coalesce(last_read_at, epoch)
	//                    OR 存在 user reply.created_at > coalesce(last_read_at, epoch)。
	CountUnreadForAdmin(ctx context.Context, adminID int64) (int64, error)
}

// ErrSupportTicketNotificationNotFound 表示 mark-one-read 时通知不存在或
// 不属于当前调用者。
//
// 有意与"权限不足"共用同一错误：spec 5.3 要求 mark-read 对非 recipient 返回 404
// （不是 403）以避免泄漏"该通知是否存在"。
var ErrSupportTicketNotificationNotFound = infraerrors.NotFound(
	"SUPPORT_TICKET_NOTIFICATION_NOT_FOUND",
	"support ticket notification not found",
)

// ErrSupportTicketNotificationRecipientRequired 表示写入通知时 RecipientUserID = 0。
// 主要用于兜底，正常路径不应触发（service 层会预先校验）。
var ErrSupportTicketNotificationRecipientRequired = infraerrors.BadRequest(
	"SUPPORT_TICKET_NOTIFICATION_RECIPIENT_REQUIRED",
	"support ticket notification recipient is required",
)
