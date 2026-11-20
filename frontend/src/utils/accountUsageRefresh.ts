import type { Account } from '@/types'

const normalizeUsageRefreshValue = (value: unknown): string => {
  if (value == null) return ''
  return String(value)
}

const normalizeSnapshotRefreshValue = (value: unknown): unknown => {
  if (Array.isArray(value)) {
    return value.map(normalizeSnapshotRefreshValue)
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .filter(([, entry]) => entry !== undefined)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, entry]) => [key, normalizeSnapshotRefreshValue(entry)])
    )
  }
  return value
}

const serializeSnapshotRefreshValue = (value: unknown): string => {
  if (value == null) return ''
  return JSON.stringify(normalizeSnapshotRefreshValue(value)) ?? ''
}

const isNonBlankString = (value: unknown): value is string => (
  typeof value === 'string' && value.trim().length > 0
)

export const buildOpenAIUsageRefreshKey = (account: Pick<Account, 'id' | 'platform' | 'type' | 'updated_at' | 'last_used_at' | 'rate_limit_reset_at' | 'extra'>): string => {
  if (account.platform !== 'openai' || account.type !== 'oauth') {
    return ''
  }

  const extra = account.extra ?? {}
  return [
    account.id,
    account.updated_at,
    account.last_used_at,
    account.rate_limit_reset_at,
    extra.codex_usage_updated_at,
    extra.codex_5h_used_percent,
    extra.codex_5h_reset_at,
    extra.codex_5h_reset_after_seconds,
    extra.codex_5h_window_minutes,
    extra.codex_7d_used_percent,
    extra.codex_7d_reset_at,
    extra.codex_7d_reset_after_seconds,
    extra.codex_7d_window_minutes
  ].map(normalizeUsageRefreshValue).join('|')
}

// Inputs of the passive /usage response: session window columns, header samples
// in extra, and window stats from usage logs (signalled by last_used_at).
export const buildAnthropicUsageRefreshKey = (account: Pick<Account, 'id' | 'platform' | 'type' | 'last_used_at' | 'session_window_start' | 'session_window_end' | 'session_window_status' | 'extra'>): string => {
  if (account.platform !== 'anthropic' || (account.type !== 'oauth' && account.type !== 'setup-token')) {
    return ''
  }

  const extra = account.extra ?? {}
  return [
    account.id,
    account.last_used_at,
    account.session_window_start,
    account.session_window_end,
    account.session_window_status,
    extra.passive_usage_sampled_at,
    extra.session_window_utilization,
    extra.passive_usage_7d_utilization,
    extra.passive_usage_7d_reset,
    extra.passive_usage_7d_oi_utilization,
    extra.passive_usage_7d_oi_reset
  ].map(normalizeUsageRefreshValue).join('|')
}

// Gemini /usage counts usage logs against a tier derived from the account, so
// new use (last_used_at) or an account edit (updated_at) changes it.
export const buildGeminiUsageRefreshKey = (account: Pick<Account, 'id' | 'platform' | 'updated_at' | 'last_used_at'>): string => {
  if (account.platform !== 'gemini') {
    return ''
  }

  return [
    account.id,
    account.updated_at,
    account.last_used_at
  ].map(normalizeUsageRefreshValue).join('|')
}

export const buildGrokUsageRefreshKey = (account: Pick<Account, 'platform' | 'extra'>): string => {
  if (account.platform !== 'grok') {
    return ''
  }

  const extra = account.extra ?? {}
  const usageSnapshot = extra.grok_usage_snapshot
  const canonicalTier = (usageSnapshot as Record<string, unknown> | null | undefined)?.subscription_tier
  const legacyQuotaFallback = isNonBlankString(canonicalTier)
    ? undefined
    : extra.grok_quota_snapshot
  return [
    serializeSnapshotRefreshValue(extra.grok_billing_snapshot),
    serializeSnapshotRefreshValue(usageSnapshot),
    serializeSnapshotRefreshValue(legacyQuotaFallback)
  ].join('|')
}

export const buildAccountUsageRefreshKey = (account: Pick<Account, 'id' | 'platform' | 'type' | 'updated_at' | 'last_used_at' | 'rate_limit_reset_at' | 'session_window_start' | 'session_window_end' | 'session_window_status' | 'extra'>): string => {
  if (account.platform === 'openai') return buildOpenAIUsageRefreshKey(account)
  if (account.platform === 'anthropic') return buildAnthropicUsageRefreshKey(account)
  if (account.platform === 'gemini') return buildGeminiUsageRefreshKey(account)
  return ''
}
