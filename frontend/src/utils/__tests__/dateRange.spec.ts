import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { getLast24HourRange } from '../dateRange'

const RFC3339_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/

describe('getLast24HourRange', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns RFC3339 boundaries aligned to the minute, exactly 24 hours apart', () => {
    vi.setSystemTime(new Date(2026, 6, 11, 14, 3, 22, 456))

    const { start, end } = getLast24HourRange()

    expect(start).toMatch(RFC3339_RE)
    expect(end).toMatch(RFC3339_RE)
    expect(new Date(end).getSeconds()).toBe(0)
    expect(new Date(end).getMilliseconds()).toBe(0)
    expect(new Date(end).getTime() - new Date(start).getTime()).toBe(24 * 60 * 60 * 1000)
  })

  it('returns identical boundaries for consecutive calls within the same minute', () => {
    vi.setSystemTime(new Date(2026, 6, 11, 14, 3, 5))
    const first = getLast24HourRange()

    vi.setSystemTime(new Date(2026, 6, 11, 14, 3, 55))
    const second = getLast24HourRange()

    expect(second).toEqual(first)
  })
})
