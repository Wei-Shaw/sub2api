import { describe, expect, it } from 'vitest'

import { durationSeverity, firstTokenSeverity, formatUsageCacheHitRate, formatUsageOutputRate } from '../latencyHealth'

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
})

describe('formatUsageOutputRate', () => {
  const row = {
    output_tokens: 1000,
    duration_ms: 20_000,
    image_count: 0,
    image_output_tokens: 0,
    billing_mode: 'token',
  }

  it.each(['sync', 'stream', 'ws_v2', 'cyber', undefined] as const)(
    'uses total duration for %s requests', (request_type) => {
      expect(formatUsageOutputRate({ ...row, request_type })).toBe('50.0 tok/s')
    }
  )

  it('rounds to one decimal place', () => {
    expect(formatUsageOutputRate({ ...row, output_tokens: 101, duration_ms: 3000 })).toBe('33.7 tok/s')
  })

  it.each([null, 0, -1, NaN, Infinity])('does not invent a rate for duration %s', (duration_ms) => {
    expect(formatUsageOutputRate({ ...row, duration_ms })).toBe('—')
  })

  it.each([0, -1, NaN, Infinity])('does not invent a rate for output %s', (output_tokens) => {
    expect(formatUsageOutputRate({ ...row, output_tokens })).toBe('—')
  })

  it.each([
    { image_count: 1 },
    { image_output_tokens: 100 },
    { billing_mode: 'image' },
    { request_type: 'live' as const },
    { request_type: 'unknown' as const },
  ])('omits non-text or unknown request rates: %j', (fields) => {
    expect(formatUsageOutputRate({ ...row, ...fields })).toBe('—')
  })
})

describe('formatUsageCacheHitRate', () => {
  const row = {
    input_tokens: 20,
    cache_creation_tokens: 0,
    cache_read_tokens: 80,
    image_count: 0,
    billing_mode: 'token' as const,
    request_type: 'stream' as const,
  }

  it('uses cache_read / (input + cache_create + cache_read)', () => {
    expect(formatUsageCacheHitRate(row)).toBe('80.0%')
    expect(formatUsageCacheHitRate({ ...row, input_tokens: 100, cache_creation_tokens: 100, cache_read_tokens: 100 })).toBe('33.3%')
    expect(formatUsageCacheHitRate({ ...row, input_tokens: 50, cache_creation_tokens: 0, cache_read_tokens: 0 })).toBe('0.0%')
  })

  it('returns em dash when there is no prompt token mass', () => {
    expect(formatUsageCacheHitRate({ ...row, input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0 })).toBe('—')
  })

  it.each([
    { image_count: 1 },
    { billing_mode: 'image' },
    { request_type: 'live' as const },
  ])('omits non-text or unknown request rates: %j', (fields) => {
    expect(formatUsageCacheHitRate({ ...row, ...fields })).toBe('—')
  })
})
