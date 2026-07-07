import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useSubscriptionStore } from '@/stores/subscriptions'

// Mock subscriptions API
const mockGetActiveSubscriptions = vi.fn()

vi.mock('@/api/subscriptions', () => ({
  default: {
    getActiveSubscriptions: (...args: any[]) => mockGetActiveSubscriptions(...args),
  },
}))

const fakeSubscriptions = [
  {
    id: 1,
    user_id: 1,
    group_id: 1,
    status: 'active' as const,
    daily_usage_usd: 5,
    weekly_usage_usd: 20,
    monthly_usage_usd: 50,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    expires_at: '2025-01-01',
  },
  {
    id: 2,
    user_id: 1,
    group_id: 2,
    status: 'active' as const,
    daily_usage_usd: 10,
    weekly_usage_usd: 40,
    monthly_usage_usd: 100,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2024-02-01',
    updated_at: '2024-02-01',
    expires_at: '2025-02-01',
  },
]

describe('useSubscriptionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // --- fetchActiveSubscriptions ---

  describe('fetchActiveSubscriptions', () => {
    it('成功获取活跃订阅', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      const result = await store.fetchActiveSubscriptions()

      expect(result).toEqual(fakeSubscriptions)
      expect(store.activeSubscriptions).toEqual(fakeSubscriptions)
      expect(store.loading).toBe(false)
    })

    it('缓存有效时返回缓存数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      // 第一次请求
      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      // 第二次请求（60秒内）- 应返回缓存
      const result = await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1) // 没有新请求
      expect(result).toEqual(fakeSubscriptions)
    })

    it('缓存过期后重新请求', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      // 推进 61 秒让缓存过期
      vi.advanceTimersByTime(61_000)

      const updatedSubs = [fakeSubscriptions[0]]
      mockGetActiveSubscriptions.mockResolvedValue(updatedSubs)

      const result = await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
      expect(result).toEqual(updatedSubs)
    })

    it('force=true 强制重新请求', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()

      const updatedSubs = [fakeSubscriptions[0]]
      mockGetActiveSubscriptions.mockResolvedValue(updatedSubs)

      const result = await store.fetchActiveSubscriptions(true)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
      expect(result).toEqual(updatedSubs)
    })

    it('并发请求共享同一个 Promise（去重）', async () => {
      let resolvePromise: (v: any) => void
      mockGetActiveSubscriptions.mockImplementation(
        () => new Promise((resolve) => { resolvePromise = resolve })
      )
      const store = useSubscriptionStore()

      // 并发发起两个请求
      const p1 = store.fetchActiveSubscriptions()
      const p2 = store.fetchActiveSubscriptions()

      // 只调用了一次 API
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      // 解决 Promise
      resolvePromise!(fakeSubscriptions)

      const [r1, r2] = await Promise.all([p1, p2])
      expect(r1).toEqual(fakeSubscriptions)
      expect(r2).toEqual(fakeSubscriptions)
    })

    it('API 错误时抛出异常', async () => {
      mockGetActiveSubscriptions.mockRejectedValue(new Error('Network error'))
      const store = useSubscriptionStore()

      await expect(store.fetchActiveSubscriptions()).rejects.toThrow('Network error')
    })
  })

  // --- hasActiveSubscriptions ---

  describe('hasActiveSubscriptions', () => {
    it('有订阅时返回 true', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()

      expect(store.hasActiveSubscriptions).toBe(true)
    })

    it('无订阅时返回 false', () => {
      const store = useSubscriptionStore()
      expect(store.hasActiveSubscriptions).toBe(false)
    })

    it('清除后返回 false', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(store.hasActiveSubscriptions).toBe(true)

      store.clear()
      expect(store.hasActiveSubscriptions).toBe(false)
    })
  })

  // --- invalidateCache ---

  describe('invalidateCache', () => {
    it('失效缓存后下次请求重新获取数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      store.invalidateCache()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
    })
  })

  // --- clear ---

  describe('clear', () => {
    it('清除所有订阅数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(store.activeSubscriptions).toHaveLength(2)

      store.clear()

      expect(store.activeSubscriptions).toHaveLength(0)
      expect(store.hasActiveSubscriptions).toBe(false)
    })
  })

  // --- polling ---

  describe('startPolling / stopPolling', () => {
    it('startPolling 不会创建重复 interval', () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])

      store.startPolling()
      store.startPolling() // 重复调用

      // 推进5分钟只触发一次
      vi.advanceTimersByTime(5 * 60 * 1000)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      store.stopPolling()
    })

    it('stopPolling 停止定期刷新', () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])

      store.startPolling()
      store.stopPolling()

      vi.advanceTimersByTime(10 * 60 * 1000)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
    })
  })
})
