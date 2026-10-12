import { describe, expect, it, vi } from 'vitest'

import {
  formatDateTimeLocalInput,
  getBrowserTimeZone,
  parseDateTimeLocalInput
} from '../format'

describe('formatDateTimeLocalInput', () => {
  it('round-trips a minute-precision local timestamp', () => {
    const timestamp = Math.floor(new Date(2026, 7, 30, 7, 24).getTime() / 1000)

    expect(formatDateTimeLocalInput(timestamp)).toBe('2026-08-30T07:24')
    expect(parseDateTimeLocalInput('2026-08-30T07:24')).toBe(timestamp)
  })
})

describe('parseDateTimeLocalInput', () => {
  it('validates leap days and years below 100 without the Date constructor offset', () => {
    expect(parseDateTimeLocalInput('2024-02-29T12:00')).not.toBeNull()
    expect(parseDateTimeLocalInput('2025-02-29T12:00')).toBeNull()
    const value = parseDateTimeLocalInput('0096-02-29T12:00')!
    expect(new Date(value * 1000).getFullYear()).toBe(96)
    expect(parseDateTimeLocalInput('0000-01-01T00:00')).toBeNull()
  })
  it('uses local date components and rejects timezone-bearing values', () => {
    const expected = Math.floor(new Date(2026, 7, 30, 7, 24, 10).getTime() / 1000)

    expect(parseDateTimeLocalInput('2026-08-30T07:24:10')).toBe(expected)
    expect(parseDateTimeLocalInput('2026-08-30T07:24:10+08:00')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T07:24:10Z')).toBeNull()
  })

  it('rejects empty, malformed, and overflowing calendar values', () => {
    expect(parseDateTimeLocalInput('')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30 07:24')).toBeNull()
    expect(parseDateTimeLocalInput('2026-02-30T07:24')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T24:00')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T07:60')).toBeNull()
  })

  it('accepts optional seconds and fractional seconds', () => {
    const expected = Math.floor(new Date(2026, 7, 30, 7, 24, 10, 120).getTime() / 1000)

    expect(parseDateTimeLocalInput('2026-08-30T07:24:10.12')).toBe(expected)
  })
})

describe('getBrowserTimeZone', () => {
  it('falls back to UTC when Intl is unavailable', () => {
    const spy = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(() => { throw new Error('unavailable') })
    try {
      expect(getBrowserTimeZone()).toBe('UTC')
    } finally {
      spy.mockRestore()
    }
  })
  it('returns a timezone identifier or the UTC fallback', () => {
    expect(getBrowserTimeZone()).toBeTruthy()
  })
})
