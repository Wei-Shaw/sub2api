/**
 * ticketUnread.ts —— 工单未读工单数 + 铃铛通知条目 的 Pinia store。
 *
 * 职责边界：
 *  1. 维护 role-aware（user vs admin）的两个统一入口：unreadCount + notifications。
 *     store 内部根据 useAuthStore().isAdmin 分流走 user/admin API，不由调用侧关心。
 *  2. 支持 60s 轮询 + visibilitychange 立即刷新，页面隐藏时暂停请求。
 *  3. `support_ticket_enabled=false` 时所有 fetch/poll 变空跑（early return），
 *     不清空已有 state 也不报错（避免开关切换时闪烁）。
 *  4. logout 清理由 auth store 显式调用 `reset()`（既有约定，见 auth store）。
 *
 * 未读定义（后端计算，前端只透传）：
 *   - 用户视角：owner 的工单里有 admin 回复晚于自己的 last_read_at；
 *   - admin 视角：任意工单 created_at / 最新用户回复 晚于该 admin 的 last_read_at。
 *
 * 与 announcementStore 的关系：铃铛面板会同时展示两者未读；badge = 两者相加。
 * 该 store 只负责工单一侧，跨 store 聚合由 UI 组件层完成。
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  getAdminTicketNotifications,
  getAdminTicketUnreadCount,
  getTicketNotifications,
  getTicketUnreadCount,
  markAdminTicketNotificationRead,
  markAllAdminTicketNotificationsRead,
  markAllTicketNotificationsRead,
  markTicketNotificationRead,
  type TicketNotification,
} from '@/api/support'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

/** 轮询频率：60s，与 spec §7.3 对齐。 */
const POLL_INTERVAL_MS = 60_000

/** debounce 触发 fetchUnreadCount 的最小间隔（用于路由 afterEach 减少无效请求）。 */
const UNREAD_COUNT_FETCH_MIN_INTERVAL_MS = 5_000

export const useTicketUnreadStore = defineStore('ticketUnread', () => {
  // ==================== State ====================

  /** 当前视角下的未读工单数（用户视角 or admin 视角，取决于 role）。 */
  const unreadCount = ref(0)

  /** 当前视角下的通知条目分页缓存（第一页；滚动加载更多时 append）。 */
  const notifications = ref<TicketNotification[]>([])

  /** 通知列表 total（分页页脚 + 判断是否还有下一页）。 */
  const total = ref(0)

  /** 已加载到第几页；1-based。 */
  const page = ref(1)

  /** 每页多少条；固定 20，与后端默认对齐。 */
  const pageSize = ref(20)

  /** loading 只覆盖"当前 in-flight"的 fetch；用于 UI 显示 spinner。 */
  const loadingCount = ref(false)
  const loadingList = ref(false)

  /** 最近一次成功拿到 unreadCount 的时间戳（Date.now()）。 */
  const lastUnreadFetchAt = ref(0)

  /** 轮询定时器 id；仅在 startPolling / stopPolling 内部维护。 */
  let pollTimer: ReturnType<typeof setInterval> | null = null

  /** 记录是否已挂载 visibilitychange listener（用于 stopPolling 幂等卸载）。 */
  let visibilityHandlerAttached = false
  const visibilityHandler = () => {
    if (document.visibilityState === 'visible') {
      // 页面回到前台立即刷新一次，让红点尽快更新（不用等下一个 60s tick）。
      void fetchUnreadCount({ force: true })
    }
  }

  // ==================== Getters ====================

  /** 是否有未读工单（红点显示条件）。 */
  const hasUnread = computed(() => unreadCount.value > 0)

  /** 未读通知条数（本地过滤 + 后端 total 混合，简单实现只看本地 items 里的未读数）。 */
  const unreadNotificationCount = computed(
    () => notifications.value.filter((n) => !n.is_read).length
  )

  /** 是否还有下一页（用于"加载更多"按钮）。 */
  const hasMore = computed(() => notifications.value.length < total.value)

  // ==================== 内部 helpers ====================

  /** feature disabled 时统一 early return；同时把状态归零避免遗留脏数据。 */
  function isFeatureEnabled(): boolean {
    const appStore = useAppStore()
    return appStore.cachedPublicSettings?.support_ticket_enabled === true
  }

  /** role-aware 分流：admin 走 admin API，普通用户走 user API。 */
  function useAdminEndpoints(): boolean {
    const authStore = useAuthStore()
    return authStore.isAdmin
  }

  // ==================== Actions: fetch ====================

  /**
   * 拉未读工单数。
   *
   * @param options.force 忽略节流；用于 route afterEach / visibilitychange 场景。
   *                       默认 false 时会应用 UNREAD_COUNT_FETCH_MIN_INTERVAL_MS 节流。
   */
  async function fetchUnreadCount(options?: { force?: boolean }): Promise<void> {
    if (!isFeatureEnabled()) return

    const now = Date.now()
    if (
      !options?.force &&
      lastUnreadFetchAt.value > 0 &&
      now - lastUnreadFetchAt.value < UNREAD_COUNT_FETCH_MIN_INTERVAL_MS
    ) {
      return
    }
    // 先占位时间戳防止并发触发（失败时会回滚）。
    lastUnreadFetchAt.value = now

    loadingCount.value = true
    try {
      const res = useAdminEndpoints()
        ? await getAdminTicketUnreadCount()
        : await getTicketUnreadCount()
      unreadCount.value = res.count
    } catch (err) {
      // 失败回滚节流时间戳，允许重试
      lastUnreadFetchAt.value = 0
      console.error('ticketUnread: fetchUnreadCount failed', err)
    } finally {
      loadingCount.value = false
    }
  }

  /**
   * 拉通知列表（分页）。
   *
   * @param options.reset  为 true 时清空重来（切换到第一页），否则 append 下一页。
   * @param options.onlyUnread 是否只拉未读；默认 false = 全量。
   */
  async function fetchNotifications(options?: {
    reset?: boolean
    onlyUnread?: boolean
  }): Promise<void> {
    if (!isFeatureEnabled()) return

    const reset = options?.reset ?? false
    const onlyUnread = options?.onlyUnread ?? false
    const targetPage = reset ? 1 : page.value + (notifications.value.length > 0 ? 1 : 0)

    loadingList.value = true
    try {
      const res = useAdminEndpoints()
        ? await getAdminTicketNotifications({
            page: targetPage,
            page_size: pageSize.value,
            only_unread: onlyUnread,
          })
        : await getTicketNotifications({
            page: targetPage,
            page_size: pageSize.value,
            only_unread: onlyUnread,
          })

      if (reset || targetPage === 1) {
        notifications.value = res.items
      } else {
        notifications.value.push(...res.items)
      }
      total.value = res.total
      page.value = targetPage
    } catch (err) {
      console.error('ticketUnread: fetchNotifications failed', err)
    } finally {
      loadingList.value = false
    }
  }

  // ==================== Actions: mark read ====================

  /**
   * 把单条通知标为已读。
   *
   * 语义：
   *  - 乐观更新：先在本地 items 里把 is_read 标 true 并递减 unreadCount；
   *  - 网络失败回滚（防止 UI 与后端不一致）。
   */
  async function markRead(id: number): Promise<void> {
    if (!isFeatureEnabled()) return
    const item = notifications.value.find((n) => n.id === id)
    if (!item) return
    if (item.is_read) return

    // Optimistic update
    item.is_read = true
    item.read_at = new Date().toISOString()

    try {
      if (useAdminEndpoints()) {
        await markAdminTicketNotificationRead(id)
      } else {
        await markTicketNotificationRead(id)
      }
    } catch (err) {
      // Rollback
      item.is_read = false
      item.read_at = '0001-01-01T00:00:00Z'
      console.error('ticketUnread: markRead failed', err)
      throw err
    }
  }

  /**
   * 一次性把当前视角所有通知标已读。
   * 返回后端 affected 计数。失败时不做本地回滚（后端已幂等）。
   */
  async function markAllRead(): Promise<number> {
    if (!isFeatureEnabled()) return 0
    try {
      const res = useAdminEndpoints()
        ? await markAllAdminTicketNotificationsRead()
        : await markAllTicketNotificationsRead()
      // 本地全量翻转为已读
      const now = new Date().toISOString()
      for (const n of notifications.value) {
        if (!n.is_read) {
          n.is_read = true
          n.read_at = now
        }
      }
      return res.affected
    } catch (err) {
      console.error('ticketUnread: markAllRead failed', err)
      throw err
    }
  }

  // ==================== Actions: polling ====================

  /**
   * 启动 60s 轮询 + visibilitychange 立即刷新。
   *
   * 幂等：多次调用只保留一个 timer；stopPolling 后再次调用可重新启动。
   */
  function startPolling(): void {
    if (!isFeatureEnabled()) return
    if (pollTimer !== null) return // 已在轮询

    // 挂载完立即拉一次（force=true 绕过节流）
    void fetchUnreadCount({ force: true })

    pollTimer = setInterval(() => {
      // 页面隐藏时不请求（浏览器 background tab 也会推迟 setInterval，但主动检查更明确）
      if (document.visibilityState !== 'visible') return
      void fetchUnreadCount({ force: true })
    }, POLL_INTERVAL_MS)

    if (!visibilityHandlerAttached) {
      document.addEventListener('visibilitychange', visibilityHandler)
      visibilityHandlerAttached = true
    }
  }

  /** 停止轮询；卸载 visibilitychange。幂等：多次调用无副作用。 */
  function stopPolling(): void {
    if (pollTimer !== null) {
      clearInterval(pollTimer)
      pollTimer = null
    }
    if (visibilityHandlerAttached) {
      document.removeEventListener('visibilitychange', visibilityHandler)
      visibilityHandlerAttached = false
    }
  }

  // ==================== Actions: lifecycle ====================

  /**
   * 完全清空 store，供 auth store 在 logout 时调用。
   * 会同时停止轮询，避免登出后仍有请求泄漏。
   */
  function reset(): void {
    stopPolling()
    unreadCount.value = 0
    notifications.value = []
    total.value = 0
    page.value = 1
    pageSize.value = 20
    loadingCount.value = false
    loadingList.value = false
    lastUnreadFetchAt.value = 0
  }

  return {
    // State (readonly access via computed / ref unwrap)
    unreadCount,
    notifications,
    total,
    page,
    pageSize,
    loadingCount,
    loadingList,

    // Getters
    hasUnread,
    unreadNotificationCount,
    hasMore,

    // Actions
    fetchUnreadCount,
    fetchNotifications,
    markRead,
    markAllRead,
    startPolling,
    stopPolling,
    reset,
  }
})
