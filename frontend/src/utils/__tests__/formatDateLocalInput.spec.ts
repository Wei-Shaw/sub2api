import { describe, expect, it, vi } from 'vitest'

import { addLocalCalendarDays, formatDateLocalInput } from '../format'

describe('formatDateLocalInput', () => {
  it('formats the calendar date in local time', () => {
    const localDate = new Date('2026-07-12T16:30:00Z')
    vi.spyOn(localDate, 'getFullYear').mockReturnValue(2026)
    vi.spyOn(localDate, 'getMonth').mockReturnValue(6)
    vi.spyOn(localDate, 'getDate').mockReturnValue(13)

    expect(formatDateLocalInput(localDate)).toBe('2026-07-13')
  })

  it('returns an empty string for an invalid date', () => {
    expect(formatDateLocalInput(new Date('invalid'))).toBe('')
  })

  it('moves by local calendar days across daylight saving time', () => {
    const originalTimezone = process.env.TZ
    process.env.TZ = 'America/New_York'
    try {
      const end = new Date(2026, 2, 9, 0, 30)
      const start = addLocalCalendarDays(end, -6)

      expect(formatDateLocalInput(start)).toBe('2026-03-03')
      expect(start.getHours()).toBe(0)
      expect(start.getMinutes()).toBe(30)
    } finally {
      if (originalTimezone === undefined) {
        delete process.env.TZ
      } else {
        process.env.TZ = originalTimezone
      }
    }
  })
})
