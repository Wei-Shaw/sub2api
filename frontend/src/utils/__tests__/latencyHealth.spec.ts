import { describe, expect, it } from 'vitest'

import { durationSeverity, firstTokenSeverity, generationTpsSeverity } from '../latencyHealth'

describe('latencyHealth', () => {
  it('classifies first-token latency at 10s/30s/60s boundaries', () => {
    expect(firstTokenSeverity(0)).toBe('good')
    expect(firstTokenSeverity(9_999)).toBe('good')
    expect(firstTokenSeverity(10_000)).toBe('warn')
    expect(firstTokenSeverity(29_999)).toBe('warn')
    expect(firstTokenSeverity(30_000)).toBe('slow')
    expect(firstTokenSeverity(59_999)).toBe('slow')
    expect(firstTokenSeverity(60_000)).toBe('critical')
  })

  it('classifies total duration at 1min/3min/5min boundaries', () => {
    expect(durationSeverity(0)).toBe('good')
    expect(durationSeverity(59_999)).toBe('good')
    expect(durationSeverity(60_000)).toBe('warn')
    expect(durationSeverity(179_999)).toBe('warn')
    expect(durationSeverity(180_000)).toBe('slow')
    expect(durationSeverity(299_999)).toBe('slow')
    expect(durationSeverity(300_000)).toBe('critical')
  })

  it('classifies generation TPS thresholds (higher is healthier)', () => {
    expect(generationTpsSeverity(30)).toBe('good')
    expect(generationTpsSeverity(29.99)).toBe('warn')
    expect(generationTpsSeverity(15)).toBe('warn')
    expect(generationTpsSeverity(14.99)).toBe('slow')
    expect(generationTpsSeverity(5)).toBe('slow')
    expect(generationTpsSeverity(4.99)).toBe('critical')
  })
})
