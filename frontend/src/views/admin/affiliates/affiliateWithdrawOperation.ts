/** 一笔线下提现登记及标识它的幂等键。 */
export interface AffiliateWithdrawOperation {
  userId: number
  amount: number
  key: string
  storageKey: string | null
  outcomeUncertain: boolean
}

// 返利额度按 8 位小数记账。
const LEDGER_SCALE = 1e8

const pendingKeys = new Map<string, string>()

function currentAdminId(): number | null {
  try {
    const user = JSON.parse(globalThis.localStorage?.getItem('auth_user') ?? 'null') as { id?: unknown } | null
    const id = user?.id
    return typeof id === 'number' && Number.isSafeInteger(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

function readStoredKey(storageKey: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(storageKey) ?? null
  } catch {
    return null
  }
}

function storeKey(storageKey: string, key: string | null) {
  try {
    if (key) globalThis.sessionStorage?.setItem(storageKey, key)
    else globalThis.sessionStorage?.removeItem(storageKey)
  } catch {
    // 浏览器存储不可用时，同一页面内的重试仍由内存中的键保护。
  }
}

/**
 * 取得该用户与金额对应的登记。结果未知的登记在完成前一直保留它的键，
 * 再次提交同一用户与金额时沿用该键，服务端据此返回首次结果而不重复扣减。
 */
export function prepareAffiliateWithdrawOperation(userId: number, amount: number): AffiliateWithdrawOperation {
  const normalizedAmount = Math.round(amount * LEDGER_SCALE) / LEDGER_SCALE
  const adminId = currentAdminId()
  const storageKey = adminId ? `sub2api:admin:affiliate-withdraw:${adminId}:${userId}:${normalizedAmount}` : null
  let key = storageKey ? pendingKeys.get(storageKey) ?? readStoredKey(storageKey) : null
  const outcomeUncertain = !!key
  if (!key) {
    const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    key = `affiliate-withdraw-${adminId ?? 'unknown'}-${requestId}`
  }
  if (storageKey) {
    pendingKeys.set(storageKey, key)
    storeKey(storageKey, key)
  }
  return { userId, amount: normalizedAmount, key, storageKey, outcomeUncertain }
}

/** 登记有了确定结果后释放它的键。 */
export function completeAffiliateWithdrawOperation(operation: AffiliateWithdrawOperation) {
  if (!operation.storageKey) return
  pendingKeys.delete(operation.storageKey)
  storeKey(operation.storageKey, null)
}
