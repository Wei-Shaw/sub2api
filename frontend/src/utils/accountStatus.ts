import type { Account } from '@/types'

type RateLimitAccount = Pick<
  Account,
  'platform' | 'type' | 'grok_free_recovery_pending' | 'rate_limit_reset_at' | 'extra'
>

export const isGrokFreeRecoveryPending = (
  account: RateLimitAccount | null | undefined
): boolean => {
  return account?.platform === 'grok' &&
    account.type === 'oauth' &&
    account.grok_free_recovery_pending === true
}

export const isAccountRateLimited = (
  account: RateLimitAccount | null | undefined,
  now = Date.now()
): boolean => {
  if (!account) return false
  if (isGrokFreeRecoveryPending(account)) return true
  if (grokExtraQuotaSnapshotBlocksScheduling(account, now)) return true
  if (!account.rate_limit_reset_at) return false

  const resetAt = Date.parse(account.rate_limit_reset_at)
  return Number.isFinite(resetAt) && resetAt > now
}

const grokFreeUsageExhaustedCodes = new Set([
  'subscription:free-usage-exhausted',
  'subscription-free-usage-exhausted'
])

const grokExtraQuotaSnapshotBlocksScheduling = (
  account: RateLimitAccount,
  now: number
): boolean => {
  if (account.platform !== 'grok' || account.type !== 'oauth') return false
  const extra = account.extra as Record<string, unknown> | undefined
  const snapshot = extra?.grok_usage_snapshot
  if (!snapshot || typeof snapshot !== 'object') return false
  const snap = snapshot as {
    provider_error_code?: string
    tokens?: { remaining?: number | null; reset_at?: string | null }
    requests?: { remaining?: number | null; reset_at?: string | null }
  }
  const code = String(snap.provider_error_code ?? '').trim().toLowerCase()
  if (grokFreeUsageExhaustedCodes.has(code)) return true
  const remainingZero = (window?: { remaining?: number | null; reset_at?: string | null }) =>
    window != null && typeof window.remaining === 'number' && window.remaining <= 0
  if (!remainingZero(snap.tokens) && !remainingZero(snap.requests)) return false
  for (const window of [snap.tokens, snap.requests]) {
    const resetRaw = window?.reset_at
    if (!resetRaw) continue
    const resetAt = Date.parse(resetRaw)
    if (Number.isFinite(resetAt) && resetAt > now) return true
  }
  return true
}
