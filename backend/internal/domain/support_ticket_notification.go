// Package domain — support_ticket_notification.go
//
// 工单站内通知（Support Ticket Notification）领域常量与事件类型。
//
// 该文件放在 domain 层的原因：
//   - 事件类型字符串会同时出现在 repository 层（作为持久化枚举）、service 层
//     （作为分发依据）、handler 层（回给前端 badge 分类）以及邮件模板层
//     （作为 event_type 到模板的路由 key）。避免各层各自 hard-code 字符串。
//   - 不包含任何行为，只声明常量与轻量校验函数，防止跨层导入循环。
//
// 事件类型对应 support_ticket_notification.event_type 列（VARCHAR(50)）。
// migration 层没有 CHECK 约束，由应用层用这三个常量做唯一入口收敛；未来新增事件
// 只需扩展常量 + 处理 service 分发，无需 DDL 变更。
package domain

// 工单通知事件类型枚举。
//
// 与 SupportTicketNotification.event_type 列（VARCHAR(50)）严格对齐；应用层
// 通过这三个常量做唯一入口收敛，新增事件时无需 DDL 变更，只需扩展常量与
// service 分发逻辑，并在邮件模板 map 中注册对应模板。
const (
	// SupportTicketNotificationEventTicketCreated 用于"用户新建工单 → 通知管理员"。
	// recipient 是管理员用户（或 ticket_notify_emails 匹配到的用户），actor 是提交者。
	SupportTicketNotificationEventTicketCreated = "ticket_created"

	// SupportTicketNotificationEventUserReplied 用于"用户在工单里回复 → 通知管理员"。
	// recipient 是管理员，actor 是回复用户。
	SupportTicketNotificationEventUserReplied = "user_replied"

	// SupportTicketNotificationEventAdminReplied 用于"管理员在工单里回复 → 通知工单 owner"。
	// recipient 是工单创建者 (ticket.user_id)，actor 是回复的管理员。
	SupportTicketNotificationEventAdminReplied = "admin_replied"
)

// SupportTicketNotificationExcerptMaxLen 是站内通知 excerpt（正文摘要）的最大字符数。
//
// 与 SQL migration + ent schema 中 excerpt 列的 VARCHAR(500) 严格对齐；
// service 层写入前必须先做 rune-level 截断（see TruncateSupportTicketNotificationExcerpt）。
// 铃铛面板 UI 通常展示前 ~120 字符即换行，但列上限给到 500 让未来 UI 可以按需展开。
const SupportTicketNotificationExcerptMaxLen = 500

// SupportTicketNotificationTitleSnapshotMaxLen 是通知 title_snapshot 列的最大字符数。
// 与 support_ticket.title 上限保持一致（列为 VARCHAR(200)）。
const SupportTicketNotificationTitleSnapshotMaxLen = 200

// IsValidSupportTicketNotificationEvent 判断给定字符串是否为合法事件类型。
// 主要用于 repository 层持久化前的兜底校验；service 层不建议依赖，因为 service
// 只应使用上面的常量，避免非法字符串跑到该函数才被拒绝。
func IsValidSupportTicketNotificationEvent(evt string) bool {
	switch evt {
	case SupportTicketNotificationEventTicketCreated,
		SupportTicketNotificationEventUserReplied,
		SupportTicketNotificationEventAdminReplied:
		return true
	default:
		return false
	}
}

// TruncateSupportTicketNotificationExcerpt 把 raw（工单正文 / 回复正文）截断到
// SupportTicketNotificationExcerptMaxLen 字符（rune 级），返回适合写入 excerpt 列的字符串。
//
// 语义要点：
//   - 不做 trim（调用方自行决定是否 trim；excerpt 通常保留前 N 个字符即可）。
//   - 若原文本本就在阈值内，直接返回。
//   - 超出阈值时截断为前 N-1 个 rune 并附加省略号 "…"，方便 UI 显示。
//
// 该 helper 放在 domain 层是为了让 repository / service / test 都能一致复用；
// service 层实测调用（see support_ticket_notification_service.go）。
func TruncateSupportTicketNotificationExcerpt(raw string) string {
	const suffix = "…"
	// 用 rune 切片保证多字节字符不被拦腰截断。
	runes := []rune(raw)
	if len(runes) <= SupportTicketNotificationExcerptMaxLen {
		return string(runes)
	}
	// 预留一个 rune 位置给省略号，让 UI 明确"内容被截断"。
	return string(runes[:SupportTicketNotificationExcerptMaxLen-1]) + suffix
}

// TruncateSupportTicketNotificationTitleSnapshot 与 excerpt 同思路，用于 title 快照写入前
// 的兜底截断。工单标题上限本身与快照列长度一致（都是 200），正常路径不会触发截断，
// 但工单 title 上限如果未来抬高，这里可以避免 INSERT 失败。
func TruncateSupportTicketNotificationTitleSnapshot(raw string) string {
	runes := []rune(raw)
	if len(runes) <= SupportTicketNotificationTitleSnapshotMaxLen {
		return string(runes)
	}
	return string(runes[:SupportTicketNotificationTitleSnapshotMaxLen])
}
