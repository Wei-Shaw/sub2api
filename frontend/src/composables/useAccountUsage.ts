import { adminAPI } from '@/api/admin'
import { createAccountUsageCache } from '@/utils/accountUsageCache'

let cache: ReturnType<typeof createAccountUsageCache> | undefined
export function useAccountUsage() {
  if (!cache) {
    let storage: Storage | undefined
    try { storage = window.localStorage } catch { /* Browser storage may be disabled. */ }
    cache = createAccountUsageCache((ids, force) => adminAPI.accounts.getBatchUsage(ids, force), storage, () => {
      try { return String(JSON.parse(storage?.getItem('auth_user') || '{}').id ?? 'anonymous') }
      catch { return 'anonymous' }
    })
  }
  return cache
}
