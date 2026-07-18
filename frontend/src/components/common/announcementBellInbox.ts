/**
 * announcementBellInbox.ts —— AnnouncementBell 工单 Tab 在"通用信箱（inbox）灰度模式"
 * 下的纯数据映射逻辑（general-inbox PR-9）。
 *
 * 抽成纯函数便于单测（组件本体依赖过多，不便整体挂载），与 pickDefaultBellTab 同风格。
 * 语义：把 inbox 里 namespace=support_ticket 的消息投影为模板既有的 TicketNotification
 * 形状，复用同一套列表 UI；is_read 由累积 ack 水位推断（seq <= localAckSeq 即已读）。
 */
import type { InboxMessage } from '@/api/inbox'
import type { TicketNotification, TicketNotificationEvent } from '@/api/support'

/** 工单事件在通用信箱中的命名空间（与后端 SupportTicketInboxNamespace 对齐）。 */
export const SUPPORT_TICKET_NAMESPACE = 'support_ticket'

/**
 * mapInboxTicketItems 把 inbox 消息映射为 TicketNotification 列表（保持 inbox 的
 * seq 倒序，即最新在前——调用方应传入已按 seq 降序的 messages）。
 */
export function mapInboxTicketItems(
  messages: InboxMessage[],
  localAckSeq: number
): TicketNotification[] {
  return messages
    .filter((m) => m.namespace === SUPPORT_TICKET_NAMESPACE)
    .map((m) => {
      const p = (m.payload ?? {}) as Record<string, unknown>
      return {
        id: m.seq,
        recipient_user_id: 0,
        ticket_id: Number(p.ticket_id) || 0,
        event_type: String(p.event ?? '') as TicketNotificationEvent | string,
        title_snapshot: String(p.title ?? ''),
        excerpt: String(p.excerpt ?? ''),
        actor_user_id: 0,
        is_read: m.seq <= localAckSeq,
        created_at: m.created_at,
        read_at: '0001-01-01T00:00:00Z',
      }
    })
}

/** countInboxTicketUnread 统计 support_ticket 未读条数（seq > ack 水位）。 */
export function countInboxTicketUnread(messages: InboxMessage[], localAckSeq: number): number {
  return messages.filter(
    (m) => m.namespace === SUPPORT_TICKET_NAMESPACE && m.seq > localAckSeq
  ).length
}
