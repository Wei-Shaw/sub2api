import type { Account } from '@/types'

export const OPENAI_SUBSCRIPTION_EXPIRY_SNAPSHOT_KEY = 'openai_subscription_expiry_snapshot'

export type OpenAISubscriptionExpirySnapshotStatus = 'available' | 'unavailable'
export type OpenAISubscriptionExpiryDisplaySource = 'upstream' | 'legacy' | 'manual' | 'none'

export interface OpenAISubscriptionExpirySnapshot {
  status: OpenAISubscriptionExpirySnapshotStatus
  expires_at?: string
  checked_at: string
  source?: string
  plan_type?: string
  will_renew?: boolean
}
export interface OpenAISubscriptionExpiryDisplay {
  expiresAt?: string
  source: OpenAISubscriptionExpiryDisplaySource
  checkedAt?: string
  snapshot?: OpenAISubscriptionExpirySnapshot
}

const validDateString = (value: unknown): string | undefined => {
  if (typeof value !== 'string' || value.trim() === '') return undefined
  return Number.isNaN(new Date(value).getTime()) ? undefined : value
}

export const readOpenAISubscriptionExpirySnapshot = (
  account: Account
): OpenAISubscriptionExpirySnapshot | undefined => {
  const value = account.extra?.[OPENAI_SUBSCRIPTION_EXPIRY_SNAPSHOT_KEY]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined

  const raw = value as Record<string, unknown>
  if (raw.status !== 'available' && raw.status !== 'unavailable') return undefined
  const checkedAt = validDateString(raw.checked_at)
  if (!checkedAt) return undefined

  return {
    status: raw.status,
    checked_at: checkedAt,
    ...(validDateString(raw.expires_at) ? { expires_at: raw.expires_at as string } : {}),
    ...(typeof raw.source === 'string' ? { source: raw.source } : {}),
    ...(typeof raw.plan_type === 'string' ? { plan_type: raw.plan_type } : {}),
    ...(typeof raw.will_renew === 'boolean' ? { will_renew: raw.will_renew } : {})
  }
}

const manualExpiry = (account: Account): string | undefined => {
  if (typeof account.expires_at !== 'number' || !Number.isFinite(account.expires_at) || account.expires_at <= 0) {
    return undefined
  }
  return new Date(account.expires_at * 1000).toISOString()
}

const legacyUpstreamExpiry = (account: Account): string | undefined =>
  validDateString(account.credentials?.subscription_expires_at) ??
  validDateString(account.parent_subscription_expires_at)

export const getOpenAISubscriptionExpiryDisplay = (
  account: Account
): OpenAISubscriptionExpiryDisplay => {
  const snapshot = readOpenAISubscriptionExpirySnapshot(account)
  if (snapshot) {
    if (snapshot.status === 'available' && snapshot.expires_at) {
      return {
        expiresAt: snapshot.expires_at,
        source: 'upstream',
        checkedAt: snapshot.checked_at,
        snapshot
      }
    }
    const fallback = manualExpiry(account)
    return {
      ...(fallback ? { expiresAt: fallback } : {}),
      source: fallback ? 'manual' : 'none',
      checkedAt: snapshot.checked_at,
      snapshot
    }
  }

  const legacy = legacyUpstreamExpiry(account)
  if (legacy) return { expiresAt: legacy, source: 'legacy' }

  const fallback = manualExpiry(account)
  return fallback
    ? { expiresAt: fallback, source: 'manual' }
    : { source: 'none' }
}
