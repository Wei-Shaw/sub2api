import type { Account } from '@/types'

function readBaseUrl(account: Pick<Account, 'credentials'> | null | undefined): string {
  if (!account?.credentials) return ''
  const raw = (account.credentials as Record<string, unknown>).base_url
  return typeof raw === 'string' ? raw.trim() : ''
}

/**
 * Adobe 外部中转账号:platform=adobe、type=apikey 且配置了 base_url。
 * 转发到外部 OpenAI 兼容上游，作为分组灾备。
 */
export function isAdobeRelayAccount(account: Pick<Account, 'platform' | 'type' | 'credentials'> | null | undefined): boolean {
  if (!account || account.platform !== 'adobe' || account.type !== 'apikey') return false
  return readBaseUrl(account) !== ''
}
