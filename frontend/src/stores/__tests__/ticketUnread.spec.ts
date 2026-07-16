/**
 * ticketUnread.spec.ts —— useTicketUnreadStore 单元测试。
 *
 * 覆盖：
 *  - feature disabled 时 fetch/poll/mark* 全部空跑（不发请求，不改状态）
 *  - role-aware 分流：admin 走 admin API、user 走 user API
 *  - markRead 乐观更新 + 失败回滚
 *  - markAllRead 幂等（0 affected 也不报错）
 *  - reset() 清空状态且停止 polling
 *  - startPolling 幂等；stopPolling 幂等
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// hoisted mocks
const {
  getTicketUnreadCount,
  getTicketNotifications,
  markTicketNotificationRead,
  markAllTicketNotificationsRead,
  getAdminTicketUnreadCount,
  getAdminTicketNotifications,
  markAdminTicketNotificationRead,
  markAllAdminTicketNotificationsRead,
} = vi.hoisted(() => ({
  getTicketUnreadCount: vi.fn(),
  getTicketNotifications: vi.fn(),
  markTicketNotificationRead: vi.fn(),
  markAllTicketNotificationsRead: vi.fn(),
  getAdminTicketUnreadCount: vi.fn(),
  getAdminTicketNotifications: vi.fn(),
  markAdminTicketNotificationRead: vi.fn(),
  markAllAdminTicketNotificationsRead: vi.fn(),
}))

vi.mock('@/api/support', () => ({
  getTicketUnreadCount,
  getTicketNotifications,
  markTicketNotificationRead,
  markAllTicketNotificationsRead,
  getAdminTicketUnreadCount,
  getAdminTicketNotifications,
  markAdminTicketNotificationRead,
  markAllAdminTicketNotificationsRead,
}))

// mock auth store：只需要 isAdmin
const authStub = { isAdmin: false }
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStub,
}))

// mock app store：cachedPublicSettings.support_ticket_enabled
const appStub = { cachedPublicSettings: { support_ticket_enabled: true } as { support_ticket_enabled: boolean } | null }
vi.mock('@/stores/app', () => ({
  useAppStore: () => appStub,
}))

import { useTicketUnreadStore } from '@/stores/ticketUnread'

function makeNotification(overrides: Partial<{ id: number; is_read: boolean }> = {}) {
  return {
    id: overrides.id ?? 1,
    recipient_user_id: 42,
    ticket_id: 100,
    event_type: 'admin_replied',
    title_snapshot: '标题',
    excerpt: '摘要',
    actor_user_id: 0,
    is_read: overrides.is_read ?? false,
    created_at: '2026-07-16T10:00:00Z',
    read_at: '0001-01-01T00:00:00Z',
  }
}

describe('useTicketUnreadStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()
    // 重置 stub 状态
    authStub.isAdmin = false
    appStub.cachedPublicSettings = { support_ticket_enabled: true }
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // --------------------------------------------------------------
  // feature disabled 短路
  // --------------------------------------------------------------

  describe('feature disabled early-return', () => {
    it('support_ticket_enabled=false 时 fetchUnreadCount 不发请求', async () => {
      appStub.cachedPublicSettings = { support_ticket_enabled: false }
      const store = useTicketUnreadStore()

      await store.fetchUnreadCount({ force: true })
      expect(getTicketUnreadCount).not.toHaveBeenCalled()
      expect(getAdminTicketUnreadCount).not.toHaveBeenCalled()
      expect(store.unreadCount).toBe(0)
    })

    it('cachedPublicSettings=null 也视为 disabled（未加载前保守处理）', async () => {
      appStub.cachedPublicSettings = null
      const store = useTicketUnreadStore()

      await store.fetchNotifications({ reset: true })
      expect(getTicketNotifications).not.toHaveBeenCalled()
    })

    it('disabled 时 startPolling 不启动定时器', () => {
      appStub.cachedPublicSettings = { support_ticket_enabled: false }
      const store = useTicketUnreadStore()

      store.startPolling()
      vi.advanceTimersByTime(120_000)
      expect(getTicketUnreadCount).not.toHaveBeenCalled()
      // 关闭时不需要 stopPolling 做任何事，直接调用也应幂等
      store.stopPolling()
    })
  })

  // --------------------------------------------------------------
  // role-aware 分流
  // --------------------------------------------------------------

  describe('role-aware routing', () => {
    it('普通用户走 user API', async () => {
      getTicketUnreadCount.mockResolvedValue({ count: 3 })
      const store = useTicketUnreadStore()

      await store.fetchUnreadCount({ force: true })

      expect(getTicketUnreadCount).toHaveBeenCalledTimes(1)
      expect(getAdminTicketUnreadCount).not.toHaveBeenCalled()
      expect(store.unreadCount).toBe(3)
    })

    it('admin 走 admin API', async () => {
      authStub.isAdmin = true
      getAdminTicketUnreadCount.mockResolvedValue({ count: 7 })
      const store = useTicketUnreadStore()

      await store.fetchUnreadCount({ force: true })

      expect(getAdminTicketUnreadCount).toHaveBeenCalledTimes(1)
      expect(getTicketUnreadCount).not.toHaveBeenCalled()
      expect(store.unreadCount).toBe(7)
    })

    it('markRead 根据角色调用对应 API', async () => {
      getTicketNotifications.mockResolvedValue({
        items: [makeNotification({ id: 101 })],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      markTicketNotificationRead.mockResolvedValue({ id: 101 })

      const store = useTicketUnreadStore()
      await store.fetchNotifications({ reset: true })
      await store.markRead(101)

      expect(markTicketNotificationRead).toHaveBeenCalledWith(101)
      expect(markAdminTicketNotificationRead).not.toHaveBeenCalled()
    })
  })

  // --------------------------------------------------------------
  // markRead 乐观更新 + 回滚
  // --------------------------------------------------------------

  describe('markRead', () => {
    it('乐观更新：本地立刻标 is_read=true', async () => {
      getTicketNotifications.mockResolvedValue({
        items: [makeNotification({ id: 101, is_read: false })],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      // 让 API resolve 拖到后面，验证乐观更新在 await 之前发生
      let resolve!: (v: unknown) => void
      markTicketNotificationRead.mockImplementation(
        () => new Promise((r) => (resolve = r as (v: unknown) => void))
      )

      const store = useTicketUnreadStore()
      await store.fetchNotifications({ reset: true })

      const p = store.markRead(101)
      expect(store.notifications[0].is_read).toBe(true) // 已经翻转
      resolve({ id: 101 })
      await p
    })

    it('API 失败时回滚 is_read=false', async () => {
      getTicketNotifications.mockResolvedValue({
        items: [makeNotification({ id: 101, is_read: false })],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      markTicketNotificationRead.mockRejectedValue(new Error('network'))

      const store = useTicketUnreadStore()
      await store.fetchNotifications({ reset: true })

      await expect(store.markRead(101)).rejects.toThrow('network')
      expect(store.notifications[0].is_read).toBe(false)
    })

    it('已读条目再次 markRead 是 no-op', async () => {
      getTicketNotifications.mockResolvedValue({
        items: [makeNotification({ id: 101, is_read: true })],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      const store = useTicketUnreadStore()
      await store.fetchNotifications({ reset: true })

      await store.markRead(101)
      expect(markTicketNotificationRead).not.toHaveBeenCalled()
    })

    it('不存在的 id 是 no-op', async () => {
      const store = useTicketUnreadStore()
      await store.markRead(999)
      expect(markTicketNotificationRead).not.toHaveBeenCalled()
    })
  })

  // --------------------------------------------------------------
  // markAllRead
  // --------------------------------------------------------------

  describe('markAllRead', () => {
    it('把本地所有未读翻转为已读', async () => {
      getTicketNotifications.mockResolvedValue({
        items: [
          makeNotification({ id: 1, is_read: false }),
          makeNotification({ id: 2, is_read: false }),
          makeNotification({ id: 3, is_read: true }),
        ],
        total: 3,
        page: 1,
        page_size: 20,
        pages: 1,
      })
      markAllTicketNotificationsRead.mockResolvedValue({ affected: 2 })

      const store = useTicketUnreadStore()
      await store.fetchNotifications({ reset: true })
      const affected = await store.markAllRead()

      expect(affected).toBe(2)
      expect(store.notifications.every((n) => n.is_read)).toBe(true)
    })

    it('affected=0 也不报错（幂等）', async () => {
      markAllTicketNotificationsRead.mockResolvedValue({ affected: 0 })
      const store = useTicketUnreadStore()
      await expect(store.markAllRead()).resolves.toBe(0)
    })
  })

  // --------------------------------------------------------------
  // polling 幂等
  // --------------------------------------------------------------

  describe('startPolling / stopPolling', () => {
    it('startPolling 幂等：多次调用只保留一个 interval', async () => {
      getTicketUnreadCount.mockResolvedValue({ count: 1 })
      const store = useTicketUnreadStore()

      store.startPolling()
      store.startPolling()
      store.startPolling()

      // 挂载时立即拉一次
      await vi.waitFor(() => expect(getTicketUnreadCount).toHaveBeenCalledTimes(1))

      // 推进 60s → 触发一次 tick
      // visibilityState 默认 'visible'（jsdom），因此 setInterval 回调会请求
      vi.advanceTimersByTime(60_000)
      await Promise.resolve()
      // 至少多一次调用（初始 + 至少 1 次 tick），但不应该 tick 3 次（多次 start 不叠加）
      const callCount = getTicketUnreadCount.mock.calls.length
      expect(callCount).toBeGreaterThanOrEqual(2)
      expect(callCount).toBeLessThanOrEqual(3)

      store.stopPolling()
    })

    it('stopPolling 后 tick 停止', async () => {
      getTicketUnreadCount.mockResolvedValue({ count: 1 })
      const store = useTicketUnreadStore()

      store.startPolling()
      await vi.waitFor(() => expect(getTicketUnreadCount).toHaveBeenCalled())

      store.stopPolling()
      const callCountAtStop = getTicketUnreadCount.mock.calls.length

      vi.advanceTimersByTime(10 * 60 * 1000)
      expect(getTicketUnreadCount.mock.calls.length).toBe(callCountAtStop)
    })

    it('stopPolling 幂等：多次调用不报错', () => {
      const store = useTicketUnreadStore()
      store.stopPolling()
      store.stopPolling()
      // 未启动过也无 panic
    })
  })

  // --------------------------------------------------------------
  // reset
  // --------------------------------------------------------------

  describe('reset', () => {
    it('清空所有状态且停止 polling', async () => {
      getTicketUnreadCount.mockResolvedValue({ count: 5 })
      getTicketNotifications.mockResolvedValue({
        items: [makeNotification({ id: 1 })],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      })

      const store = useTicketUnreadStore()
      await store.fetchUnreadCount({ force: true })
      await store.fetchNotifications({ reset: true })
      store.startPolling()

      expect(store.unreadCount).toBe(5)
      expect(store.notifications.length).toBe(1)

      store.reset()

      expect(store.unreadCount).toBe(0)
      expect(store.notifications).toEqual([])
      expect(store.total).toBe(0)
      expect(store.page).toBe(1)

      // reset 后 polling 已停：advanceTimersByTime 不再触发 API
      const beforeCalls = getTicketUnreadCount.mock.calls.length
      vi.advanceTimersByTime(120_000)
      expect(getTicketUnreadCount.mock.calls.length).toBe(beforeCalls)
    })
  })
})
