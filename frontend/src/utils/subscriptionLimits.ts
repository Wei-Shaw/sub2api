import type { UserSubscription } from '@/types'

export type SubscriptionLimitWindow = 'daily' | 'weekly' | 'monthly'
export type SubscriptionLimitInput = number | string | null | undefined

export function parseOptionalSubscriptionLimit(value: SubscriptionLimitInput): number | null {
  if (value === null || value === undefined) return null

  const normalized = typeof value === 'string' ? value.trim() : value
  if (normalized === '') return null

  const parsed = Number(normalized)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new RangeError('subscription limit must be greater than zero')
  }
  return parsed
}

export function getEffectiveSubscriptionLimit(
  subscription: UserSubscription,
  window: SubscriptionLimitWindow
): number | null {
  const override = subscription[`${window}_limit_usd`]
  if (override != null && override > 0) return override

  const groupLimit = subscription.group?.[`${window}_limit_usd`]
  return groupLimit != null && groupLimit > 0 ? groupLimit : null
}

export function hasAnyEffectiveSubscriptionLimit(subscription: UserSubscription): boolean {
  return (
    getEffectiveSubscriptionLimit(subscription, 'daily') != null ||
    getEffectiveSubscriptionLimit(subscription, 'weekly') != null ||
    getEffectiveSubscriptionLimit(subscription, 'monthly') != null
  )
}
