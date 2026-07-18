// Package service — support_ticket_inbox_bridge.go
//
// general-inbox PR-6：把工单通知事件"双写"进通用信箱（inbox）。
//
// 设计要点：
//   - 复用 SupportTicketNotificationService 已有的三个 Notify 触发点，在写完旧的
//     support_ticket_notification 记录 + 邮件之后，再向 inbox 发布同一事件；
//   - direct（单播）用于 admin 回复 → 工单 owner；broadcast（广播 + role=admin
//     定向）用于用户新建/回复 → 全体管理员。广播避免了对"管理员群体"做一次 fan-out
//     写放大，命中判定推迟到 catchup 读取阶段；
//   - dedup_key 采用 "<event>:<id>" 形式（新建按 ticket_id，回复按 reply_id），
//     保证重复触发（如重试）幂等；
//   - 所有失败都 swallow 成 warn 日志：inbox 发布失败绝不回滚工单主流程，
//     与旧通知记录的"log warn + swallow"策略一致；
//   - 整体受 inboxEnabled（config.Inbox.V1Enabled）+ inboxPub != nil 双重开关控制，
//     灰度期间可随时回退到仅旧通知表。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/inbox"
)

// SupportTicketInboxNamespace 是工单事件在通用信箱中的命名空间。
const SupportTicketInboxNamespace = "support_ticket"

// adminTargetingJSON 是"仅投递给管理员"的广播定向表达式：role == "admin"。
// 与 AttributeProvider 暴露的 role 属性（server 层 inbox_glue.go）对应。
var adminTargetingJSON = json.RawMessage(`{"op":"equals","attr":"role","value":"admin"}`)

// ticketInboxPayload 是工单事件写入 inbox 的 payload（前端铃铛下拉直接渲染）。
type ticketInboxPayload struct {
	Namespace string `json:"namespace"`
	Event     string `json:"event"`
	TicketID  int64  `json:"ticket_id"`
	Title     string `json:"title"`
	Excerpt   string `json:"excerpt,omitempty"`
	ActorName string `json:"actor_name,omitempty"`
	PortalURL string `json:"portal_url,omitempty"`
}

// inboxReady 汇总 inbox 发布的双重开关：装配了 publisher 且灰度开关打开。
func (s *SupportTicketNotificationService) inboxReady() bool {
	return s.inboxEnabled && s.inboxPub != nil
}

// buildTicketInboxPayload 组装并序列化工单事件 payload。
func buildTicketInboxPayload(event string, evt SupportTicketEventContext, actorName, portalURL string) (json.RawMessage, error) {
	p := ticketInboxPayload{
		Namespace: SupportTicketInboxNamespace,
		Event:     event,
		TicketID:  evt.Ticket.ID,
		Title:     evt.Ticket.Title,
		Excerpt:   evt.Excerpt,
		ActorName: actorName,
		PortalURL: portalURL,
	}
	return json.Marshal(p)
}

// publishInboxToAdmins 向全体管理员广播一条工单事件（用户新建/回复）。
// dedupID：新建事件传 ticket_id，回复事件传 reply_id。
func (s *SupportTicketNotificationService) publishInboxToAdmins(
	ctx context.Context,
	evt SupportTicketEventContext,
	event, actorName, portalURL string,
	dedupID int64,
) {
	if !s.inboxReady() {
		return
	}
	payload, err := buildTicketInboxPayload(event, evt, actorName, portalURL)
	if err != nil {
		slog.Warn("support_ticket_inbox: marshal broadcast payload failed",
			"ticket_id", evt.Ticket.ID, "event", event, "err", err)
		return
	}
	if _, _, err := s.inboxPub.PublishBroadcast(ctx, inbox.PublishBroadcastInput{
		Namespace: SupportTicketInboxNamespace,
		DedupKey:  ticketInboxDedupKey(event, dedupID),
		Targeting: adminTargetingJSON,
		Payload:   payload,
	}); err != nil {
		slog.Warn("support_ticket_inbox: publish broadcast failed",
			"ticket_id", evt.Ticket.ID, "event", event, "err", err)
	}
}

// publishInboxDirect 单播一条工单事件给指定用户（admin 回复 → 工单 owner）。
func (s *SupportTicketNotificationService) publishInboxDirect(
	ctx context.Context,
	recipientID int64,
	evt SupportTicketEventContext,
	event, actorName, portalURL string,
	dedupID int64,
) {
	if !s.inboxReady() {
		return
	}
	if recipientID <= 0 {
		return
	}
	payload, err := buildTicketInboxPayload(event, evt, actorName, portalURL)
	if err != nil {
		slog.Warn("support_ticket_inbox: marshal direct payload failed",
			"ticket_id", evt.Ticket.ID, "event", event, "err", err)
		return
	}
	if _, _, err := s.inboxPub.PublishToUser(ctx, inbox.PublishDirectInput{
		RecipientID: recipientID,
		Namespace:   SupportTicketInboxNamespace,
		DedupKey:    ticketInboxDedupKey(event, dedupID),
		Payload:     payload,
	}); err != nil {
		slog.Warn("support_ticket_inbox: publish direct failed",
			"ticket_id", evt.Ticket.ID, "recipient_user_id", recipientID, "event", event, "err", err)
	}
}

// ticketInboxDedupKey 组装 dedup_key："<event>:<id>"。event 为 snake_case、id 为十进制，
// 均落在 inbox dedupKeyPattern 允许的字符集内。
func ticketInboxDedupKey(event string, id int64) string {
	return fmt.Sprintf("%s:%d", event, id)
}
