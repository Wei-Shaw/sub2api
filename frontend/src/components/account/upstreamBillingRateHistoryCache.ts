import type { UpstreamBillingRateHistoryResponse } from '@/types'

export const UPSTREAM_BILLING_RATE_HISTORY_CACHE_LIMIT = 20

export type UpstreamBillingRateHistoryCacheEntry = {
  data: UpstreamBillingRateHistoryResponse
  etag: string | null
}

const historyCache = new Map<string, UpstreamBillingRateHistoryCacheEntry>()

const keyFor = (accountID: number, days: number) => `${accountID}:${days}`

export const getUpstreamBillingRateHistoryCache = (accountID: number, days: number) =>
  historyCache.get(keyFor(accountID, days))

export const setUpstreamBillingRateHistoryCache = (
  accountID: number,
  days: number,
  entry: UpstreamBillingRateHistoryCacheEntry
) => {
  const key = keyFor(accountID, days)
  historyCache.delete(key)
  historyCache.set(key, entry)
  while (historyCache.size > UPSTREAM_BILLING_RATE_HISTORY_CACHE_LIMIT) {
    const oldest = historyCache.keys().next().value as string | undefined
    if (oldest == null) break
    historyCache.delete(oldest)
  }
}

export const invalidateUpstreamBillingRateHistoryCache = (accountID?: number) => {
  if (accountID == null) {
    historyCache.clear()
    return
  }
  const prefix = `${accountID}:`
  for (const key of historyCache.keys()) {
    if (key.startsWith(prefix)) historyCache.delete(key)
  }
}
