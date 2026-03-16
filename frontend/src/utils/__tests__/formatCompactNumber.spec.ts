import { describe, expect, it REDACTED from 'vitest'
import { formatCompactNumber REDACTED from '../format'

describe('formatCompactNumber', () => {
  it('formats boundary values with K/M/B', () => {
    expect(formatCompactNumber(0)).toBe('0')
    expect(formatCompactNumber(999)).toBe('999')
    expect(formatCompactNumber(1000)).toBe('1.0K')
    expect(formatCompactNumber(999999)).toBe('1000.0K')
    expect(formatCompactNumber(1000000)).toBe('1.0M')
    expect(formatCompactNumber(1000000000)).toBe('1.0B')
  REDACTED)

  it('supports disabling billion unit (requests style)', () => {
    expect(formatCompactNumber(1000000000, { allowBillions: false REDACTED)).toBe('1000.0M')
  REDACTED)

  it('returns 0 for nullish input', () => {
    expect(formatCompactNumber(null)).toBe('0')
    expect(formatCompactNumber(undefined)).toBe('0')
  REDACTED)
REDACTED)
