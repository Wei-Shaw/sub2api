import { describe, expect, it } from 'vitest'
import type { UsageProgress } from '@/types'
import { estimateWindowTotalCost } from '../weeklyQuotaEstimate'

const window = (overrides: Partial<UsageProgress> = {}): UsageProgress => ({
  utilization: 40, resets_at: null, remaining_seconds: 0,
  window_stats: { requests: 2, tokens: 300, cost: 12, standard_cost: 24, user_cost: 48 }, ...overrides
})

describe('shared GPT and Claude weekly cost estimate', () => {
  it('uses the existing GPT account-cost formula, not standard or user cost', () => {
    expect(estimateWindowTotalCost(window())).toBe(30)
  })
  it.each([0.01, 0.5, 1, 100, 120])('preserves GPT behavior for positive utilization %s', utilization => {
    expect(estimateWindowTotalCost(window({ utilization }))).toBe(1200 / utilization)
  })
  it.each([0, -1, NaN, Infinity, -Infinity])('rejects invalid utilization %s', utilization => {
    expect(estimateWindowTotalCost(window({ utilization }))).toBeNull()
  })
  it.each([0, -1, NaN, Infinity, Number.MAX_VALUE])('rejects invalid cost/overflow %s', cost => {
    expect(estimateWindowTotalCost(window({ window_stats: { requests: 1, tokens: 300, cost } }))).toBeNull()
  })
  it('adds no request-count or reset-time threshold beyond GPT behavior', () => {
    for (const resets_at of [null, 'invalid', '2020-01-01T00:00:00Z']) {
      expect(estimateWindowTotalCost(window({ resets_at, window_stats: { requests: 0, tokens: 0, cost: 12 } }))).toBe(30)
    }
  })
  it('keeps unknown usage or cost unavailable', () => {
    expect(estimateWindowTotalCost(null)).toBeNull()
    expect(estimateWindowTotalCost(undefined)).toBeNull()
    expect(estimateWindowTotalCost(window({ window_stats: null }))).toBeNull()
  })
})
