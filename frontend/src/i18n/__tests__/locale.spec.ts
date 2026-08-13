import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// `setLocale` lazily pulls the router and three stores in only to refresh the tab title.
// Stub them so the locale-sync assertions below do not need a live app.
vi.mock('@/router', () => ({
  default: { currentRoute: { value: { name: 'dashboard', path: '/dashboard', meta: {} } } }
}))
vi.mock('@/router/title', () => ({
  resolveRouteDocumentTitle: () => 'stubbed title'
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null, siteName: 'sub2api' })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false }) }))
vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] })
}))

const originalLanguage = navigator.language

function stubBrowserLanguage(language: string): void {
  Object.defineProperty(navigator, 'language', { value: language, configurable: true })
}

/** Re-import the module graph so the lazily cached locale is resolved from scratch. */
async function freshLocaleModule() {
  vi.resetModules()
  return import('@/i18n/locale')
}

describe('i18n/locale', () => {
  beforeEach(() => {
    localStorage.clear()
    stubBrowserLanguage('en-US')
  })

  afterEach(() => {
    stubBrowserLanguage(originalLanguage)
    localStorage.clear()
    vi.resetModules()
  })

  it('defaults to en when nothing was stored and the browser is not Chinese', async () => {
    const { getLocale, DEFAULT_LOCALE } = await freshLocaleModule()

    expect(getLocale()).toBe('en')
    expect(DEFAULT_LOCALE).toBe('en')
  })

  it('restores the stored locale on a fresh load', async () => {
    localStorage.setItem('sub2api_locale', 'zh')

    const { getLocale } = await freshLocaleModule()

    expect(getLocale()).toBe('zh')
  })

  it('sniffs a Chinese browser when nothing was stored', async () => {
    stubBrowserLanguage('zh-CN')

    const { getLocale } = await freshLocaleModule()

    expect(getLocale()).toBe('zh')
  })

  it('ignores a stored value that is not a supported locale', async () => {
    localStorage.setItem('sub2api_locale', 'fr-FR')

    const { getLocale } = await freshLocaleModule()

    expect(getLocale()).toBe('en')
  })

  it('reads back what was set', async () => {
    const { getLocale, setCurrentLocale } = await freshLocaleModule()

    setCurrentLocale('zh')
    expect(getLocale()).toBe('zh')

    setCurrentLocale('en')
    expect(getLocale()).toBe('en')
  })

  it('collapses an unsupported value to the default locale', async () => {
    const { getLocale, setCurrentLocale } = await freshLocaleModule()

    setCurrentLocale('zh')
    setCurrentLocale('de')

    expect(getLocale()).toBe('en')
  })

  it('narrows only supported locale codes', async () => {
    const { isLocaleCode } = await freshLocaleModule()

    expect(isLocaleCode('en')).toBe(true)
    expect(isLocaleCode('zh')).toBe(true)
    expect(isLocaleCode('zh-CN')).toBe(false)
    expect(isLocaleCode('')).toBe(false)
  })

  it('imports without building an i18n instance', async () => {
    const mod = await freshLocaleModule()

    // A leaf module: if it ever grew a `vue-i18n` import, the API layer would be coupled to
    // `createI18n()` again — the exact regression this split exists to prevent.
    expect(Object.keys(mod).sort()).toEqual([
      'DEFAULT_LOCALE',
      'LOCALE_STORAGE_KEY',
      'getDefaultLocale',
      'getLocale',
      'isLocaleCode',
      'setCurrentLocale'
    ])
  })
})

describe('i18n/locale kept in sync with the vue-i18n instance', () => {
  beforeEach(() => {
    localStorage.clear()
    stubBrowserLanguage('en-US')
    vi.resetModules()
  })

  afterEach(() => {
    stubBrowserLanguage(originalLanguage)
    localStorage.clear()
    document.documentElement.removeAttribute('lang')
    vi.resetModules()
  })

  it('boots the i18n instance on the same locale getLocale reports', async () => {
    localStorage.setItem('sub2api_locale', 'zh')

    const { i18n } = await import('@/i18n')
    const { getLocale } = await import('@/i18n/locale')

    expect(i18n.global.locale.value).toBe('zh')
    expect(getLocale()).toBe('zh')
  })

  it('follows setLocale, and still sets the html lang hook', async () => {
    const { setLocale } = await import('@/i18n')
    const { getLocale } = await import('@/i18n/locale')

    expect(getLocale()).toBe('en')

    await setLocale('zh')

    expect(getLocale()).toBe('zh')
    expect(localStorage.getItem('sub2api_locale')).toBe('zh')
    // `html[lang="zh"]` is a load-bearing CSS hook for the CJK font stack.
    expect(document.documentElement.getAttribute('lang')).toBe('zh')

    await setLocale('en')

    expect(getLocale()).toBe('en')
    expect(document.documentElement.getAttribute('lang')).toBe('en')
  })

  it('ignores an unsupported locale in setLocale', async () => {
    const { setLocale } = await import('@/i18n')
    const { getLocale } = await import('@/i18n/locale')

    await setLocale('fr')

    expect(getLocale()).toBe('en')
    expect(localStorage.getItem('sub2api_locale')).toBeNull()
  })

  it('follows a direct write to the vue-i18n locale ref', async () => {
    const { i18n } = await import('@/i18n')
    const { getLocale } = await import('@/i18n/locale')

    // Components hold the same ref via `useI18n().locale`; a write there must not leave
    // `Accept-Language` reporting the old locale.
    i18n.global.locale.value = 'zh'

    expect(getLocale()).toBe('zh')
  })

  it('re-exports getLocale from @/i18n for existing callers', async () => {
    const i18nModule = await import('@/i18n')
    const localeModule = await import('@/i18n/locale')

    expect(i18nModule.getLocale).toBe(localeModule.getLocale)
  })
})
