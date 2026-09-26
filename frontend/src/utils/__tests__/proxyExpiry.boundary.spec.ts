import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { proxyExpiryLabelKey } from '../proxyExpiry'

const now = new Date('2026-09-18T12:00:00Z')
const atOffset = (ms: number) => new Date(now.getTime() + ms).toISOString()

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})
afterEach(() => vi.useRealTimers())

describe('proxy expiry at the deadline', () => {
  it.each([0, -1, -60 * 60 * 1000, -24 * 60 * 60 * 1000 + 1])(
    'shows expired at offset %i before the backend status changes', offset => {
      expect(proxyExpiryLabelKey(atOffset(offset), 'active')).toEqual({ key: 'admin.proxies.expired' })
    }
  )

  it('keeps a future deadline in the expiring state', () => {
    expect(proxyExpiryLabelKey(atOffset(1), 'active')).toEqual({
      key: 'admin.proxies.expiringInDays', params: { days: 1 }
    })
  })

  it('preserves the overdue-day count after a full day', () => {
    expect(proxyExpiryLabelKey(atOffset(-24 * 60 * 60 * 1000), 'active')).toEqual({
      key: 'admin.proxies.overdueDays', params: { days: 1 }
    })
  })
})
