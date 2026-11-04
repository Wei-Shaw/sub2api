const OFFICIAL_GEMINI_API_HOST = 'generativelanguage.googleapis.com'

/** Gemini API Key 账号的 base_url 是否为官方地址；留空视为官方地址。 */
export function isOfficialGeminiBaseUrl(raw: string | null | undefined): boolean {
  const value = (raw ?? '').trim()
  if (!value) return true
  try {
    const url = new URL(value)
    const path = url.pathname.replace(/\/+$/, '')
    return (
      url.protocol === 'https:' &&
      url.hostname.toLowerCase() === OFFICIAL_GEMINI_API_HOST &&
      url.port === '' &&
      path === '' &&
      url.search === ''
    )
  } catch {
    return false
  }
}

/** 上游缓存有效期上限（秒）：非正数或非数字视为未配置。 */
export function normalizeExplicitCacheUpstreamMaxTTL(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const seconds = Number(value)
  return Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : null
}

/** 把上游显式缓存开关写入（或移出）账号凭据。 */
export function applyExplicitCacheUpstreamCredentials(
  credentials: Record<string, unknown>,
  enabled: boolean,
  maxTTL: unknown
): void {
  if (!enabled) {
    delete credentials.explicit_cache_upstream
    delete credentials.explicit_cache_upstream_max_ttl_seconds
    return
  }
  credentials.explicit_cache_upstream = true
  const seconds = normalizeExplicitCacheUpstreamMaxTTL(maxTTL)
  if (seconds === null) {
    delete credentials.explicit_cache_upstream_max_ttl_seconds
  } else {
    credentials.explicit_cache_upstream_max_ttl_seconds = seconds
  }
}
