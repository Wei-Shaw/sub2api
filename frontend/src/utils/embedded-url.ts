/**
 * Shared URL builder for iframe-embedded pages.
 * Used by CustomPageView to build consistent URLs with user_id, theme, lang,
 * ui_mode, src_host and src_url parameters.
 *
 * 安全说明：`token` 是用户真实的面板访问 JWT（与 `Authorization: Bearer` 同一枚，
 * 默认有效期 24 小时，且管理员打开管理员可见的自定义页面时泄露的是管理员令牌）。
 * 因此它**不再默认透传**：只有当调用方显式传入 `passToken = true`
 * （对应菜单项的 `pass_token` 开关，后端要求该菜单项必须是 https 地址）时才会附加。
 */

const EMBEDDED_USER_ID_QUERY_KEY = 'user_id'
const EMBEDDED_AUTH_TOKEN_QUERY_KEY = 'token'
const EMBEDDED_THEME_QUERY_KEY = 'theme'
const EMBEDDED_LANG_QUERY_KEY = 'lang'
const EMBEDDED_UI_MODE_QUERY_KEY = 'ui_mode'
const EMBEDDED_UI_MODE_VALUE = 'embedded'
const EMBEDDED_SRC_HOST_QUERY_KEY = 'src_host'
const EMBEDDED_SRC_QUERY_KEY = 'src_url'

/**
 * 父页面地址只保留 origin + pathname：查询串/hash 可能带上会话相关的一次性参数，
 * 没有任何已知的接入方需要它们，而 src_host 已经提供了来源站点。
 */
function currentSourceUrl(): string {
  try {
    const parsed = new URL(window.location.href)
    return `${parsed.origin}${parsed.pathname}`
  } catch {
    return window.location.origin
  }
}

export function buildEmbeddedUrl(
  baseUrl: string,
  userId?: number,
  authToken?: string | null,
  theme: 'light' | 'dark' = 'light',
  lang?: string,
  passToken = false,
): string {
  if (!baseUrl) return baseUrl
  try {
    const url = new URL(baseUrl)
    if (userId) {
      url.searchParams.set(EMBEDDED_USER_ID_QUERY_KEY, String(userId))
    }
    if (passToken && authToken) {
      url.searchParams.set(EMBEDDED_AUTH_TOKEN_QUERY_KEY, authToken)
    }
    url.searchParams.set(EMBEDDED_THEME_QUERY_KEY, theme)
    if (lang) {
      url.searchParams.set(EMBEDDED_LANG_QUERY_KEY, lang)
    }
    url.searchParams.set(EMBEDDED_UI_MODE_QUERY_KEY, EMBEDDED_UI_MODE_VALUE)
    // Source tracking: let the embedded page know where it's being loaded from
    if (typeof window !== 'undefined') {
      url.searchParams.set(EMBEDDED_SRC_HOST_QUERY_KEY, window.location.origin)
      url.searchParams.set(EMBEDDED_SRC_QUERY_KEY, currentSourceUrl())
    }
    return url.toString()
  } catch {
    return baseUrl
  }
}

export function detectTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}
