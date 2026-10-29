import { i18n } from '@/i18n'
import type { AccountListItem, AccountUsageInfo } from '@/types'
import { platformLabel } from '@/utils/platformColors'

export interface PoolWindow {
  id: string
  remaining: number | null
  resets_at?: string
  display?: string
}
export function sortQuotaWindows<T extends { id: string }>(windows: readonly T[]): T[] {
  const order = [
    'minute',
    '5h',
    'daily',
    'weekly',
    'monthly',
    'rolling',
    'current',
    'total',
    'balance'
  ]
  return [...windows].sort(
    (a, b) =>
      (order.indexOf(a.id) < 0 ? 99 : order.indexOf(a.id)) -
      (order.indexOf(b.id) < 0 ? 99 : order.indexOf(b.id))
  )
}
export interface QuotaPool {
  id: string
  name: string
  windows: PoolWindow[]
}
/** Presentation is driven by reported pools/windows, never by model-name guesses. */
const windowLabels: Record<string, string> = {
  minute: '1m',
  daily: '1d',
  weekly: '7d',
  monthly: '1mo'
}
export const quotaWindowLabel = (id: string) =>
  windowLabels[id] ??
  (['current', 'rolling', 'total', 'balance'].includes(id)
    ? i18n.global.t(`admin.quota.windows.${id}`)
    : id)
export function quotaPoolGroups<T>(pools: readonly T[], columns = 2): T[][] {
  const size = Math.max(1, columns)
  return Array.from({ length: Math.ceil(pools.length / size) }, (_, i) =>
    pools.slice(i * size, (i + 1) * size)
  )
}
// Normalize existing provider responses at the UI boundary. No additional requests.
export function usagePools(platform: string, usage?: AccountUsageInfo): QuotaPool[] {
  if (usage?.quota_pools?.length) return usage.quota_pools
  if (!usage) return []

  const window = (id: string, value: AccountUsageInfo['five_hour'] | undefined): PoolWindow[] => {
    if (!value) return []
    return [quotaWindow(id, value.utilization, value.resets_at)]
  }
  if (platform === 'openai' || platform === 'anthropic') {
    const pools: QuotaPool[] = [
      {
        id: 'shared',
        name: platform === 'openai' ? 'GPT / Codex' : 'Claude',
        windows: [
          ...window('5h', usage.five_hour),
          ...window('weekly', usage.seven_day),
          ...window('monthly', usage.thirty_day)
        ]
      }
    ]
    if (platform === 'anthropic') {
      pools.push({
        id: 'sonnet',
        name: 'Sonnet',
        windows: window('weekly', usage.seven_day_sonnet)
      })
      pools.push({ id: 'fable', name: 'Fable', windows: window('weekly', usage.seven_day_fable) })
    }
    return pools.filter((pool) => pool.windows.length)
  }
  if (platform === 'gemini') {
    // Google One / Code Assist share one pool; do not count the same capacity per model.
    if (usage.gemini_shared_minute || usage.gemini_shared_daily)
      return [
        {
          id: 'shared',
          name: 'Gemini',
          windows: [
            ...window('minute', usage.gemini_shared_minute),
            ...window('daily', usage.gemini_shared_daily)
          ]
        }
      ]
    return [
      {
        id: 'pro',
        name: 'Pro',
        windows: [
          ...window('minute', usage.gemini_pro_minute),
          ...window('daily', usage.gemini_pro_daily)
        ]
      },
      {
        id: 'flash',
        name: 'Flash',
        windows: [
          ...window('minute', usage.gemini_flash_minute),
          ...window('daily', usage.gemini_flash_daily)
        ]
      }
    ].filter((pool) => pool.windows.length)
  }
  if (platform === 'grok') return grokQuotaView(usage).pools
  return []
}
/** Convert a known used percentage; null is unknown, never unlimited. */
export function quotaWindow(
  id: string,
  used: number | null | undefined,
  reset?: string | null
): PoolWindow {
  return {
    id,
    remaining:
      typeof used === 'number' && Number.isFinite(used)
        ? Math.max(0, Math.min(1, 1 - used / 100))
        : null,
    ...(reset ? { resets_at: reset } : {})
  }
}
export function tierQuotaPools(
  name: string,
  tiers: readonly { window: string; used_percent: number; reset_at?: string }[]
): QuotaPool[] {
  return tiers.length
    ? [
        {
          id: 'subscription',
          name,
          windows: tiers.map((tier) => quotaWindow(tier.window, tier.used_percent, tier.reset_at))
        }
      ]
    : []
}
export function snapshotQuotaPools(account: AccountListItem): QuotaPool[] {
  const ollama = account.ollama_cloud_usage
  if (ollama?.eligible) {
    const data = ollama.snapshot?.data
    const windows: PoolWindow[] = []
    if (data?.five_hour)
      windows.push(quotaWindow('5h', data.five_hour.used_percent, data.five_hour.reset_at))
    if (data?.seven_day)
      windows.push(quotaWindow('weekly', data.seven_day.used_percent, data.seven_day.reset_at))
    return windows.length ? [{ id: 'ollama', name: 'Ollama Cloud', windows }] : []
  }
  const opencode = account.opencode_go_usage
  if (opencode?.eligible) {
    const data = opencode.snapshot?.data
    const windows: PoolWindow[] = []
    if (data?.rolling) windows.push(quotaWindow('5h', data.rolling.percent, data.rolling.resets_at))
    if (data?.weekly)
      windows.push(quotaWindow('weekly', data.weekly.percent, data.weekly.resets_at))
    if (data?.monthly)
      windows.push(quotaWindow('monthly', data.monthly.percent, data.monthly.resets_at))
    return windows.length ? [{ id: 'opencode-go', name: 'OpenCode Go', windows }] : []
  }
  const extra = account.extra ?? {}
  const tiers = ['5h', 'weekly', 'monthly'].flatMap((window) => {
    const used = extra[`${account.platform}_${window}_used_percent`]
    const reset = extra[`${account.platform}_${window}_reset_at`]
    return typeof used === 'number'
      ? [{ window, used_percent: used, reset_at: typeof reset === 'string' ? reset : undefined }]
      : []
  })
  return tierQuotaPools(platformLabel(account.platform), tiers)
}
export function configuredQuotaPools(account: AccountListItem): QuotaPool[] {
  const windows: PoolWindow[] = []
  const extra = account.extra ?? {}
  for (const [id, period] of [
    ['daily', 1],
    ['weekly', 7]
  ] as const) {
    const limit = account[id === 'daily' ? 'quota_daily_limit' : 'quota_weekly_limit'] ?? 0
    if (limit <= 0) continue
    const used = account[id === 'daily' ? 'quota_daily_used' : 'quota_weekly_used'] ?? 0
    let reset: string | undefined
    if (extra[`quota_${id}_reset_mode`] === 'fixed')
      reset = extra[`quota_${id}_reset_at`] as string | undefined
    else if (typeof extra[`quota_${id}_start`] === 'string') {
      const time = Date.parse(extra[`quota_${id}_start`] as string)
      if (Number.isFinite(time)) reset = new Date(time + period * 86400000).toISOString()
    }
    windows.push(quotaWindow(id, (used / limit) * 100, reset))
  }
  if ((account.quota_limit ?? 0) > 0)
    windows.push(quotaWindow('total', ((account.quota_used ?? 0) / account.quota_limit!) * 100))
  return windows.length
    ? [{ id: 'local-budget', name: i18n.global.t('admin.quota.localBudget'), windows }]
    : []
}
export function balanceQuotaPools(
  entries: readonly { currency: string; balance: number }[]
): QuotaPool[] {
  return entries.map((entry) => ({
    id: 'balance:' + entry.currency,
    name: entry.currency || i18n.global.t('admin.quota.balance'),
    windows: [
      {
        id: 'balance',
        remaining: null,
        display: entry.balance.toFixed(2)
      }
    ]
  }))
}
export function snapshotBalancePools(account: AccountListItem): QuotaPool[] {
  const extra = account.extra ?? {}
  const multi = extra[`${account.platform}_balances`]
  if (Array.isArray(multi))
    return balanceQuotaPools(
      multi.filter(
        (entry): entry is { currency: string; balance: number } =>
          !!entry && typeof entry.currency === 'string' && typeof entry.balance === 'number'
      )
    )
  const balance = extra[`${account.platform}_balance`]
  return typeof balance === 'number'
    ? balanceQuotaPools([
        { currency: String(extra[`${account.platform}_balance_currency`] || ''), balance }
      ])
    : []
}
export function grokQuotaView(usage?: AccountUsageInfo) {
  const billing = usage?.grok_billing
  const free = (value: string) => value.includes('free') || value.includes('basic')
  const paid = (value: string) => !!value && !free(value) && !value.includes('unknown')
  const plan = (billing?.plan || '').trim().toLowerCase()
  const tier = (usage?.subscription_tier || '').trim().toLowerCase()
  const entitlement = (usage?.grok_entitlement_status || '').toLowerCase()
  const isFree = (() => {
    if (free(tier)) return true
    if (paid(tier)) return false
    if (
      billing?.usage_percent != null ||
      billing?.used_percent != null ||
      (billing?.monthly_limit_cents ?? 0) > 0
    )
      return false
    if (paid(plan)) return false
    return free(plan) || free(entitlement) || billing != null
  })()
  const weekly =
    billing?.period_type?.toLowerCase() === 'weekly' && billing.usage_percent != null
      ? {
          utilization: Math.min(100, Math.max(0, billing.usage_percent)),
          resetsAt: billing.period_end || null,
          windowStats: usage?.grok_local_usage_7d || usage?.seven_day?.window_stats || null
        }
      : null
  const monthlyUsed =
    billing?.used_percent != null && Number.isFinite(billing.used_percent)
      ? billing.used_percent
      : (billing?.monthly_limit_cents ?? 0) > 0 && billing?.used_cents != null
        ? (billing.used_cents / billing.monthly_limit_cents!) * 100
        : null
  const monthly =
    monthlyUsed != null &&
    !(billing?.period_type?.toLowerCase() === 'weekly' && billing.monthly_limit_cents == null)
      ? {
          utilization: Math.min(100, Math.max(0, monthlyUsed)),
          resetsAt: billing?.billing_period_end || billing?.period_end || null,
          windowStats: usage?.grok_local_usage_monthly || usage?.thirty_day?.window_stats || null
        }
      : null
  const limit = usage?.grok_free_token_limit
  const freeBar =
    isFree && usage?.grok_local_usage_24h && typeof limit === 'number' && limit > 0
      ? {
          utilization: Math.min(
            100,
            (Math.max(0, usage.grok_local_usage_24h.tokens || 0) / limit) * 100
          ),
          limit
        }
      : null
  const windows = isFree
    ? freeBar
      ? [quotaWindow('24h', freeBar.utilization)]
      : []
    : [
        ...(weekly ? [quotaWindow('weekly', weekly.utilization, weekly.resetsAt)] : []),
        ...(monthly ? [quotaWindow('monthly', monthly.utilization, monthly.resetsAt)] : [])
      ]
  const pools: QuotaPool[] = windows.length
    ? [
        {
          id: isFree ? 'grok-soft-token' : 'grok-billing',
          name: isFree ? i18n.global.t('admin.quota.grokTokens') : 'Grok',
          windows
        }
      ]
    : []
  for (const [id, name, quota] of [
    ['grok-requests', i18n.global.t('admin.quota.grokRequests'), usage?.grok_request_quota],
    ['grok-tokens', i18n.global.t('admin.quota.grokTokens'), usage?.grok_token_quota]
  ] as const) {
    if (!quota) continue
    const used =
      quota.remaining != null && (quota.limit ?? 0) > 0
        ? (1 - quota.remaining / quota.limit!) * 100
        : null
    const reset =
      quota.reset_at ||
      (quota.reset_unix ? new Date(quota.reset_unix * 1000).toISOString() : undefined)
    const window = quotaWindow('current', used, reset)
    if (used == null && quota.remaining != null)
      window.display = i18n.global.t('admin.quota.remainingValue', { value: quota.remaining })
    pools.push({ id, name, windows: [window] })
  }
  return { isFree, weekly, monthly, freeBar, pools }
}
export interface PoolRow extends QuotaPool {
  identity: string
  accountId: number
  accountName: string
  platform: string
  tier: string
  eligible: boolean
  updatedAt: string | null
  source: string
  stale: boolean
}
export function accountPools(
  account: AccountListItem,
  usage?: AccountUsageInfo,
  now = Date.now()
): PoolRow[] {
  const stamp =
    usage?.updated_at ||
    account.ollama_cloud_usage?.snapshot?.fetched_at ||
    account.opencode_go_usage?.snapshot?.fetched_at ||
    String(account.extra?.[`${account.platform}_usage_updated_at`] || '') ||
    null
  const stale =
    !stamp ||
    !Number.isFinite(Date.parse(stamp)) ||
    now - Date.parse(stamp) > 30 * 60 * 1000 ||
    Date.parse(stamp) > now + 60000
  const reported = usagePools(account.platform, usage)
  const pools = [
    ...(reported.length ? reported : snapshotQuotaPools(account)),
    ...configuredQuotaPools(account),
    ...snapshotBalancePools(account)
  ]
  return pools.map((p) => ({
    ...p,
    identity: String(account.credentials?.email || account.credentials?.account_id || account.id)
      .trim()
      .toLowerCase(),
    accountId: account.id,
    accountName: account.name,
    platform: account.platform,
    tier:
      usage?.subscription_tier ||
      String(account.credentials?.plan_type || account.credentials?.tier_id || 'unknown'),
    eligible: account.status === 'active' && account.schedulable !== false,
    updatedAt: p.id === 'local-budget' ? new Date(now).toISOString() : stamp,
    source: usage?.source || (account.platform === 'gemini' ? 'local-estimate' : 'quota-snapshot'),
    stale: p.id === 'local-budget' ? false : stale
  }))
}
export function aggregatePools(rows: PoolRow[], now = Date.now()) {
  const groups = new Map<
    string,
    {
      name: string
      platform: string
      tier: string
      windows: Map<
        string,
        { id: string; total: number; known: number; accounts: number; nextReset?: string }
      >
    }
  >()
  const seen = new Set<string>()
  for (const row of rows.filter((r) => r.eligible && r.windows.some((w) => !w.display))) {
    const identity = [row.platform, row.identity, row.id].join(':')
    if (seen.has(identity)) continue
    seen.add(identity)
    // Unknown plans are deliberately separate; capacity equivalence is not established.
    const key = [row.platform, row.id, row.tier, row.tier === 'unknown' ? row.accountId : ''].join(
      ':'
    )
    if (!groups.has(key))
      groups.set(key, {
        name: row.name,
        platform: row.platform,
        tier: row.tier,
        windows: new Map()
      })
    const group = groups.get(key)!
    for (const w of row.windows) {
      if (!group.windows.has(w.id))
        group.windows.set(w.id, { id: w.id, total: 0, known: 0, accounts: 0 })
      const sum = group.windows.get(w.id)!
      sum.accounts++
      const validReset =
        !w.resets_at || (Number.isFinite(Date.parse(w.resets_at)) && Date.parse(w.resets_at) > now)
      if (
        !row.stale &&
        validReset &&
        w.remaining !== null &&
        Number.isFinite(w.remaining) &&
        w.remaining >= 0 &&
        w.remaining <= 1
      ) {
        sum.total += w.remaining
        sum.known++
        if (w.resets_at && (!sum.nextReset || Date.parse(w.resets_at) < Date.parse(sum.nextReset)))
          sum.nextReset = w.resets_at
      }
    }
  }
  return [...groups.entries()].map(([key, g]) => ({ key, ...g, windows: [...g.windows.values()] }))
}
