/**
 * Support Ticket Notification API 客户端签名与 URL 契约测试。
 *
 * 只覆盖：
 *   - URL / method / query 参数是否与后端路由严格对齐（后端 handler test 已断言 recipient 隔离与响应体）
 *   - 类型契约（IsExact）：TicketNotification / TicketUnreadResponse 与后端 dto 字段一一对应
 *
 * 不测：
 *   - 后端错误码映射（依赖 ErrorFrom；由后端 test 覆盖）
 *   - 网络失败重试（apiClient 层已有 client.spec.ts）
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
  },
}))

import {
  getTicketUnreadCount,
  getTicketNotifications,
  markTicketNotificationRead,
  markAllTicketNotificationsRead,
  getAdminTicketUnreadCount,
  getAdminTicketNotifications,
  markAdminTicketNotificationRead,
  markAllAdminTicketNotificationsRead,
  type TicketNotification,
  type TicketUnreadResponse,
  type TicketNotificationListResponse,
  type TicketNotificationMarkReadResponse,
  type TicketNotificationMarkAllReadResponse,
} from '@/api/support'

// ============================================================
// 类型契约（编译期断言，若后端字段变动会直接 tsc 报错）
// ============================================================

type Assert<T extends true> = T
type IsExact<T, U> = (<G>() => G extends T ? 1 : 2) extends <G>() => G extends U ? 1 : 2
  ? (<G>() => G extends U ? 1 : 2) extends <G>() => G extends T ? 1 : 2
    ? true
    : false
  : false

type ExpectedTicketNotification = {
  id: number
  recipient_user_id: number
  ticket_id: number
  event_type: string
  title_snapshot: string
  excerpt: string
  actor_user_id: number
  is_read: boolean
  created_at: string
  read_at: string
}

// 允许 event_type 更严格（联合字符串），因此用 IsAssignable 的双向宽松版本：
// TicketNotification 的字段集合 = ExpectedTicketNotification（值层面），
// 但 event_type 是 union 精化。断言：把 event_type 强转成 string 后完全一致。
type NormalizedTicketNotification = Omit<TicketNotification, 'event_type'> & { event_type: string }

const notificationContract: Assert<
  IsExact<NormalizedTicketNotification, ExpectedTicketNotification>
> = true

const unreadContract: Assert<IsExact<TicketUnreadResponse, { count: number }>> = true

const markReadContract: Assert<IsExact<TicketNotificationMarkReadResponse, { id: number }>> = true

const markAllContract: Assert<
  IsExact<TicketNotificationMarkAllReadResponse, { affected: number }>
> = true

// ============================================================
// URL 契约（运行期）
// ============================================================

describe('Support Ticket Notification API — user endpoints', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('GET /support/tickets/unread-count returns { count }', async () => {
    get.mockResolvedValue({ data: { count: 3 } as TicketUnreadResponse })

    const res = await getTicketUnreadCount()

    expect(get).toHaveBeenCalledWith(
      '/support/tickets/unread-count',
      expect.objectContaining({ signal: undefined })
    )
    expect(res).toEqual({ count: 3 })
  })

  it('GET /support/tickets/notifications sends default page / page_size and omits only_unread when false', async () => {
    const list: TicketNotificationListResponse = {
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 1,
    }
    get.mockResolvedValue({ data: list })

    await getTicketNotifications()

    // only_unread=false 时不传（保持 URL 短，与 backend 默认一致）
    expect(get).toHaveBeenCalledWith(
      '/support/tickets/notifications',
      expect.objectContaining({
        params: { page: 1, page_size: 20 },
      })
    )
  })

  it('GET /support/tickets/notifications passes only_unread=true only when explicit', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 1 } })

    await getTicketNotifications({ page: 2, page_size: 30, only_unread: true })

    expect(get).toHaveBeenCalledWith(
      '/support/tickets/notifications',
      expect.objectContaining({
        params: { page: 2, page_size: 30, only_unread: true },
      })
    )
  })

  it('POST /support/tickets/notifications/:id/read uses interpolated id', async () => {
    post.mockResolvedValue({ data: { id: 101 } })

    const res = await markTicketNotificationRead(101)

    expect(post).toHaveBeenCalledWith('/support/tickets/notifications/101/read')
    expect(res).toEqual({ id: 101 })
  })

  it('POST /support/tickets/notifications/read-all returns affected count', async () => {
    post.mockResolvedValue({ data: { affected: 5 } })

    const res = await markAllTicketNotificationsRead()

    expect(post).toHaveBeenCalledWith('/support/tickets/notifications/read-all')
    expect(res).toEqual({ affected: 5 })
  })
})

describe('Support Ticket Notification API — admin endpoints', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('GET /admin/support/tickets/unread-count', async () => {
    get.mockResolvedValue({ data: { count: 7 } as TicketUnreadResponse })

    const res = await getAdminTicketUnreadCount()

    expect(get).toHaveBeenCalledWith(
      '/admin/support/tickets/unread-count',
      expect.objectContaining({ signal: undefined })
    )
    expect(res).toEqual({ count: 7 })
  })

  it('GET /admin/support/tickets/notifications passes params', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 1 } })

    await getAdminTicketNotifications({ only_unread: true })

    expect(get).toHaveBeenCalledWith(
      '/admin/support/tickets/notifications',
      expect.objectContaining({
        params: { page: 1, page_size: 20, only_unread: true },
      })
    )
  })

  it('POST /admin/support/tickets/notifications/:id/read', async () => {
    post.mockResolvedValue({ data: { id: 555 } })
    const res = await markAdminTicketNotificationRead(555)
    expect(post).toHaveBeenCalledWith('/admin/support/tickets/notifications/555/read')
    expect(res).toEqual({ id: 555 })
  })

  it('POST /admin/support/tickets/notifications/read-all', async () => {
    post.mockResolvedValue({ data: { affected: 3 } })
    const res = await markAllAdminTicketNotificationsRead()
    expect(post).toHaveBeenCalledWith('/admin/support/tickets/notifications/read-all')
    expect(res).toEqual({ affected: 3 })
  })
})

describe('Support Ticket Notification API — type contracts', () => {
  it('TicketNotification stays field-aligned with backend dto.SupportTicketNotificationItem', () => {
    expect(notificationContract).toBe(true)
    expect(unreadContract).toBe(true)
    expect(markReadContract).toBe(true)
    expect(markAllContract).toBe(true)
  })
})
