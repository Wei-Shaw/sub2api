/**
 * Locale source of truth — deliberately a leaf module with no imports at all.
 *
 * `api/client` needs the active locale for its `Accept-Language` header. Reading it from
 * `@/i18n` meant every module graph that touched the API layer also evaluated
 * `createI18n()`, so any spec that mocked `vue-i18n` with a partial factory blew up on the
 * first *late* import of `api/client` ("No \"createI18n\" export is defined on the
 * \"vue-i18n\" mock"). Keeping the locale here decouples the two: nothing in this file
 * pulls in `vue-i18n`, a store, or the router, so importing it can never build an i18n
 * instance.
 *
 * `@/i18n` keeps `i18n.global.locale` and the value below in lockstep (see the sync watcher
 * in `./index.ts`), so both readers always agree on what locale is active.
 */

export type LocaleCode = 'en' | 'zh'

export const LOCALE_STORAGE_KEY = 'sub2api_locale'

export const DEFAULT_LOCALE: LocaleCode = 'en'

export function isLocaleCode(value: string): value is LocaleCode {
  return value === 'en' || value === 'zh'
}

/**
 * Resolve the locale to boot with: an explicit past choice wins, otherwise sniff the
 * browser, otherwise fall back to the default.
 */
export function getDefaultLocale(): LocaleCode {
  const saved = localStorage.getItem(LOCALE_STORAGE_KEY)
  if (saved && isLocaleCode(saved)) {
    return saved
  }

  const browserLang = navigator.language.toLowerCase()
  if (browserLang.startsWith('zh')) {
    return 'zh'
  }

  return DEFAULT_LOCALE
}

// Resolved lazily so this module has no import-time side effects, then cached so the
// answer cannot drift mid-session the way a fresh `getDefaultLocale()` read could.
let currentLocale: LocaleCode | null = null

export function getLocale(): LocaleCode {
  if (currentLocale === null) {
    currentLocale = getDefaultLocale()
  }
  return currentLocale
}

/**
 * Record the active locale. Unknown values collapse to {@link DEFAULT_LOCALE} so
 * `getLocale()` never hands out a code the backend does not understand.
 */
export function setCurrentLocale(locale: string): void {
  currentLocale = isLocaleCode(locale) ? locale : DEFAULT_LOCALE
}
