import { describe, expect, it } from 'vitest'
import { getAccountExpiryTimestamp } from '../accountExpiry'

describe('getAccountExpiryTimestamp', () => {
  it.each([
    ['2026-01-31T12:34:56.789', 1, '2026-02-28T12:34:56'],
    ['2028-01-31T12:34:56.789', 1, '2028-02-29T12:34:56'],
    ['2028-02-29T12:34:56.789', 12, '2029-02-28T12:34:56'],
    ['2026-12-31T12:34:56.789', 1, '2027-01-31T12:34:56'],
    // With TZ=Australia/Melbourne, October 1 contains a DST gap; avoid using it as an intermediate date.
    ['2028-10-31T02:30:00', 1, '2028-11-30T02:30:00'],
  ])('adds %s by %i calendar months with month-end clamping', (input, months, expected) => {
    const now = new Date(input)
    const originalTimestamp = now.getTime()

    expect(getAccountExpiryTimestamp(months, now)).toBe(new Date(expected).getTime() / 1000)
    expect(now.getTime()).toBe(originalTimestamp)
  })
})
