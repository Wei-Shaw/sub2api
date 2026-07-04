import type { Account } from '@/types'

/**
 * 读取账号 credentials 中的 base_url(去空白)。
 */
function readBaseUrl(account: Pick<Account, 'credentials'> | null | undefined): string {
  if (!account?.credentials) return ''
  const raw = (account.credentials as Record<string, unknown>).base_url
  return typeof raw === 'string' ? raw.trim() : ''
}

/**
 * Kiro 外部中转账号:platform=kiro、type=apikey 且配置了 base_url。
 * 这类账号转发到外部 Anthropic 兼容上游({base_url}/v1/messages),
 * 不直连 AWS、无 Kiro credits,作为分组兜底/灾备。
 */
export function isKiroRelayAccount(account: Pick<Account, 'platform' | 'type' | 'credentials'> | null | undefined): boolean {
  if (!account || account.platform !== 'kiro' || account.type !== 'apikey') return false
  return readBaseUrl(account) !== ''
}

/**
 * Kiro 直连 AWS 的 API Key 账号:platform=kiro、type=apikey 且未配置 base_url。
 * 这类账号用 ksk_ 直连 q.{region}.amazonaws.com,展示 Kiro credits。
 */
export function isKiroDirectApiKeyAccount(account: Pick<Account, 'platform' | 'type' | 'credentials'> | null | undefined): boolean {
  if (!account || account.platform !== 'kiro' || account.type !== 'apikey') return false
  return readBaseUrl(account) === ''
}
