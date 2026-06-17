const OAUTH_PROMO_CODE_KEY = 'oauth_promo_code'

export function normalizeOAuthPromoCode(value?: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value
  return typeof raw === 'string' ? raw.trim() : ''
}

export function storeOAuthPromoCode(value?: unknown): void {
  if (typeof window === 'undefined') {
    return
  }
  const code = normalizeOAuthPromoCode(value)
  try {
    if (code) {
      window.sessionStorage.setItem(OAUTH_PROMO_CODE_KEY, code)
    } else {
      window.sessionStorage.removeItem(OAUTH_PROMO_CODE_KEY)
    }
  } catch {
    // 忽略浏览器存储异常。
  }
}

export function loadOAuthPromoCode(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  try {
    return normalizeOAuthPromoCode(window.sessionStorage.getItem(OAUTH_PROMO_CODE_KEY))
  } catch {
    return ''
  }
}

export function resolveOAuthPromoCode(...values: unknown[]): string {
  for (const value of values) {
    const code = normalizeOAuthPromoCode(value)
    if (code) {
      storeOAuthPromoCode(code)
      return code
    }
  }
  return loadOAuthPromoCode()
}
