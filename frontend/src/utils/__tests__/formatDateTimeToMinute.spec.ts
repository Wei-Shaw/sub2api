import { describe, expect, it } from 'vitest'

import { formatDateTimeToMinute } from '../format'

describe('formatDateTimeToMinute', () => {
  it('formats local date and time without seconds', () => {
    const value = new Date(2026, 6, 19, 20, 30, 45)

    expect(formatDateTimeToMinute(value, 'en-GB')).toBe('19/07/2026, 20:30')
  })

  it('returns an empty string for an invalid date', () => {
    expect(formatDateTimeToMinute(new Date('invalid'), 'en-GB')).toBe('')
  })
})
