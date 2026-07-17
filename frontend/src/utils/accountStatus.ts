import type { Account } from '@/types'

type RateLimitAccount = Pick<
  Account,
  'platform' | 'type' | 'grok_free_recovery_pending' | 'rate_limit_reset_at'
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
  if (!account.rate_limit_reset_at) return false

  const resetAt = Date.parse(account.rate_limit_reset_at)
  return Number.isFinite(resetAt) && resetAt > now
}
