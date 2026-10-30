import { i18n } from '@/i18n'
import quotaZh from '@/i18n/locales/zh/admin/quota'
i18n.global.setLocaleMessage('zh', { admin: { quota: quotaZh } })
i18n.global.locale.value = 'zh'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createAccountUsageCache, ACCOUNT_USAGE_TTL, ACCOUNT_USAGE_LOAD_ERROR, supportsBatchUsage } from '../accountUsageCache'
import { quotaCountdown } from '../quotaCountdown'
import type { AccountUsageInfo } from '@/types'

const usage = (value = 0.8) => ({ updated_at: new Date().toISOString(), quota_pools: [{ id: 'gemini', name: 'Gemini', windows: [{ id: '5h', remaining: value }] }] }) as AccountUsageInfo
const result = (ids: number[], value = 0.8) => ({ usage: Object.fromEntries(ids.map(id => [id, usage(value)])), errors: {} })
const storage = () => {
  const map = new Map<string, string>()
  return { getItem: (key: string) => map.get(key) ?? null, setItem: (key: string, value: string) => { map.set(key, value) } }
}

describe('shared persisted account usage', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-03T04:00:00Z')) })
  afterEach(() => vi.useRealTimers())

  it('coalesces overlapping list and overview requests into one batch', async () => {
    const fetch = vi.fn(async (ids: number[]) => result(ids))
    const cache = createAccountUsageCache(fetch, storage())
    await Promise.all([cache.load([1, 2]), cache.load([2, 3])])
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0][0]).toEqual([1, 2, 3])
    await cache.load([1, 2, 3])
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('upgrades a queued refresh without duplicating the request', async () => {
    const fetch = vi.fn(async (ids: number[]) => result(ids))
    const cache = createAccountUsageCache(fetch, storage())
    const first = cache.load([1])
    expect(cache.loading.value[1]).toBe(true)
    await Promise.all([first, cache.load([1], true)])
    expect(fetch.mock.calls).toEqual([[[1], true]])
    expect(cache.loading.value[1]).toBe(false)
  })

  it('loads new accounts while another batch is in flight without fetching overlaps twice', async () => {
    let finish!: (value: ReturnType<typeof result>) => void
    const fetch = vi.fn()
      .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
      .mockImplementation(async (ids: number[]) => result(ids, 0.2))
    const cache = createAccountUsageCache(fetch, storage())
    const first = cache.load([1]); await Promise.resolve()
    const second = cache.load([1, 2]); await Promise.resolve()
    expect(fetch.mock.calls.map(call => call[0])).toEqual([[1], [2]])
    finish(result([1])); await Promise.all([first, second])
    expect(cache.usage.value[1]?.quota_pools?.[0].windows[0].remaining).toBe(0.8)
    expect(cache.usage.value[2]?.quota_pools?.[0].windows[0].remaining).toBe(0.2)
  })

  it('survives a page reload for five minutes without extending the deadline', async () => {
    const disk = storage()
    const fetch = vi.fn(async (ids: number[]) => result(ids))
    await createAccountUsageCache(fetch, disk).load([1])
    vi.advanceTimersByTime(ACCOUNT_USAGE_TTL - 1000)
    const reloaded = createAccountUsageCache(fetch, disk)
    expect(reloaded.usage.value[1]).not.toBeNull()
    await reloaded.load([1])
    expect(fetch).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1001)
    expect(fetch).toHaveBeenCalledTimes(1) // Expiry alone never queries.
    await reloaded.load([1])
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('fetches only accounts absent from the shared cache', async () => {
    const fetch = vi.fn(async (ids: number[]) => result(ids))
    const cache = createAccountUsageCache(fetch, storage())
    await cache.load([1, 2])
    await cache.load([1, 2, 3])
    expect(fetch.mock.calls[1][0]).toEqual([3])
  })

  it('shares an in-flight request even when the other view clicks refresh', async () => {
    let finish!: (value: ReturnType<typeof result>) => void
    const fetch = vi.fn(() => new Promise<ReturnType<typeof result>>(resolve => { finish = resolve }))
    const cache = createAccountUsageCache(fetch, storage())
    const list = cache.load([1])
    await Promise.resolve()
    expect(cache.loading.value[1]).toBe(true)
    const overview = cache.load([1], true)
    await Promise.resolve()
    expect(fetch).toHaveBeenCalledTimes(1)
    finish(result([1])); await Promise.all([list, overview])
    expect(cache.loading.value[1]).toBe(false)
  })

  it('explicit refresh updates both subscribers and persisted data', async () => {
    let value = 0.8
    const disk = storage()
    const fetch = vi.fn(async (ids: number[]) => result(ids, value))
    const cache = createAccountUsageCache(fetch, disk)
    const list = cache.usage, overview = cache.usage
    await cache.load([1]); value = 0.4
    await cache.load([1], true)
    expect(fetch.mock.calls[1]).toEqual([[1], true])
    expect(list.value[1]).toBe(overview.value[1])
    expect(overview.value[1]?.quota_pools?.[0].windows[0].remaining).toBe(0.4)
    await createAccountUsageCache(fetch, disk).load([1])
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('persists failures and releases loading state instead of retrying on each view', async () => {
    const fetch = vi.fn().mockRejectedValue(new Error('offline'))
    const disk = storage()
    const cache = createAccountUsageCache(fetch, disk)
    await cache.load([1])
    expect(cache.errors.value[1]).toBe(ACCOUNT_USAGE_LOAD_ERROR)
    expect(cache.loading.value[1]).toBe(false)
    await createAccountUsageCache(fetch, disk).load([1])
    expect(fetch).toHaveBeenCalledTimes(1)
    await cache.load([1], true)
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('does not mix snapshots between signed-in admins or accept an old in-flight result', async () => {
    let user = 'first'
    let finish!: (value: ReturnType<typeof result>) => void
    const fetch = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockImplementation(async (ids: number[]) => result(ids, 0.2))
    const cache = createAccountUsageCache(fetch, storage(), () => user)
    const old = cache.load([1]); await Promise.resolve()
    user = 'second'
    await cache.load([1])
    finish(result([1], 0.9)); await old; await Promise.resolve()
    expect(cache.usage.value[1]?.quota_pools?.[0].windows[0].remaining).toBe(0.2)
  })

  it('does not accept an earlier request after switching away and back to the same admin', async () => {
    let user = 'first'
    let finishOld!: (value: ReturnType<typeof result>) => void
    let finishNew!: (value: ReturnType<typeof result>) => void
    const fetch = vi.fn()
      .mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { finishNew = resolve }))
    const cache = createAccountUsageCache(fetch, storage(), () => user)
    const old = cache.load([1]); await Promise.resolve()
    user = 'second'; await cache.load([])
    user = 'first'
    const current = cache.load([1]); await Promise.resolve()
    finishOld(result([1], 0.9)); await old; await Promise.resolve()
    expect(cache.usage.value[1]).toBeUndefined()
    expect(cache.loading.value[1]).toBe(true)
    finishNew(result([1], 0.2)); await current
    expect(cache.usage.value[1]?.quota_pools?.[0].windows[0].remaining).toBe(0.2)
  })

  it('does not refresh the TTL merely because a cell emits the cached data', async () => {
    const fetch = vi.fn(async (ids: number[]) => result(ids))
    const cache = createAccountUsageCache(fetch, storage())
    await cache.load([1])
    vi.advanceTimersByTime(ACCOUNT_USAGE_TTL - 1000)
    cache.set(1, cache.usage.value[1]!)
    vi.advanceTimersByTime(1001)
    await cache.load([1])
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})

describe('quota countdown', () => {
  it('shows reset time without inventing a refill or querying on expiry', () => {
    const now = Date.parse('2026-10-03T04:00:00Z')
    expect(quotaCountdown(undefined, now)).toBe('—')
    expect(quotaCountdown('invalid', now)).toBe('—')
    expect(quotaCountdown('2026-10-03T03:00:00Z', now)).toBe('待更新')
    expect(quotaCountdown('2026-10-03T04:20:00Z', now)).toBe('20m')
    expect(quotaCountdown('2026-10-03T06:13:00Z', now)).toBe('2h 13m')
    expect(quotaCountdown('2026-10-07T05:00:00Z', now)).toBe('4d 1h')
  })
})

describe('batch usage eligibility shared by list and overview', () => {
  it('retains upstream account-type rules', () => {
    for (const platform of ['antigravity', 'openai', 'grok'] as const) {
      expect(supportsBatchUsage({ platform, type: 'oauth' })).toBe(true)
      expect(supportsBatchUsage({ platform, type: 'setup-token' })).toBe(false)
      expect(supportsBatchUsage({ platform, type: 'apikey' })).toBe(false)
    }
    expect(supportsBatchUsage({ platform: 'anthropic', type: 'setup-token' })).toBe(true)
    expect(supportsBatchUsage({ platform: 'anthropic', type: 'apikey' })).toBe(false)
    expect(supportsBatchUsage({ platform: 'gemini', type: 'apikey' })).toBe(true)
    expect(supportsBatchUsage({ platform: 'ollama', type: 'apikey' })).toBe(false)
  })
})
