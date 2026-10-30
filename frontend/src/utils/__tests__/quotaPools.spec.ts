import { i18n } from '@/i18n'
import quotaZh from '@/i18n/locales/zh/admin/quota'
i18n.global.setLocaleMessage('zh', { admin: { quota: quotaZh } })
i18n.global.locale.value = 'zh'
import { describe, expect, it } from 'vitest'
import {
  accountPools,
  aggregatePools,
  usagePools,
  quotaPoolGroups,
  quotaWindowLabel,
  snapshotQuotaPools,
  configuredQuotaPools,
  balanceQuotaPools
} from '../quotaPools'
import type { AccountListItem, AccountUsageInfo } from '@/types'
const now = Date.parse('2026-09-28T12:00:00Z')
const account = (id = 1) =>
  ({
    id,
    name: 'test',
    platform: 'antigravity',
    type: 'oauth',
    status: 'active',
    schedulable: true,
    credentials: { email: `${id}@example.com` }
  }) as AccountListItem
const usage = (remaining = 0.8) =>
  ({
    source: 'antigravity-oauth',
    updated_at: new Date(now).toISOString(),
    subscription_tier: 'PRO',
    quota_pools: [
      {
        id: 'gemini',
        name: 'Gemini',
        windows: [
          { id: '5h', remaining, resets_at: '2026-09-28T14:00:00Z' },
          { id: 'weekly', remaining: 0.6 }
        ]
      }
    ]
  }) as AccountUsageInfo
describe('shared quota pools', () => {
  it('sums accounts once per upstream pool', () => {
    const rows = [
      ...accountPools(account(1), usage(), now),
      ...accountPools(account(2), usage(0.5), now)
    ]
    expect(aggregatePools(rows, now)[0].windows[0]).toEqual({
      id: '5h',
      total: 1.3,
      known: 2,
      accounts: 2,
      nextReset: '2026-09-28T14:00:00Z'
    })
  })
  it('does not count duplicate account registrations', () => {
    const rows = [
      ...accountPools(account(), usage(), now),
      ...accountPools({ ...account(), id: 3 }, usage(), now)
    ]
    expect(aggregatePools(rows, now)[0].windows[0].accounts).toBe(1)
  })
  it('does not infer weekly capacity from model percentages', () => {
    const u = {
      ...usage(),
      quota_pools: undefined,
      antigravity_quota: { 'gemini-3-flash': { utilization: 0, reset_time: '' } }
    }
    expect(accountPools(account(), u, now)).toEqual([])
  })
  it('excludes stale, expired, invalid and disabled capacity', () => {
    const stale = accountPools(account(), { ...usage(), updated_at: '2026-09-28T10:00:00Z' }, now)
    expect(aggregatePools(stale, now)[0].windows[0].known).toBe(0)
    const expired = accountPools(account(), usage(), now)
    expired[0].windows[0].resets_at = '2026-09-28T11:00:00Z'
    expect(aggregatePools(expired, now)[0].windows[0].known).toBe(0)
    expect(
      aggregatePools(accountPools({ ...account(), schedulable: false }, usage(), now), now)
    ).toEqual([])
  })
  it('keeps different plans and channels separate', () => {
    const rows = [
      ...accountPools(account(), usage(), now),
      ...accountPools(account(2), { ...usage(), subscription_tier: 'ULTRA' }, now)
    ]
    expect(aggregatePools(rows, now)).toHaveLength(2)
  })
})

const report = (value: object) => value as AccountUsageInfo
describe('adaptive quota adapters', () => {
  it('uses authoritative pools on any platform without guessing model names', () => {
    const data = usage()
    expect(usagePools('new-provider', data)).toBe(data.quota_pools)
  })
  it('Codex can have only weekly or monthly limits; no invented 5h window', () => {
    const pools = usagePools(
      'openai',
      report({ seven_day: { utilization: 20 }, thirty_day: { utilization: 75 } })
    )
    expect(pools[0].windows).toEqual([
      { id: 'weekly', remaining: 0.8 },
      { id: 'monthly', remaining: 0.25 }
    ])
    expect(quotaWindowLabel('monthly')).toBe('1mo')
  })
  it('keeps Claude shared, Sonnet and Fable limits distinct', () => {
    const pools = usagePools(
      'anthropic',
      report({
        five_hour: { utilization: 100 },
        seven_day_sonnet: { utilization: 0 },
        seven_day_fable: { utilization: 50 }
      })
    )
    expect(pools.map((p) => p.id)).toEqual(['shared', 'sonnet', 'fable'])
    expect(pools.map((p) => p.windows[0].remaining)).toEqual([0, 1, 0.5])
  })
  it('keeps unavailable data unknown and clamps reported percentages', () => {
    expect(usagePools('openai')).toEqual([])
    expect(
      usagePools('openai', report({ five_hour: {}, seven_day: { utilization: 120 } }))[0].windows
    ).toEqual([
      { id: '5h', remaining: null },
      { id: 'weekly', remaining: 0 }
    ])
  })
  it('Gemini shared limits take precedence over model breakdowns', () => {
    const pools = usagePools(
      'gemini',
      report({ gemini_shared_daily: { utilization: 40 }, gemini_pro_daily: { utilization: 40 } })
    )
    expect(pools).toHaveLength(1)
    expect(pools[0].windows).toEqual([{ id: 'daily', remaining: 0.6 }])
  })
  it('native Gemini supports minute/day per model', () => {
    const pools = usagePools(
      'gemini',
      report({
        gemini_pro_minute: { utilization: 20 },
        gemini_pro_daily: { utilization: 50 },
        gemini_flash_daily: { utilization: 0 }
      })
    )
    expect(pools.map((p) => p.id)).toEqual(['pro', 'flash'])
    expect(pools[0].windows.map((w) => w.id)).toEqual(['minute', 'daily'])
  })
  it('groups arbitrary pool counts into two readable columns', () => {
    expect(quotaPoolGroups([1, 2, 3, 4, 5])).toEqual([[1, 2], [3, 4], [5]])
    expect(quotaPoolGroups([])).toEqual([])
  })
})

describe('all account quota sources', () => {
  it.each([
    'kimi',
    'zhipu',
    'deepseek',
    'minimax',
    'opencode_go',
    'command_code',
    'typesafe',
    'cline',
    'future-provider'
  ])('adapts %s subscription snapshots by available windows', (platform) => {
    const a = { ...account(), platform, extra: { [platform + '_monthly_used_percent']: 25 } }
    expect(snapshotQuotaPools(a)[0].windows).toEqual([{ id: 'monthly', remaining: 0.75 }])
  })
  it('prefers embedded cloud snapshots over provider-name guesses', () => {
    const a = {
      ...account(),
      ollama_cloud_usage: {
        eligible: true,
        snapshot: { data: { seven_day: { used_percent: 80 } } }
      }
    } as AccountListItem
    expect(snapshotQuotaPools(a)[0]).toMatchObject({
      name: 'Ollama Cloud',
      windows: [{ id: 'weekly', remaining: expect.closeTo(0.2) }]
    })
  })
  it('supports fixed and rolling local budgets plus non-resetting totals', () => {
    const a = {
      ...account(),
      type: 'apikey',
      quota_daily_limit: 10,
      quota_daily_used: 3,
      quota_weekly_limit: 100,
      quota_weekly_used: 5,
      quota_limit: 200,
      quota_used: 20,
      extra: {
        quota_daily_reset_mode: 'fixed',
        quota_daily_reset_at: '2026-10-31T00:00:00Z',
        quota_weekly_start: '2026-10-29T00:00:00Z'
      }
    } as AccountListItem
    expect(configuredQuotaPools(a)[0].windows).toEqual([
      { id: 'daily', remaining: 0.7, resets_at: '2026-10-31T00:00:00Z' },
      { id: 'weekly', remaining: 0.95, resets_at: '2026-11-05T00:00:00.000Z' },
      { id: 'total', remaining: 0.9 }
    ])
  })
  it('shows multiple currency balances without manufacturing a percentage', () => {
    expect(
      balanceQuotaPools([
        { currency: 'CNY', balance: 20 },
        { currency: 'USD', balance: 3 }
      ]).map((p) => p.windows[0])
    ).toEqual([
      { id: 'balance', remaining: null, display: '20.00' },
      { id: 'balance', remaining: null, display: '3.00' }
    ])
  })
  it('keeps Grok billing windows and current header quotas separate', () => {
    const pools = usagePools(
      'grok',
      report({
        subscription_tier: 'SuperGrok',
        grok_billing: {
          period_type: 'weekly',
          usage_percent: 20,
          monthly_limit_cents: 10000,
          used_cents: 5000
        },
        grok_request_quota: { limit: 50, remaining: 10 }
      })
    )
    expect(pools[0].windows).toEqual([
      { id: 'weekly', remaining: 0.8 },
      { id: 'monthly', remaining: 0.5 }
    ])
    expect(pools[1].windows[0]).toMatchObject({ id: 'current', remaining: expect.closeTo(0.2) })
  })
  it('uses rolling 24h for Grok free soft limits, not 5h', () => {
    const pools = usagePools(
      'grok',
      report({
        subscription_tier: 'free',
        grok_free_token_limit: 1000,
        grok_local_usage_24h: { tokens: 400 }
      })
    )
    expect(pools[0].windows).toEqual([{ id: '24h', remaining: 0.6 }])
  })
})

it('finds the earliest reset only among deduplicated, counted windows', () => {
  const rows = [
    ...accountPools(account(1), usage(), now),
    ...accountPools(account(2), usage(), now),
    ...accountPools(account(3), usage(), now)
  ]
  rows[1].windows[0].resets_at = '2026-09-28T15:00:00+02:00'
  rows[2].windows[0].resets_at = '2026-09-28T12:30:00Z'
  rows[2].stale = true
  const duplicate = {
    ...rows[0],
    windows: [{ id: '5h', remaining: 0.8, resets_at: '2026-09-28T12:15:00Z' }]
  }
  const total = aggregatePools([...rows, duplicate], now)[0].windows[0]
  expect(total.known).toBe(2)
  expect(total.nextReset).toBe('2026-09-28T15:00:00+02:00')
})
