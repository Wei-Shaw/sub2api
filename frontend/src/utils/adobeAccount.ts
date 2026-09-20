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

/**
 * 把用户贴进 Adobe Cookie 框的内容归一成 `k=v; k=v`。
 *
 * 接受裸 cookie、`Cookie:` 前缀、cookie 数组、`{cookie}` / `{cookies}` 对象，
 * 以及插件导出的 `sub2api-data` envelope。
 */
export function extractAdobeCookieInput(input: string): string {
  const text = input.trim()
  if (!text) return ''
  if (looksLikeJsonValue(text)) {
    try {
      return normalizeAdobeCookieValue(JSON.parse(text))
    } catch {
      return stripCookieHeaderPrefix(text)
    }
  }
  return stripCookieHeaderPrefix(text)
}

function looksLikeJsonValue(text: string): boolean {
  const start = text[0]
  return start === '{' || start === '[' || start === '"'
}

function stripCookieHeaderPrefix(text: string): string {
  if (text.slice(0, 7).toLowerCase() === 'cookie:') {
    return text.slice(text.indexOf(':') + 1).trim()
  }
  return text
}

function normalizeAdobeCookieValue(value: unknown, depth = 0): string {
  if (depth > 4 || value == null) return ''
  if (typeof value === 'string') {
    const text = value.trim()
    if (!text) return ''
    if (looksLikeJsonValue(text)) {
      try {
        return normalizeAdobeCookieValue(JSON.parse(text), depth + 1)
      } catch {
        return stripCookieHeaderPrefix(text)
      }
    }
    return stripCookieHeaderPrefix(text)
  }
  if (Array.isArray(value)) {
    const pairs: string[] = []
    for (const item of value) {
      if (typeof item === 'string') {
        const trimmed = item.trim()
        if (trimmed) pairs.push(trimmed)
        continue
      }
      if (item && typeof item === 'object' && !Array.isArray(item)) {
        const record = item as Record<string, unknown>
        const name = typeof record.name === 'string' ? record.name.trim() : ''
        if (!name) continue
        const cookieValue = typeof record.value === 'string' ? record.value.trim() : ''
        pairs.push(`${name}=${cookieValue}`)
      }
    }
    return pairs.join('; ')
  }
  if (typeof value !== 'object') return ''

  const record = value as Record<string, unknown>
  if (Array.isArray(record.accounts) && record.accounts.length > 0) {
    const first = record.accounts[0]
    if (first && typeof first === 'object') {
      const credentials = (first as Record<string, unknown>).credentials
      if (credentials && typeof credentials === 'object') {
        const extracted = normalizeAdobeCookieValue(
          (credentials as Record<string, unknown>).cookie,
          depth + 1
        )
        if (extracted) return extracted
      }
    }
  }
  if ('cookies' in record) {
    return normalizeAdobeCookieValue(record.cookies, depth + 1)
  }
  if ('cookie' in record) {
    return normalizeAdobeCookieValue(record.cookie, depth + 1)
  }
  return ''
}

