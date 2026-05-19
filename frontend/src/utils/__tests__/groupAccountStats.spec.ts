import { describe, expect, it } from 'vitest'

import { getAvailableGroupAccountCount } from '@/utils/groupAccountStats'

describe('groupAccountStats utils', () => {
  it('prefers the explicit available count from the backend', () => {
    expect(getAvailableGroupAccountCount({
      available_account_count: 3,
      active_account_count: 5,
      rate_limited_account_count: 9,
    })).toBe(3)
  })

  it('never displays a negative explicit available count', () => {
    expect(getAvailableGroupAccountCount({
      available_account_count: -1,
    })).toBe(0)
  })

  it('falls back to a non-negative active-minus-limited calculation for older payloads', () => {
    expect(getAvailableGroupAccountCount({
      active_account_count: 0,
      rate_limited_account_count: 8,
    })).toBe(0)

    expect(getAvailableGroupAccountCount({
      active_account_count: 2,
      rate_limited_account_count: 8,
    })).toBe(0)
  })

  it('falls back to zero when the count is missing', () => {
    expect(getAvailableGroupAccountCount()).toBe(0)
    expect(getAvailableGroupAccountCount({})).toBe(0)
  })
})
