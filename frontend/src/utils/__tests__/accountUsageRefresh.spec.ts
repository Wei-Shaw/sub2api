import { describe, expect, it } from 'vitest'
import {
  buildAccountUsageRefreshKey,
  buildAnthropicUsageRefreshKey,
  buildGeminiUsageRefreshKey,
  buildGrokUsageRefreshKey,
  buildOpenAIUsageRefreshKey
} from '../accountUsageRefresh'

describe('buildAccountUsageRefreshKey', () => {
  it.each(['oauth', 'setup-token'])('tracks every Claude %s passive snapshot input', (type) => {
    const base = { id: 1, platform: 'anthropic', type, extra: {} } as any
    for (const field of ['last_used_at', 'session_window_start', 'session_window_end', 'session_window_status']) {
      expect(buildAccountUsageRefreshKey({ ...base, [field]: 'new' })).not.toBe(buildAccountUsageRefreshKey(base))
    }
    for (const field of ['session_window_utilization', 'passive_usage_sampled_at', 'passive_usage_7d_utilization', 'passive_usage_7d_reset', 'passive_usage_7d_oi_utilization', 'passive_usage_7d_oi_reset']) {
      expect(buildAccountUsageRefreshKey({ ...base, extra: { [field]: 1 } })).not.toBe(buildAccountUsageRefreshKey(base))
    }
  })

  it.each(['anthropic', 'gemini'])('ignores unrelated %s changes and extra key ordering', (platform) => {
    const base = { id: 1, platform, type: 'oauth', extra: { session_window_utilization: 0, other: 1 } } as any
    expect(buildAccountUsageRefreshKey({ ...base, current_concurrency: 5, extra: { other: 2, session_window_utilization: 0 } } as any)).toBe(buildAccountUsageRefreshKey(base))
    expect(buildAccountUsageRefreshKey({ ...base, last_used_at: 'new' })).not.toBe(buildAccountUsageRefreshKey(base))
  })

  it('tracks an account edit for Gemini but not for Claude', () => {
    const claude = { id: 1, platform: 'anthropic', type: 'oauth', extra: {} } as any
    expect(buildAccountUsageRefreshKey({ ...claude, updated_at: 'new' })).toBe(buildAccountUsageRefreshKey(claude))
    const gemini = { id: 1, platform: 'gemini', type: 'oauth', extra: {} } as any
    expect(buildAccountUsageRefreshKey({ ...gemini, updated_at: 'new' })).not.toBe(buildAccountUsageRefreshKey(gemini))
  })

  it('retains OpenAI keys and excludes unsupported platforms/types', () => {
    const base = { id: 1, platform: 'openai', type: 'oauth', extra: {} } as any
    expect(buildAccountUsageRefreshKey(base)).toBe(buildOpenAIUsageRefreshKey(base))
    expect(buildAccountUsageRefreshKey({ ...base, platform: 'anthropic', type: 'apikey' })).toBe('')
    expect(buildAccountUsageRefreshKey({ ...base, platform: 'antigravity' })).toBe('')
  })
})

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

describe('buildGrokUsageRefreshKey', () => {
  it('changes when a canonical Grok billing or usage snapshot changes', () => {
    const base = {
      platform: 'grok',
      extra: {
        grok_billing_snapshot: { plan: 'Free', usage_percent: 0 },
        grok_usage_snapshot: { subscription_tier: 'Free', status_code: 200 }
      }
    } as any

    expect(buildGrokUsageRefreshKey(base)).not.toBe(buildGrokUsageRefreshKey({
      ...base,
      extra: {
        ...base.extra,
        grok_billing_snapshot: { plan: 'SuperGrok', usage_percent: 0 }
      }
    }))
    expect(buildGrokUsageRefreshKey(base)).not.toBe(buildGrokUsageRefreshKey({
      ...base,
      extra: {
        ...base.extra,
        grok_usage_snapshot: { subscription_tier: 'SuperGrok', status_code: 200 }
      }
    }))
  })

  it('ignores object key order and a legacy alias shadowed by canonical usage', () => {
    const first = {
      platform: 'grok',
      extra: {
        grok_billing_snapshot: {
          plan: 'SuperGrok',
          limits: { monthly: 100, weekly: 25 }
        },
        grok_usage_snapshot: { status_code: 200, subscription_tier: 'SuperGrok' },
        grok_quota_snapshot: { subscription_tier: 'Free' }
      }
    } as any
    const reordered = {
      platform: 'grok',
      extra: {
        grok_quota_snapshot: { subscription_tier: 'SuperGrok Heavy' },
        grok_usage_snapshot: { subscription_tier: 'SuperGrok', status_code: 200 },
        grok_billing_snapshot: {
          limits: { weekly: 25, monthly: 100 },
          plan: 'SuperGrok'
        }
      }
    } as any

    expect(buildGrokUsageRefreshKey(first)).toBe(buildGrokUsageRefreshKey(reordered))
  })

  it('uses the legacy quota alias only when the canonical snapshot is absent', () => {
    const base = {
      platform: 'grok',
      extra: { grok_quota_snapshot: { subscription_tier: 'Free' } }
    } as any
    const next = {
      platform: 'grok',
      extra: { grok_quota_snapshot: { subscription_tier: 'SuperGrok' } }
    } as any

    expect(buildGrokUsageRefreshKey(base)).not.toBe(buildGrokUsageRefreshKey(next))
  })

  it('tracks the legacy tier when the canonical snapshot has no usable tier', () => {
    for (const canonicalSnapshot of [
      { status_code: 200 },
      { status_code: 200, subscription_tier: '   ' },
    ]) {
      const base = {
        platform: 'grok',
        extra: {
          grok_usage_snapshot: canonicalSnapshot,
          grok_quota_snapshot: { subscription_tier: 'Free' },
        },
      } as any
      const next = {
        ...base,
        extra: {
          ...base.extra,
          grok_quota_snapshot: { subscription_tier: 'SuperGrok' },
        },
      }

      expect(buildGrokUsageRefreshKey(base)).not.toBe(buildGrokUsageRefreshKey(next))
    }
  })

  it('returns an empty key for non-Grok accounts', () => {
    expect(buildGrokUsageRefreshKey({
      platform: 'openai',
      extra: { grok_usage_snapshot: { subscription_tier: 'SuperGrok' } }
    } as any)).toBe('')
  })
})

describe('buildAnthropicUsageRefreshKey', () => {
  const base = {
    id: 7,
    platform: 'anthropic',
    type: 'oauth',
    updated_at: '2026-10-25T10:00:00Z',
    last_used_at: '2026-10-25T09:59:00Z',
    rate_limit_reset_at: null,
    session_window_start: '2026-10-25T08:00:00Z',
    session_window_end: '2026-10-25T13:00:00Z',
    session_window_status: 'allowed',
    extra: {
      passive_usage_sampled_at: '2026-10-25T09:59:00Z',
      session_window_utilization: 0.1,
      passive_usage_7d_utilization: 0.2,
      passive_usage_7d_reset: 1761900000,
      passive_usage_7d_oi_utilization: 0.3,
      passive_usage_7d_oi_reset: 1761900000
    }
  } as any

  it('changes when any input of the passive /usage response changes', () => {
    const variants = [
      { last_used_at: '2026-10-25T10:01:00Z' },
      { session_window_start: '2026-10-25T13:00:00Z' },
      { session_window_end: '2026-10-25T18:00:00Z' },
      { session_window_status: 'rejected' },
      { extra: { ...base.extra, passive_usage_sampled_at: '2026-10-25T10:01:00Z' } },
      { extra: { ...base.extra, session_window_utilization: 0.5 } },
      { extra: { ...base.extra, passive_usage_7d_utilization: 0.6 } },
      { extra: { ...base.extra, passive_usage_7d_reset: 1762500000 } },
      { extra: { ...base.extra, passive_usage_7d_oi_utilization: 0.7 } },
      { extra: { ...base.extra, passive_usage_7d_oi_reset: 1762500000 } }
    ]
    for (const variant of variants) {
      expect(buildAnthropicUsageRefreshKey({ ...base, ...variant })).not.toBe(buildAnthropicUsageRefreshKey(base))
    }
  })

  it('covers setup-token accounts', () => {
    const setupToken = { ...base, type: 'setup-token' }
    expect(buildAnthropicUsageRefreshKey(setupToken)).not.toBe('')
    expect(buildAnthropicUsageRefreshKey({
      ...setupToken,
      extra: { ...base.extra, session_window_utilization: 0.9 }
    })).not.toBe(buildAnthropicUsageRefreshKey(setupToken))
  })

  it('returns an empty key for other Anthropic types and other platforms', () => {
    expect(buildAnthropicUsageRefreshKey({ ...base, type: 'apikey' })).toBe('')
    expect(buildAnthropicUsageRefreshKey({ ...base, platform: 'openai' })).toBe('')
  })
})

describe('buildGeminiUsageRefreshKey', () => {
  const base = {
    id: 8,
    platform: 'gemini',
    type: 'oauth',
    updated_at: '2026-10-25T10:00:00Z',
    last_used_at: '2026-10-25T09:59:00Z'
  } as any

  it('changes when the account is used or updated', () => {
    expect(buildGeminiUsageRefreshKey({ ...base, last_used_at: '2026-10-25T10:01:00Z' })).not.toBe(buildGeminiUsageRefreshKey(base))
    expect(buildGeminiUsageRefreshKey({ ...base, updated_at: '2026-10-25T10:01:00Z' })).not.toBe(buildGeminiUsageRefreshKey(base))
  })

  it('returns an empty key for non-Gemini accounts', () => {
    expect(buildGeminiUsageRefreshKey({ ...base, platform: 'anthropic' })).toBe('')
  })
})
