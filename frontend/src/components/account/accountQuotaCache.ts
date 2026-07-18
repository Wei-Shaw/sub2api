import { adminAPI } from '@/api/admin'
import type { AccountQuotaResult } from '@/types'

const CACHE_TTL = 5 * 60 * 1000
const cache = new Map<number, { value: AccountQuotaResult; timestamp: number }>()
const inFlight = new Map<number, Promise<AccountQuotaResult>>()

export async function fetchAccountQuota(accountId: number, force = false): Promise<AccountQuotaResult> {
  const pending = inFlight.get(accountId)
  if (pending) return pending
  if (!force) {
    const cached = cache.get(accountId)
    if (cached && Date.now() - cached.timestamp < CACHE_TTL) return cached.value
  }
  const request = adminAPI.accounts.getQuota(accountId, 'active', force).then(value => {
    cache.set(accountId, { value, timestamp: Date.now() })
    return value
  }).finally(() => {
    inFlight.delete(accountId)
  })
  inFlight.set(accountId, request)
  return request
}
