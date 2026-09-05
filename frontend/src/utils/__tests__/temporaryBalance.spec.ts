import { describe, expect, it } from 'vitest'

import {
  getActiveTemporaryBalance,
  getTemporaryBalanceStatus,
  isTemporaryBalanceExpired,
} from '../temporaryBalance'

describe('temporary balance helpers', () => {
  const now = new Date('2026-09-02T12:00:00Z')

  it('returns a positive grant only while its expiry is in the future', () => {
    expect(getActiveTemporaryBalance({ temporary_balance: 12.5, temporary_balance_expires_at: '2026-09-03T00:00:00Z' }, now)).toBe(12.5)
    expect(getActiveTemporaryBalance({ temporary_balance: 12.5, temporary_balance_expires_at: '2026-09-01T00:00:00Z' }, now)).toBe(0)
    expect(getActiveTemporaryBalance({ temporary_balance: 12.5, temporary_balance_expires_at: null }, now)).toBe(0)
  })

  it('marks missing or expired grants as inactive, without treating them as usable', () => {
    expect(isTemporaryBalanceExpired({ temporary_balance: 0, temporary_balance_expires_at: null }, now)).toBe(false)
    expect(isTemporaryBalanceExpired({ temporary_balance: 5, temporary_balance_expires_at: '2026-09-01T00:00:00Z' }, now)).toBe(true)
    expect(getTemporaryBalanceStatus({ temporary_balance: 5, temporary_balance_expires_at: '2026-09-01T00:00:00Z' }, now)).toBe('expired')
    expect(getTemporaryBalanceStatus({ temporary_balance: 5, temporary_balance_expires_at: '2026-09-03T00:00:00Z' }, now)).toBe('active')
    expect(getTemporaryBalanceStatus({ temporary_balance: 0, temporary_balance_expires_at: null }, now)).toBe('none')
  })
})
