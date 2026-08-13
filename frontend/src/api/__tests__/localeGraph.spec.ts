import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * Guards the module-graph split between the API layer and the i18n instance.
 *
 * `api/client` reads the active locale for its `Accept-Language` header. It used to read it
 * from `@/i18n`, which calls `createI18n()` at module scope — so importing *any* API module
 * built a full i18n instance. Specs that mock `vue-i18n` with a partial factory (only
 * `useI18n`, which is all a component under test needs) then died on the first late import
 * of `api/client` with:
 *
 *   Error: [vitest] No "createI18n" export is defined on the "vue-i18n" mock.
 *
 * Under `--coverage` the instrumented module load was slow enough that this first import
 * landed after the test environment had torn down, surfacing as an unhandled rejection that
 * failed the whole run (`Errors 1 error`, exit 1) while every test passed. The blamed spec
 * changed run to run, because the coupling — not any one spec — was the defect.
 *
 * The partial mock below is the whole point: it must stay partial.
 */
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } })
}))

const originalLanguage = navigator.language

function stubBrowserLanguage(language: string): void {
  Object.defineProperty(navigator, 'language', { value: language, configurable: true })
}

function okAdapter() {
  return vi.fn().mockResolvedValue({
    status: 200,
    data: { code: 0, data: {} },
    headers: {},
    config: {},
    statusText: 'OK'
  })
}

async function acceptLanguageOf(apiClient: {
  defaults: { adapter?: unknown }
  get: (url: string) => Promise<unknown>
}): Promise<string> {
  const adapter = okAdapter()
  apiClient.defaults.adapter = adapter
  await apiClient.get('/test')
  return adapter.mock.calls[0][0].headers.get('Accept-Language')
}

describe('api/client locale coupling', () => {
  beforeEach(() => {
    localStorage.clear()
    stubBrowserLanguage('en-US')
    vi.resetModules()
  })

  afterEach(() => {
    stubBrowserLanguage(originalLanguage)
    localStorage.clear()
    vi.resetModules()
  })

  it('imports without pulling createI18n into the graph', async () => {
    // Would throw "No \"createI18n\" export is defined on the \"vue-i18n\" mock" if any module
    // reachable from api/client still built an i18n instance.
    await expect(import('@/api/client')).resolves.toHaveProperty('apiClient')
  })

  it('sends the default locale as Accept-Language', async () => {
    const { apiClient } = await import('@/api/client')

    expect(await acceptLanguageOf(apiClient)).toBe('en')
  })

  it('sends the stored locale after a reload', async () => {
    localStorage.setItem('sub2api_locale', 'zh')

    const { apiClient } = await import('@/api/client')

    expect(await acceptLanguageOf(apiClient)).toBe('zh')
  })

  it('follows a language switch', async () => {
    const { apiClient } = await import('@/api/client')
    const { setCurrentLocale } = await import('@/i18n/locale')

    setCurrentLocale('zh')

    expect(await acceptLanguageOf(apiClient)).toBe('zh')
  })

  it('falls back to the default locale when the active locale is unsupported', async () => {
    const { apiClient } = await import('@/api/client')
    const { setCurrentLocale } = await import('@/i18n/locale')

    setCurrentLocale('zh')
    setCurrentLocale('ja')

    expect(await acceptLanguageOf(apiClient)).toBe('en')
  })
})
