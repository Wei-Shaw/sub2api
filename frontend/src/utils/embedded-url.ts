const EMBEDDED_USER_ID_QUERY_KEY = 'user_id'
const EMBEDDED_AUTH_TOKEN_QUERY_KEY = 'token'
const EMBEDDED_THEME_QUERY_KEY = 'theme'
const EMBEDDED_LANG_QUERY_KEY = 'lang'
const EMBEDDED_UI_MODE_QUERY_KEY = 'ui_mode'
const EMBEDDED_UI_MODE_VALUE = 'embedded'
const EMBEDDED_SRC_HOST_QUERY_KEY = 'src_host'
const EMBEDDED_SRC_QUERY_KEY = 'src_url'

export type EmbeddedQueryParamKey =
  | 'user_id'
  | 'token'
  | 'theme'
  | 'lang'
  | 'ui_mode'
  | 'src_host'
  | 'src_url'

export const DEFAULT_EMBEDDED_QUERY_PARAM_KEYS: EmbeddedQueryParamKey[] = [
  'user_id',
  'token',
  'theme',
  'lang',
  'ui_mode',
  'src_host',
  'src_url',
]

export interface EmbeddedUrlOptions {
  appendParams?: boolean
  paramKeys?: EmbeddedQueryParamKey[]
}

export function buildEmbeddedUrl(
  baseUrl: string,
  userId?: number,
  authToken?: string | null,
  theme: 'light' | 'dark' = 'light',
  lang?: string,
  options: EmbeddedUrlOptions = {},
): string {
  if (!baseUrl) return baseUrl
  if (options.appendParams === false) return baseUrl
  try {
    const url = new URL(baseUrl)
    const paramKeys = new Set(options.paramKeys ?? DEFAULT_EMBEDDED_QUERY_PARAM_KEYS)
    if (userId && paramKeys.has('user_id')) {
      url.searchParams.set(EMBEDDED_USER_ID_QUERY_KEY, String(userId))
    }
    if (authToken && paramKeys.has('token')) {
      url.searchParams.set(EMBEDDED_AUTH_TOKEN_QUERY_KEY, authToken)
    }
    if (paramKeys.has('theme')) {
      url.searchParams.set(EMBEDDED_THEME_QUERY_KEY, theme)
    }
    if (lang && paramKeys.has('lang')) {
      url.searchParams.set(EMBEDDED_LANG_QUERY_KEY, lang)
    }
    if (paramKeys.has('ui_mode')) {
      url.searchParams.set(EMBEDDED_UI_MODE_QUERY_KEY, EMBEDDED_UI_MODE_VALUE)
    }
    if (typeof window !== 'undefined' && paramKeys.has('src_host')) {
      url.searchParams.set(EMBEDDED_SRC_HOST_QUERY_KEY, window.location.origin)
    }
    if (typeof window !== 'undefined' && paramKeys.has('src_url')) {
      url.searchParams.set(EMBEDDED_SRC_QUERY_KEY, window.location.href)
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
