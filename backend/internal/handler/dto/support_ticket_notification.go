// Package dto — support_ticket_notification.go
//
// 工单通知 & 未读计数相关的响应/请求 DTO。
//
// 与 service 层 `SupportTicketNotification` domain 结构解耦：
//   - service 层用 `*int64` / `*time.Time` 表达 optional；
//   - DTO 层扁平化为 JSON 友好的 int64 + time.Time，未设置时用 0 / 零时刻表示，
//     前端根据 `is_read` 字段决定是否解读 `read_at`。
//
// 这两个 shape（unread-count / notification-list）在 user 与 admin 两条路由
// 上完全一致——admin 的差异只在 recipient 是自己（角色隔离由后端 handler 层
// 拿 subject.UserID 决定），DTO 无需为 admin 再单独造一份。
package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SupportTicketUnreadCountResponse 是 GET /support/tickets/unread-count（含 admin 对称路由）的响应。
//
// Count 语义：
//   - 用户视角：调用方名下"存在管理员回复且尚未看过"的工单数量；
//   - 管理员视角：全站"新工单 or 有用户回复且当前 admin 尚未看过"的工单数量。
//
// 使用 int64 是因为 CountUnread* 底层 SQL COUNT() 返回 int64，直连即可。
// 前端会把它当作 badge 数字展示（可能会 clamp 到 99+，但那是前端职责）。
type SupportTicketUnreadCountResponse struct {
	Count int64 `json:"count"`
}

// SupportTicketNotificationItem 是通知列表的单项 shape。
//
// 字段含义：
//   - ID / RecipientUserID：唯一标识 + 归属校验；
//   - TicketID：前端点击跳转 /support/tickets/:id 或 /admin/... 用；
//   - EventType：`ticket_created` / `user_replied` / `admin_replied`（domain 常量）；
//   - TitleSnapshot / Excerpt：事件发生时的标题 & 正文摘要（防止事件后原工单
//     被修改导致 bell 显示错乱）；
//   - ActorUserID：触发者用户 ID；0 表示未知（domain 层用 *int64，DTO 展平）；
//   - IsRead / ReadAt：读游标；ReadAt 为零值时表示未读（IsRead=false）；
//   - CreatedAt：事件时间，前端按 desc 排序展示。
type SupportTicketNotificationItem struct {
	ID              int64     `json:"id"`
	RecipientUserID int64     `json:"recipient_user_id"`
	TicketID        int64     `json:"ticket_id"`
	EventType       string    `json:"event_type"`
	TitleSnapshot   string    `json:"title_snapshot"`
	Excerpt         string    `json:"excerpt"`
	ActorUserID     int64     `json:"actor_user_id"`
	IsRead          bool      `json:"is_read"`
	CreatedAt       time.Time `json:"created_at"`
	ReadAt          time.Time `json:"read_at"`
}

// SupportTicketNotificationItemFromService 把 service domain 结构展平为 JSON DTO。
//
// nil-safe 字段：
//   - ActorUserID *int64 → 0；
//   - ReadAt *time.Time → zero time（前端结合 IsRead 判断，不要单独依赖 ReadAt.IsZero）。
func SupportTicketNotificationItemFromService(n service.SupportTicketNotification) SupportTicketNotificationItem {
	item := SupportTicketNotificationItem{
		ID:              n.ID,
		RecipientUserID: n.RecipientUserID,
		TicketID:        n.TicketID,
		EventType:       n.EventType,
		TitleSnapshot:   n.TitleSnapshot,
		Excerpt:         n.Excerpt,
		IsRead:          n.IsRead,
		CreatedAt:       n.CreatedAt,
	}
	if n.ActorUserID != nil {
		item.ActorUserID = *n.ActorUserID
	}
	if n.ReadAt != nil {
		item.ReadAt = *n.ReadAt
	}
	return item
}

// SupportTicketNotificationItemsFromService 批量转换，nil/空输入返回 []SupportTicketNotificationItem{}
// —— 保持"空数组"而非 nil 让 JSON 序列化稳定，前端可安全 len()/map。
func SupportTicketNotificationItemsFromService(items []service.SupportTicketNotification) []SupportTicketNotificationItem {
	out := make([]SupportTicketNotificationItem, 0, len(items))
	for i := range items {
		out = append(out, SupportTicketNotificationItemFromService(items[i]))
	}
	return out
}

// SupportTicketMarkAllReadResponse 是 POST /notifications/read-all 的响应，
// 返回本次实际标记的行数（前端可展示 toast，例如 "已标记 3 条为已读"）。
type SupportTicketMarkAllReadResponse struct {
	Affected int64 `json:"affected"`
}
