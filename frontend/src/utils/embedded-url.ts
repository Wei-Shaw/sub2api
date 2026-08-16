/**
 * Shared URL builder for iframe-embedded pages.
 * Used by PurchaseSubscriptionView and CustomPageView to build consistent URLs
 * with non-secret presentation context only. Authentication credentials must
 * never be placed in iframe URLs because URLs leak through history, logs, and
 * the embedded origin itself.
 */

const EMBEDDED_THEME_QUERY_KEY = 'theme'
const EMBEDDED_LANG_QUERY_KEY = 'lang'
const EMBEDDED_UI_MODE_QUERY_KEY = 'ui_mode'
const EMBEDDED_UI_MODE_VALUE = 'embedded'

export function buildEmbeddedUrl(
  baseUrl: string,
  theme: 'light' | 'dark' = 'light',
  lang?: string,
): string {
  if (!baseUrl) return baseUrl
  try {
    const url = new URL(baseUrl)
    url.searchParams.set(EMBEDDED_THEME_QUERY_KEY, theme)
    if (lang) {
      url.searchParams.set(EMBEDDED_LANG_QUERY_KEY, lang)
    }
    url.searchParams.set(EMBEDDED_UI_MODE_QUERY_KEY, EMBEDDED_UI_MODE_VALUE)
    return url.toString()
  } catch {
    return baseUrl
  }
}

export function detectTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}
