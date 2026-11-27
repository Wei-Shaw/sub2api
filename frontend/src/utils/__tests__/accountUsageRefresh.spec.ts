import { describe, expect, it } from 'vitest'
import { buildAccountUsageRefreshKey, buildGrokUsageRefreshKey, buildOpenAIUsageRefreshKey } from '../accountUsageRefresh'

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
    expect(buildAccountUsageRefreshKey({ ...base, updated_at: 'new', current_concurrency: 5, extra: { other: 2, session_window_utilization: 0 } } as any)).toBe(buildAccountUsageRefreshKey(base))
    expect(buildAccountUsageRefreshKey({ ...base, last_used_at: 'new' })).not.toBe(buildAccountUsageRefreshKey(base))
  })

  it('tracks the Gemini quota tier and ignores other credential keys', () => {
    const base = { id: 1, platform: 'gemini', type: 'oauth', credentials: { oauth_type: 'google_one', tier_id: 'google_ai_pro' } } as any
    expect(buildAccountUsageRefreshKey({ ...base, credentials: { ...base.credentials, tier_id: 'google_ai_ultra' } })).not.toBe(buildAccountUsageRefreshKey(base))
    expect(buildAccountUsageRefreshKey({ ...base, credentials: { ...base.credentials, oauth_type: 'ai_studio' } })).not.toBe(buildAccountUsageRefreshKey(base))
    const legacy = { id: 1, platform: 'gemini', type: 'oauth', credentials: {} } as any
    expect(buildAccountUsageRefreshKey({ ...legacy, credentials: { project_id: 'p-1' } })).not.toBe(buildAccountUsageRefreshKey(legacy))
    expect(buildAccountUsageRefreshKey({ ...base, credentials: { ...base.credentials, client_id: 'other', expires_at: '999' } })).toBe(buildAccountUsageRefreshKey(base))
    const apiKey = { id: 1, platform: 'gemini', type: 'apikey', credentials: { base_url: 'https://relay.example' } } as any
    expect(buildAccountUsageRefreshKey({ ...apiKey, credentials: { ...apiKey.credentials, base_url: 'https://generativelanguage.googleapis.com' } })).not.toBe(buildAccountUsageRefreshKey(apiKey))
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
