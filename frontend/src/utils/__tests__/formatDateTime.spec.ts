import { describe, expect, it } from 'vitest'

import { formatDateTime } from '../format'

describe('formatDateTime', () => {
  it('uses a 24-hour clock even for locales that default to 12-hour time', () => {
    const value = new Date(2026, 7, 23, 21, 16, 31)

    const formatted = formatDateTime(value, undefined, 'en-US')

    expect(formatted).toContain('21:16:31')
    expect(formatted).not.toMatch(/\b(?:AM|PM)\b/i)
  })
})
