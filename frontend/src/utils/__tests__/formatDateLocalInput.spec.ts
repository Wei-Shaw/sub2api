import { describe, expect, it, vi REDACTED from 'vitest'

import { formatDateLocalInput REDACTED from '../format'

describe('formatDateLocalInput', () => {
  it('formats the calendar date in local time', () => {
    const localDate = new Date('2026-07-12T16:30:00Z')
    vi.spyOn(localDate, 'getFullYear').mockReturnValue(2026)
    vi.spyOn(localDate, 'getMonth').mockReturnValue(6)
    vi.spyOn(localDate, 'getDate').mockReturnValue(13)

    expect(formatDateLocalInput(localDate)).toBe('2026-07-13')
  REDACTED)

  it('returns an empty string for an invalid date', () => {
    expect(formatDateLocalInput(new Date('invalid'))).toBe('')
  REDACTED)
REDACTED)
