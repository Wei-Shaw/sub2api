import { describe, expect, it } from 'vitest'
import { resolveCodexUsageWindow } from '@/utils/codexUsage'

describe('resolveCodexUsageWindow', () => {
  it('快照为空时返回空窗口', () => {
    const result = resolveCodexUsageWindow(null, '5h', new Date('2026-02-20T08:00:00Z'))
    expect(result).toEqual({ usedPercent: null, resetAt: null })
  })

  it('优先使用后端提供的绝对重置时间', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_5h_used_percent: 55,
        codex_5h_reset_at: '2026-02-20T10:00:00Z',
        codex_5h_reset_after_seconds: 1
      },
      '5h',
      new Date('2026-02-20T08:00:00Z')
    )

    expect(result.usedPercent).toBe(55)
    expect(result.resetAt).toBe('2026-02-20T10:00:00.000Z')
  })

  it('窗口已过期时自动归零', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_7d_used_percent: 100,
        codex_7d_reset_at: '2026-02-20T07:00:00Z'
      },
      '7d',
      new Date('2026-02-20T08:00:00Z')
    )

    expect(result.usedPercent).toBe(0)
    expect(result.resetAt).toBe('2026-02-20T07:00:00.000Z')
  })

  it('无绝对时间时使用 updated_at + seconds 回退计算', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_5h_used_percent: 20,
        codex_5h_reset_after_seconds: 3600,
        codex_usage_updated_at: '2026-02-20T06:30:00Z'
      },
      '5h',
      new Date('2026-02-20T07:00:00Z')
    )

    expect(result.usedPercent).toBe(20)
    expect(result.resetAt).toBe('2026-02-20T07:30:00.000Z')
  })

  it('支持 legacy primary/secondary 字段映射', () => {
    const snapshot = {
      codex_primary_window_minutes: 10080,
      codex_primary_used_percent: 70,
      codex_primary_reset_after_seconds: 86400,
      codex_secondary_window_minutes: 300,
      codex_secondary_used_percent: 15,
      codex_secondary_reset_after_seconds: 1200,
      codex_usage_updated_at: '2026-02-20T07:00:00Z'
    }

    const result5h = resolveCodexUsageWindow(snapshot, '5h', new Date('2026-02-20T07:05:00Z'))
    const result7d = resolveCodexUsageWindow(snapshot, '7d', new Date('2026-02-20T07:05:00Z'))

    expect(result5h.usedPercent).toBe(15)
    expect(result5h.resetAt).toBe('2026-02-20T07:20:00.000Z')
    expect(result7d.usedPercent).toBe(70)
    expect(result7d.resetAt).toBe('2026-02-21T07:00:00.000Z')
  })

  it('legacy 5h 在 primary<=360 时优先 primary 并支持字符串数字', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_primary_window_minutes: '300',
        codex_primary_used_percent: '21',
        codex_primary_reset_after_seconds: '1800',
        codex_secondary_window_minutes: '10080',
        codex_secondary_used_percent: '99',
        codex_secondary_reset_after_seconds: '99999',
        codex_usage_updated_at: '2026-02-20T08:00:00Z'
      },
      '5h',
      new Date('2026-02-20T08:10:00Z')
    )

    expect(result.usedPercent).toBe(21)
    expect(result.resetAt).toBe('2026-02-20T08:30:00.000Z')
  })

  it('legacy 5h 在无窗口信息时回退 secondary', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_secondary_used_percent: 19,
        codex_secondary_reset_after_seconds: 120,
        codex_usage_updated_at: '2026-02-20T08:00:00Z'
      },
      '5h',
      new Date('2026-02-20T08:00:01Z')
    )

    expect(result.usedPercent).toBe(19)
    expect(result.resetAt).toBe('2026-02-20T08:02:00.000Z')
  })

  it('legacy 场景下 secondary 为 7d 时能正确识别', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_primary_window_minutes: 300,
        codex_primary_used_percent: 5,
        codex_primary_reset_after_seconds: 600,
        codex_secondary_window_minutes: 10080,
        codex_secondary_used_percent: 66,
        codex_secondary_reset_after_seconds: 7200,
        codex_usage_updated_at: '2026-02-20T07:00:00Z'
      },
      '7d',
      new Date('2026-02-20T07:30:00Z')
    )

    expect(result.usedPercent).toBe(66)
    expect(result.resetAt).toBe('2026-02-20T09:00:00.000Z')
  })

  it('绝对时间非法时回退到 updated_at + seconds', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_5h_used_percent: 33,
        codex_5h_reset_at: 'not-a-date',
        codex_5h_reset_after_seconds: 900,
        codex_usage_updated_at: '2026-02-20T07:30:00Z'
      },
      '5h',
      new Date('2026-02-20T07:40:00Z')
    )

    expect(result.usedPercent).toBe(33)
    expect(result.resetAt).toBe('2026-02-20T07:45:00.000Z')
  })

  it('updated_at 非法且无绝对时间时 resetAt 返回 null', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_5h_used_percent: 10,
        codex_5h_reset_after_seconds: 123,
        codex_usage_updated_at: 'invalid-time'
      },
      '5h',
      new Date('2026-02-20T08:00:00Z')
    )

    expect(result.usedPercent).toBe(10)
    expect(result.resetAt).toBeNull()
  })

  it('reset_after_seconds 为负数时按 0 秒处理', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_5h_used_percent: 80,
        codex_5h_reset_after_seconds: -30,
        codex_usage_updated_at: '2026-02-20T08:00:00Z'
      },
      '5h',
      new Date('2026-02-20T07:59:00Z')
    )

    expect(result.usedPercent).toBe(80)
    expect(result.resetAt).toBe('2026-02-20T08:00:00.000Z')
  })

  it('百分比缺失时仍可计算 resetAt 供倒计时展示', () => {
    const result = resolveCodexUsageWindow(
      {
        codex_7d_reset_after_seconds: 60,
        codex_usage_updated_at: '2026-02-20T08:00:00Z'
      },
      '7d',
      new Date('2026-02-20T08:00:01Z')
    )

    expect(result.usedPercent).toBeNull()
    expect(result.resetAt).toBe('2026-02-20T08:01:00.000Z')
  })
})
