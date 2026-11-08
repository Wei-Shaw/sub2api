import { describe, expect, it } from 'vitest'
import {
  durationOverheadSeverity,
  durationSeverity,
  firstTokenOverheadSeverity,
  firstTokenSeverity,
  formatUsageOutputRate,
  upstreamOverheadMs,
} from '../latencyHealth'

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

describe('upstreamOverheadMs', () => {
  it('返回本站与上游的毫秒差值', () => {
    expect(upstreamOverheadMs(1500, 1000)).toBe(500)
    expect(upstreamOverheadMs(1000, 1000)).toBe(0)
    // 本站反而更快时是负数，不做截断，否则会掩盖时钟/采样异常。
    expect(upstreamOverheadMs(800, 1000)).toBe(-200)
    // 带小数也要取整成毫秒整数。
    expect(upstreamOverheadMs(1234.6, 1000)).toBe(235)
  })

  it('任一侧缺失时返回 null，而不是 0', () => {
    // 0 的含义是「完全没有中转开销」，与「上游数据还没对账到」正好相反，
    // 用 0 顶替会让管理员以为中转毫无成本。
    expect(upstreamOverheadMs(null, 1000)).toBeNull()
    expect(upstreamOverheadMs(undefined, 1000)).toBeNull()
    expect(upstreamOverheadMs(1000, null)).toBeNull()
    expect(upstreamOverheadMs(1000, undefined)).toBeNull()
    // 与旧实现不同：这里不再需要「上游为 0 就返回 null」的特判（没有除法了），
    // 上游真的是 0 时差值本身就是有意义的信息。
    expect(upstreamOverheadMs(1000, 0)).toBe(1000)
  })
})

describe('中转开销分档', () => {
  it('首字开销按 300ms / 1s / 3s 分档', () => {
    expect(firstTokenOverheadSeverity(-500)).toBe('good')
    expect(firstTokenOverheadSeverity(0)).toBe('good')
    expect(firstTokenOverheadSeverity(299)).toBe('good')
    expect(firstTokenOverheadSeverity(300)).toBe('warn')
    expect(firstTokenOverheadSeverity(999)).toBe('warn')
    expect(firstTokenOverheadSeverity(1_000)).toBe('slow')
    expect(firstTokenOverheadSeverity(2_999)).toBe('slow')
    expect(firstTokenOverheadSeverity(3_000)).toBe('critical')
  })

  it('总耗时开销按 1s / 3s / 10s 分档（比首字放宽一档）', () => {
    expect(durationOverheadSeverity(-1)).toBe('good')
    expect(durationOverheadSeverity(999)).toBe('good')
    expect(durationOverheadSeverity(1_000)).toBe('warn')
    expect(durationOverheadSeverity(2_999)).toBe('warn')
    expect(durationOverheadSeverity(3_000)).toBe('slow')
    expect(durationOverheadSeverity(9_999)).toBe('slow')
    expect(durationOverheadSeverity(10_000)).toBe('critical')
  })

  it('同一毫秒数在首字与总耗时两套阈值下分档不同', () => {
    // 1.5 秒中转开销：对首字是「明显偏重」，对总耗时只是「略可感」——
    // 总耗时量级更大，容忍度本就该更宽。
    expect(firstTokenOverheadSeverity(1_500)).toBe('slow')
    expect(durationOverheadSeverity(1_500)).toBe('warn')
  })
})
