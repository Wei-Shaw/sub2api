/**
 * announcementBellInbox 纯函数单测（general-inbox PR-9 §9.9）：
 * 覆盖 inbox 消息 → 工单通知条目映射、命名空间过滤、未读水位推断、未读计数。
 */
import { describe, expect, it } from 'vitest'
import type { InboxMessage } from '@/api/inbox'
import {
  mapInboxTicketItems,
  countInboxTicketUnread,
  SUPPORT_TICKET_NAMESPACE,
} from '../announcementBellInbox'

function msg(seq: number, namespace: string, payload: Record<string, unknown> = {}): InboxMessage {
  return {
    seq,
    scope: 'broadcast',
    namespace,
    payload,
    created_at: '2026-07-18T00:00:00Z',
  }
}

describe('mapInboxTicketItems', () => {
  it('仅保留 support_ticket namespace 并映射字段', () => {
    const messages: InboxMessage[] = [
      msg(3, SUPPORT_TICKET_NAMESPACE, {
        event: 'admin_replied',
        ticket_id: 42,
        title: '标题',
        excerpt: '摘要',
      }),
      msg(2, 'announcement', { title: '公告' }), // 过滤掉
    ]
    const items = mapInboxTicketItems(messages, 0)
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({
      id: 3,
      ticket_id: 42,
      event_type: 'admin_replied',
      title_snapshot: '标题',
      excerpt: '摘要',
      is_read: false,
      created_at: '2026-07-18T00:00:00Z',
    })
  })

  it('is_read 由 localAckSeq 推断（seq <= 水位即已读）', () => {
    const messages = [
      msg(5, SUPPORT_TICKET_NAMESPACE, { ticket_id: 1 }),
      msg(10, SUPPORT_TICKET_NAMESPACE, { ticket_id: 2 }),
    ]
    const items = mapInboxTicketItems(messages, 5)
    expect(items.find((i) => i.id === 5)?.is_read).toBe(true) // 5 <= 5
    expect(items.find((i) => i.id === 10)?.is_read).toBe(false) // 10 > 5
  })

  it('payload 缺字段时降级为默认值，不抛错', () => {
    const items = mapInboxTicketItems([msg(1, SUPPORT_TICKET_NAMESPACE)], 0)
    expect(items[0]).toMatchObject({
      id: 1,
      ticket_id: 0,
      event_type: '',
      title_snapshot: '',
      excerpt: '',
    })
  })
})

describe('countInboxTicketUnread', () => {
  it('只统计 support_ticket 且 seq > 水位的消息', () => {
    const messages = [
      msg(6, SUPPORT_TICKET_NAMESPACE),
      msg(7, SUPPORT_TICKET_NAMESPACE),
      msg(5, SUPPORT_TICKET_NAMESPACE), // <= 水位，已读
      msg(9, 'announcement'), // 其它 namespace 不计
    ]
    expect(countInboxTicketUnread(messages, 5)).toBe(2)
  })

  it('全部已读时为 0', () => {
    const messages = [msg(1, SUPPORT_TICKET_NAMESPACE), msg(2, SUPPORT_TICKET_NAMESPACE)]
    expect(countInboxTicketUnread(messages, 10)).toBe(0)
  })
})
