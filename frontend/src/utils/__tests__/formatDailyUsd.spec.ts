import { describe, it, expect } from 'vitest'
import { formatDailyUsd } from '@/utils/format'

describe('formatDailyUsd', () => {
  it('整数走千分位逗号，去尾零', () => {
    expect(formatDailyUsd(100000)).toBe('100,000')
    expect(formatDailyUsd(2000000)).toBe('2,000,000')
    expect(formatDailyUsd(50)).toBe('50')
    expect(formatDailyUsd(0)).toBe('0')
  })

  it('小数走千分位逗号 + 保留有效小数（去尾零）', () => {
    expect(formatDailyUsd(1234.56)).toBe('1,234.56')
    expect(formatDailyUsd(0.05)).toBe('0.05')
    expect(formatDailyUsd(0.123456)).toBe('0.123456')
    // 超过 6 位小数会截断
    expect(formatDailyUsd(0.1234567)).toBe('0.123457')
  })

  it('minimumFractionDigits=2 对齐监控页用量列', () => {
    expect(formatDailyUsd(100000, 2)).toBe('100,000.00')
    expect(formatDailyUsd(0, 2)).toBe('0.00')
    expect(formatDailyUsd(50, 2)).toBe('50.00')
    // 至少 2 位：1234.5 → 1,234.50（补尾零）
    expect(formatDailyUsd(1234.5, 2)).toBe('1,234.50')
    expect(formatDailyUsd(0.1, 2)).toBe('0.10')
    // 超过 2 位的有效小数照常保留
    expect(formatDailyUsd(0.123, 2)).toBe('0.123')
  })

  it('null / undefined / NaN 返回 0', () => {
    expect(formatDailyUsd(null)).toBe('0')
    expect(formatDailyUsd(undefined)).toBe('0')
    expect(formatDailyUsd(NaN)).toBe('0')
    expect(formatDailyUsd(Infinity)).toBe('0')
  })

  it('null 在 minimumFractionDigits=2 下返回 0.00', () => {
    expect(formatDailyUsd(null, 2)).toBe('0.00')
  })
})
