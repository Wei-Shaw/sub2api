import { describe, expect, it } from 'vitest'
import { buildOpenAIUsageRefreshKey } from '../accountUsageRefresh'

describe('buildOpenAIUsageRefreshKey', () => {
  it('会在 codex 快照变化时生成不同 key', () => {
    const base = {
      id: 1,
      platform: 'openai',
      type: 'oauth',
      updated_at: '2026-03-07T10:00:00Z',
      last_used_at: '2026-03-07T09:59:00Z',
      extra: {
        codex_usage_updated_at: '2026-03-07T10:00:00Z',
        codex_5h_used_percent: 0,
        codex_7d_used_percent: 0
      }
    } as any

    const next = {
      ...base,
      extra: {
        ...base.extra,
        codex_usage_updated_at: '2026-03-07T10:01:00Z',
        codex_5h_used_percent: 100
      }
    }

    expect(buildOpenAIUsageRefreshKey(base)).not.toBe(buildOpenAIUsageRefreshKey(next))
  })

  it('会在 codex primary/secondary 新口径字段变化时生成不同 key', () => {
    const base = {
      id: 4,
      platform: 'openai',
      type: 'oauth',
      updated_at: '2026-03-07T10:00:00Z',
      last_used_at: '2026-03-07T09:59:00Z',
      extra: {
        codex_usage_updated_at: '2026-03-07T10:00:00Z',
        codex_primary_used_percent: 0,
        codex_primary_reset_at: '2026-03-07T12:00:00Z',
        codex_primary_reset_after_seconds: 7200,
        codex_primary_window_minutes: 300,
        codex_secondary_used_percent: 2,
        codex_secondary_reset_at: '2026-03-14T10:00:00Z',
        codex_secondary_reset_after_seconds: 604800,
        codex_secondary_window_minutes: 10080,
        codex_5h_used_percent: 100,
        codex_7d_used_percent: 2
      }
    } as any

    const next = {
      ...base,
      extra: {
        ...base.extra,
        codex_primary_used_percent: 1
      }
    }

    expect(buildOpenAIUsageRefreshKey(base)).not.toBe(buildOpenAIUsageRefreshKey(next))
  })

  it('会在 last_used_at 变化时生成不同 key', () => {
    const base = {
      id: 3,
      platform: 'openai',
      type: 'oauth',
      updated_at: '2026-03-07T10:00:00Z',
      last_used_at: '2026-03-07T10:00:00Z',
      extra: {
        codex_usage_updated_at: '2026-03-07T10:00:00Z',
        codex_5h_used_percent: 12,
        codex_7d_used_percent: 24
      }
    } as any

    const next = {
      ...base,
      last_used_at: '2026-03-07T10:02:00Z'
    }

    expect(buildOpenAIUsageRefreshKey(base)).not.toBe(buildOpenAIUsageRefreshKey(next))
  })

  it('非 OpenAI OAuth 账号返回空 key', () => {
    expect(buildOpenAIUsageRefreshKey({
      id: 2,
      platform: 'anthropic',
      type: 'oauth',
      updated_at: '2026-03-07T10:00:00Z',
      last_used_at: '2026-03-07T10:00:00Z',
      extra: {}
    } as any)).toBe('')
  })
})
