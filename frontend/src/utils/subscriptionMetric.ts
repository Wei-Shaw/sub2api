import { formatTokenCount } from '@/utils/format'
import type { UserSubscription } from '@/types'

type UsageWindow = 'daily' | 'weekly' | 'monthly'

export function isTokenSubscription(sub: UserSubscription): boolean {
  return sub.group?.subscription_type === 'subscription_token'
}

// windowMetric 返回窗口的已用量与限额，token 型与 USD 型自动切换字段来源。
export function windowMetric(sub: UserSubscription, window: UsageWindow) {
  const token = isTokenSubscription(sub)
  const g = sub.group
  switch (window) {
    case 'daily':
      return token
        ? { used: sub.daily_usage_tokens || 0, limit: g?.daily_limit_tokens ?? null }
        : { used: sub.daily_usage_usd || 0, limit: g?.daily_limit_usd ?? null }
    case 'weekly':
      return token
        ? { used: sub.weekly_usage_tokens || 0, limit: g?.weekly_limit_tokens ?? null }
        : { used: sub.weekly_usage_usd || 0, limit: g?.weekly_limit_usd ?? null }
    default:
      return token
        ? { used: sub.monthly_usage_tokens || 0, limit: g?.monthly_limit_tokens ?? null }
        : { used: sub.monthly_usage_usd || 0, limit: g?.monthly_limit_usd ?? null }
  }
}

export function hasWindowLimit(sub: UserSubscription, window: UsageWindow): boolean {
  const { limit } = windowMetric(sub, window)
  return !!limit && limit > 0
}

export function windowPercentage(sub: UserSubscription, window: UsageWindow): number {
  const { used, limit } = windowMetric(sub, window)
  if (!limit || limit <= 0) return 0
  return (used / limit) * 100
}

export function maxUsagePercentage(sub: UserSubscription): number {
  const percentages: number[] = [
    windowPercentage(sub, 'daily'),
    windowPercentage(sub, 'weekly'),
    windowPercentage(sub, 'monthly')
  ].filter((p) => p > 0)
  return percentages.length > 0 ? Math.max(...percentages) : 0
}

export function isSubscriptionUnlimited(sub: UserSubscription): boolean {
  return (
    !hasWindowLimit(sub, 'daily') &&
    !hasWindowLimit(sub, 'weekly') &&
    !hasWindowLimit(sub, 'monthly')
  )
}

export function formatWindowUsage(sub: UserSubscription, window: UsageWindow): string {
  const { used, limit } = windowMetric(sub, window)
  if (isTokenSubscription(sub)) {
    return `${formatTokenCount(used)} / ${limit != null ? formatTokenCount(limit) : '∞'}`
  }
  return `$${used.toFixed(2)} / ${limit != null ? `$${limit.toFixed(2)}` : '∞'}`
}

export function progressBarClass(pct: number): string {
  if (pct >= 90) return 'bg-red-500'
  if (pct >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

export function progressBarWidth(pct: number): string {
  return `${Math.min(pct, 100)}%`
}
