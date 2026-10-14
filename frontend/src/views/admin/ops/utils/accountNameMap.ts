import { accountsAPI } from '@/api/admin/accounts'

// 运维监控请求明细只带 account_id，账号名由前端用 lite 账号列表建映射；
// 模块级缓存避免每次打开弹窗都拉全量账号。
const ACCOUNT_NAME_PAGE_SIZE = 1000

let accountNameCache: Map<number, string> | null = null

export async function getAccountNameMap(force = false): Promise<Map<number, string>> {
  if (!force && accountNameCache) return accountNameCache

  const map = new Map<number, string>()
  let page = 1
  let total = Number.POSITIVE_INFINITY
  while (map.size < total) {
    const res = await accountsAPI.list(page, ACCOUNT_NAME_PAGE_SIZE, {
      lite: '1',
      include_scheduler_score: '0'
    })
    total = res.total || 0
    const items = res.items || []
    if (items.length === 0) break
    for (const item of items) map.set(item.id, item.name)
    page++
  }

  accountNameCache = map
  return map
}

export function resetAccountNameCacheForTests() {
  accountNameCache = null
}
