import { describe, expect, it } from 'vitest'
import {
  formatCompactNumber,
  formatDurationMs,
  formatExactDurationMs,
  formatExactNumber
} from '../opsFormatters'

describe('opsFormatters adaptive units', () => {
  it.each([
    [0, '0'],
    [0.25, '0.25'],
    [999, '999'],
    [999.99, '1K'],
    [1_000, '1K'],
    [12_500, '12.5K'],
    [12_500_000, '12.5M'],
    [-2_500_000_000, '-2.5B'],
    [1_250_000_000_000, '1.3T']
  ])('compacts %s as %s', (value, expected) => {
    expect(formatCompactNumber(value)).toBe(expected)
  })

  it('handles invalid compact values', () => {
    expect(formatCompactNumber(null)).toBe('-')
    expect(formatCompactNumber(Number.POSITIVE_INFINITY)).toBe('-')
  })

  it.each([
    [0, '0 ms'],
    [8.25, '8.3 ms'],
    [999, '999 ms'],
    [1_500, '1.5 s'],
    [125_000, '2.1 min'],
    [7_200_000, '2 h'],
    [172_800_000, '2 d']
  ])('formats %s milliseconds as %s', (value, expected) => {
    expect(formatDurationMs(value)).toBe(expected)
  })

  it('keeps exact values available for titles', () => {
    expect(formatExactNumber(12_345_678.9)).toBe('12,345,678.9')
    expect(formatExactDurationMs(125_000)).toBe('125,000 ms')
  })
})
