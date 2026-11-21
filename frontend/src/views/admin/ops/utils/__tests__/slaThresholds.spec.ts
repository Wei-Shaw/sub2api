import { describe, expect, it } from 'vitest'

import { getSLAProgressPercent, getSLAThresholdLevel } from '../slaThresholds'

describe('SLA threshold level', () => {
  it('uses the configured boundary with a warning buffer', () => {
    expect(getSLAThresholdLevel(null, 99.5)).toBe('normal')
    expect(getSLAThresholdLevel(100, null)).toBe('normal')
    expect(getSLAThresholdLevel(99.49, 99.5)).toBe('critical')
    expect(getSLAThresholdLevel(99.5, 99.5)).toBe('warning')
    expect(getSLAThresholdLevel(99.6, 99.5)).toBe('normal')
  })

  it('supports a custom threshold', () => {
    expect(getSLAThresholdLevel(89.99, 90)).toBe('critical')
    expect(getSLAThresholdLevel(90, 90)).toBe('warning')
    expect(getSLAThresholdLevel(90.11, 90)).toBe('normal')
  })

  it('derives the progress range from the configured threshold', () => {
    expect(getSLAProgressPercent(89.9, 90)).toBe(0)
    expect(getSLAProgressPercent(90, 90)).toBe(0)
    expect(getSLAProgressPercent(95, 90)).toBe(50)
    expect(getSLAProgressPercent(100, 90)).toBe(100)
    expect(getSLAProgressPercent(99.4, 99.5)).toBe(0)
    expect(getSLAProgressPercent(99.75, 99.5)).toBe(50)
    expect(getSLAProgressPercent(null, 90)).toBe(0)
  })
})
