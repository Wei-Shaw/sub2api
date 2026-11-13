import { adminAPI } from '@/api/admin'
import { createAccountUsageCache } from '@/utils/accountUsageCache'

let cache: ReturnType<typeof createAccountUsageCache> | undefined
export function useAccountUsage() {
  if (!cache) {
    cache = createAccountUsageCache((ids, force) => adminAPI.accounts.getBatchUsage(ids, force), () => {
      try { return String(JSON.parse(window.localStorage.getItem('auth_user') || '{}').id ?? 'anonymous') }
      catch { return 'anonymous' }
    })
  }
  return cache
}
