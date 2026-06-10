// Package dto — support_ticket.go
//
// 工单系统对外暴露的响应 DTO。设计要点：
//
//   - SupportTicketListItem 编译期不含 ChatContext 字段：列表路径不会泄露大字段
//     给前端（spec D1.A 的二级保险——repo 层已经把 ChatContext 置 nil，DTO 层
//     更进一步把字段从 schema 里完全移除，编译期保证序列化结果不会包含它）。
//   - SupportTicketDetail 是 GetByID 路径的完整投影，含 ChatContext + Replies。
//   - 时间字段统一用 time.Time（JSON 默认 RFC3339），与项目其他 DTO 保持一致。
package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// SupportTicketListItem 是工单列表 / admin 列表的元素 DTO。
//
// 注意 ChatContext 字段缺席：列表场景永远不返回该字段，编译期阻止疏漏。
type SupportTicketListItem struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Category  string     `json:"category"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SupportTicketDetail 是单工单详情（含完整 chat_context 与回复时间线）。
type SupportTicketDetail struct {
	ID          int64                `json:"id"`
	UserID      int64                `json:"user_id"`
	Title       string               `json:"title"`
	Content     string               `json:"content"`
	Category    string               `json:"category"`
	Status      string               `json:"status"`
	Priority    string               `json:"priority"`
	ChatContext *string              `json:"chat_context,omitempty"`
	ClosedAt    *time.Time           `json:"closed_at,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Replies     []SupportTicketReply `json:"replies"`
}

// SupportTicketReply 是单条回复 DTO。
//
// AuthorID 用 *int64：FK ON DELETE SET NULL 后 author 用户被删除时为空；
// is_admin 是写入时的角色快照，独立于 author_id 是否为空。
type SupportTicketReply struct {
	ID        int64     `json:"id"`
	TicketID  int64     `json:"ticket_id"`
	AuthorID  *int64    `json:"author_id,omitempty"`
	IsAdmin   bool      `json:"is_admin"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SupportTicketCategoriesResponse 是 GET /api/v1/support/categories 的返回。
type SupportTicketCategoriesResponse struct {
	Categories      []string `json:"categories"`
	DefaultPriority string   `json:"default_priority"`
}

// SupportTicketListItemFromService 把 service.SupportTicket 翻译成列表元素 DTO。
//
// 显式不携带 ChatContext —— 即使 service 层 list view 有意外携带 ChatContext，
// DTO 字段缺席仍然能保证不落到响应 body。
func SupportTicketListItemFromService(t *service.SupportTicket) *SupportTicketListItem {
	if t == nil {
		return nil
	}
	return &SupportTicketListItem{
		ID:        t.ID,
		UserID:    t.UserID,
		Title:     t.Title,
		Content:   t.Content,
		Category:  t.Category,
		Status:    t.Status,
		Priority:  t.Priority,
		ClosedAt:  t.ClosedAt,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// SupportTicketDetailFromService 把 service.SupportTicketWithReplies 翻译成详情 DTO。
func SupportTicketDetailFromService(twr *service.SupportTicketWithReplies) *SupportTicketDetail {
	if twr == nil {
		return nil
	}
	t := twr.Ticket
	out := &SupportTicketDetail{
		ID:          t.ID,
		UserID:      t.UserID,
		Title:       t.Title,
		Content:     t.Content,
		Category:    t.Category,
		Status:      t.Status,
		Priority:    t.Priority,
		ChatContext: t.ChatContext,
		ClosedAt:    t.ClosedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		Replies:     make([]SupportTicketReply, 0, len(twr.Replies)),
	}
	for i := range twr.Replies {
		out.Replies = append(out.Replies, *SupportTicketReplyFromService(&twr.Replies[i]))
	}
	return out
}

// SupportTicketReplyFromService 翻译单条回复。
func SupportTicketReplyFromService(r *service.SupportTicketReply) *SupportTicketReply {
	if r == nil {
		return nil
	}
	return &SupportTicketReply{
		ID:        r.ID,
		TicketID:  r.TicketID,
		AuthorID:  r.AuthorID,
		IsAdmin:   r.IsAdmin,
		Content:   r.Content,
		CreatedAt: r.CreatedAt,
	}
}
