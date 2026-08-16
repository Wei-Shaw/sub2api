import type {
  Account,
  AccountQuotaWindowKey,
  AccountUsageInfo,
  AccountWindowBoundaryStatus,
  AccountWindowUsageForecast,
  AccountWindowUsageItem,
  AccountWindowUsageResponse,
  AccountWindowUsageTarget,
  UsageProgress
} from '@/types'

const MINUTE_MS = 60_000
const DAY_MS = 24 * 60 * MINUTE_MS
const MAX_RANGE_MS = 32 * DAY_MS
const SNAPSHOT_MAX_AGE_MS = 5 * MINUTE_MS
const BOUNDARY_MATCH_TOLERANCE_MS = 2 * MINUTE_MS

export interface AccountQuotaWindowRange {
  startTime: string
  endTime: string
}

export interface AccountQuotaWindowModel {
  key: AccountQuotaWindowKey
  utilization: number | null
  resetAt: string | null
  boundaryStatus: AccountWindowBoundaryStatus
  currentRange: AccountQuotaWindowRange | null
  previousRange: AccountQuotaWindowRange | null
  current: AccountWindowUsageItem | null
  previous: AccountWindowUsageItem | null
  forecast: AccountWindowUsageForecast | null
}

interface WindowCandidate {
  key: AccountQuotaWindowKey
  progress: UsageProgress | null
  utilization: number | null
  boundaryKind: 'five_hour' | 'seven_day' | 'monthly'
}

interface ResolvedBoundary {
  status: AccountWindowBoundaryStatus
  startMs?: number
  endMs?: number
}

export function isAccountUsageSnapshotFresh(
  usage: AccountUsageInfo | null | undefined,
  now: Date = new Date()
): boolean {
  if (!usage?.updated_at) return true
  const observedAt = Date.parse(usage.updated_at)
  if (!Number.isFinite(observedAt)) return false
  const age = now.getTime() - observedAt
  return age >= -MINUTE_MS && age <= SNAPSHOT_MAX_AGE_MS
}

export function buildAccountQuotaWindows(
  account: Account,
  usage: AccountUsageInfo,
  now: Date = new Date()
): AccountQuotaWindowModel[] {
  const candidates = collectCandidates(account, usage)
  return candidates.map((candidate) => {
    const boundary = resolveBoundary(account, usage, candidate, now)
    if (boundary.status !== 'ready' || boundary.startMs === undefined || boundary.endMs === undefined) {
      return emptyWindow(candidate, boundary.status)
    }

    const currentEndMs = Math.min(now.getTime(), boundary.endMs)
    const durationMs = boundary.endMs - boundary.startMs
    if (currentEndMs <= boundary.startMs || durationMs <= 0 || durationMs > MAX_RANGE_MS) {
      return emptyWindow(candidate, 'inconsistent_boundary')
    }

    return {
      key: candidate.key,
      utilization: candidate.utilization,
      resetAt: new Date(boundary.endMs).toISOString(),
      boundaryStatus: 'ready',
      currentRange: toRange(boundary.startMs, currentEndMs),
      previousRange: toRange(boundary.startMs - durationMs, boundary.startMs),
      current: null,
      previous: null,
      forecast: null
    }
  })
}

export function buildAccountWindowUsageTargets(
  windows: AccountQuotaWindowModel[]
): AccountWindowUsageTarget[] {
  const targets: AccountWindowUsageTarget[] = []
  for (const window of windows) {
    if (window.boundaryStatus !== 'ready' || !window.currentRange || !window.previousRange) continue
    targets.push({
      window_key: window.key,
      period: 'current',
      start_time: window.currentRange.startTime,
      end_time: window.currentRange.endTime
    })
    targets.push({
      window_key: window.key,
      period: 'previous',
      start_time: window.previousRange.startTime,
      end_time: window.previousRange.endTime
    })
  }
  return targets
}

export function applyAccountWindowUsageResponse(
  windows: AccountQuotaWindowModel[],
  response: AccountWindowUsageResponse
): AccountQuotaWindowModel[] {
  const items = new Map(response.items.map((item) => [`${item.window_key}:${item.period}`, item]))
  return windows.map((window) => {
    const current = items.get(`${window.key}:current`) ?? null
    const previous = items.get(`${window.key}:previous`) ?? null
    return {
      ...window,
      current,
      previous,
      forecast: estimateAccountWindowUsage(current, previous, window.utilization)
    }
  })
}

export function estimateAccountWindowUsage(
  current: AccountWindowUsageItem | null | undefined,
  previous: AccountWindowUsageItem | null | undefined,
  usedPercent: number | null | undefined
): AccountWindowUsageForecast | null {
  const currentMetrics = usableMetrics(current)
  if (
    current?.matched &&
    currentMetrics &&
    isNonZero(currentMetrics) &&
    typeof usedPercent === 'number' &&
    Number.isFinite(usedPercent) &&
    usedPercent > 0 &&
    usedPercent <= 100
  ) {
    const multiplier = 100 / usedPercent
    const requests = Math.round(currentMetrics.total_requests * multiplier)
    const tokens = Math.round(currentMetrics.total_tokens * multiplier)
    const cost = roundCurrency(currentMetrics.account_cost * multiplier)
    if ([requests, tokens, cost].every(Number.isFinite)) {
      return {
        total_requests: Math.max(currentMetrics.total_requests, requests),
        total_tokens: Math.max(currentMetrics.total_tokens, tokens),
        account_cost: Math.max(currentMetrics.account_cost, cost),
        basis: 'quota'
      }
    }
  }

  const previousMetrics = usableMetrics(previous)
  if (previous?.matched && previousMetrics && isNonZero(previousMetrics)) {
    return { ...previousMetrics, account_cost: roundCurrency(previousMetrics.account_cost), basis: 'previous' }
  }
  return null
}

function collectCandidates(account: Account, usage: AccountUsageInfo): WindowCandidate[] {
  const byKey = new Map<AccountQuotaWindowKey, WindowCandidate>()
  if (usage.five_hour || (account.session_window_start && account.session_window_end)) {
    byKey.set('five_hour', candidate('five_hour', usage.five_hour, 'five_hour'))
  }

  if (usage.seven_day) {
    const key = isMonthlyDuration(usage.seven_day.window_minutes) ? 'thirty_day' : 'seven_day'
    byKey.set(key, candidate(key, usage.seven_day, key === 'thirty_day' ? 'monthly' : 'seven_day'))
  } else if (hasActiveGrokWeeklyBoundary(account, usage)) {
    byKey.set('seven_day', candidate('seven_day', null, 'seven_day', grokWeeklyUtilization(usage)))
  }

  if (usage.thirty_day || hasGrokMonthlyBoundary(account, usage)) {
    byKey.set(
      'thirty_day',
      candidate('thirty_day', usage.thirty_day ?? null, 'monthly', grokMonthlyUtilization(usage))
    )
  }

  return ['five_hour', 'seven_day', 'thirty_day']
    .map((key) => byKey.get(key as AccountQuotaWindowKey))
    .filter((value): value is WindowCandidate => Boolean(value))
}

function candidate(
  key: AccountQuotaWindowKey,
  progress: UsageProgress | null | undefined,
  boundaryKind: WindowCandidate['boundaryKind'],
  fallbackUtilization?: number | null
): WindowCandidate {
  return {
    key,
    progress: progress ?? null,
    utilization: finiteNonNegative(progress?.utilization) ?? finiteNonNegative(fallbackUtilization),
    boundaryKind
  }
}

function resolveBoundary(
  account: Account,
  usage: AccountUsageInfo,
  candidate: WindowCandidate,
  now: Date
): ResolvedBoundary {
  if (!isAccountUsageSnapshotFresh(usage, now)) return { status: 'stale_snapshot' }

  if (account.platform === 'grok') {
    const explicit = candidate.boundaryKind === 'seven_day' && usage.grok_billing?.period_type === 'weekly'
      ? parseActiveRange(usage.grok_billing?.period_start, usage.grok_billing?.period_end, now)
      : candidate.boundaryKind === 'monthly'
        ? parseActiveRange(usage.grok_billing?.billing_period_start, usage.grok_billing?.billing_period_end, now)
        : null
    if (explicit) return explicit
  }

  const resetMs = parseTimestamp(candidate.progress?.resets_at)
  if (candidate.boundaryKind === 'five_hour') {
    const session = parseActiveRange(account.session_window_start, account.session_window_end, now)
    if (session?.status === 'ready' && session.endMs !== undefined) {
      if (resetMs === null || Math.abs(session.endMs - resetMs) <= BOUNDARY_MATCH_TOLERANCE_MS) {
        return session
      }
    }
    if (resetMs === null && session) return session
  }

  if (resetMs === null) return { status: 'missing_boundary' }
  if (resetMs <= now.getTime()) return { status: 'expired_boundary' }

  const rawDurationMinutes = candidate.progress?.window_minutes
  let durationMinutes = finitePositive(candidate.progress?.window_minutes)
  if (rawDurationMinutes !== null && rawDurationMinutes !== undefined && !durationMinutes) {
    return { status: 'inconsistent_boundary' }
  }
  if (!durationMinutes && candidate.boundaryKind === 'five_hour') durationMinutes = 300
  if (!durationMinutes && candidate.boundaryKind === 'seven_day') durationMinutes = 7 * 24 * 60
  if (!durationMinutes) return { status: 'missing_boundary' }

  const durationMs = durationMinutes * MINUTE_MS
  if (!Number.isFinite(durationMs) || durationMs <= 0 || durationMs > MAX_RANGE_MS) {
    return { status: 'inconsistent_boundary' }
  }
  const startMs = resetMs - durationMs
  if (startMs >= now.getTime()) return { status: 'inconsistent_boundary' }
  return { status: 'ready', startMs, endMs: resetMs }
}

function parseActiveRange(
  startRaw: string | null | undefined,
  endRaw: string | null | undefined,
  now: Date
): ResolvedBoundary | null {
  if (!startRaw || !endRaw) return null
  const startMs = parseTimestamp(startRaw)
  const endMs = parseTimestamp(endRaw)
  if (startMs === null || endMs === null || startMs >= endMs || endMs - startMs > MAX_RANGE_MS) {
    return { status: 'inconsistent_boundary' }
  }
  if (endMs <= now.getTime()) return { status: 'expired_boundary' }
  if (startMs >= now.getTime()) return { status: 'inconsistent_boundary' }
  return { status: 'ready', startMs, endMs }
}

function emptyWindow(
  candidate: WindowCandidate,
  status: AccountWindowBoundaryStatus
): AccountQuotaWindowModel {
  return {
    key: candidate.key,
    utilization: candidate.utilization,
    resetAt: candidate.progress?.resets_at ?? null,
    boundaryStatus: status,
    currentRange: null,
    previousRange: null,
    current: null,
    previous: null,
    forecast: null
  }
}

function toRange(startMs: number, endMs: number): AccountQuotaWindowRange {
  return { startTime: new Date(startMs).toISOString(), endTime: new Date(endMs).toISOString() }
}

function usableMetrics(item: AccountWindowUsageItem | null | undefined) {
  if (!item) return null
  return {
    total_requests: finiteNonNegative(item.total_requests) ?? 0,
    total_tokens: finiteNonNegative(item.total_tokens) ?? 0,
    account_cost: finiteNonNegative(item.account_cost) ?? 0
  }
}

function isNonZero(metrics: { total_requests: number; total_tokens: number; account_cost: number }) {
  return metrics.total_requests > 0 || metrics.total_tokens > 0 || metrics.account_cost > 0
}

function finiteNonNegative(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

function finitePositive(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

function parseTimestamp(value: string | null | undefined): number | null {
  if (!value) return null
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : null
}

function roundCurrency(value: number): number {
  return Math.round((value + Number.EPSILON) * 100) / 100
}

function isMonthlyDuration(windowMinutes: number | null | undefined): boolean {
  return typeof windowMinutes === 'number' && Number.isFinite(windowMinutes) && windowMinutes >= 28 * 24 * 60
}

function hasActiveGrokWeeklyBoundary(account: Account, usage: AccountUsageInfo): boolean {
  return account.platform === 'grok' && usage.grok_billing?.period_type === 'weekly' &&
    Boolean(usage.grok_billing.period_start && usage.grok_billing.period_end)
}

function hasGrokMonthlyBoundary(account: Account, usage: AccountUsageInfo): boolean {
  return account.platform === 'grok' &&
    Boolean(usage.grok_billing?.billing_period_start && usage.grok_billing?.billing_period_end)
}

function grokWeeklyUtilization(usage: AccountUsageInfo): number | null {
  return finiteNonNegative(usage.grok_billing?.usage_percent)
}

function grokMonthlyUtilization(usage: AccountUsageInfo): number | null {
  return finiteNonNegative(usage.grok_billing?.used_percent ?? usage.grok_billing?.usage_percent)
}
