import { isRef, watch } from 'vue'
import { createI18n } from 'vue-i18n'
import {
  DEFAULT_LOCALE,
  LOCALE_STORAGE_KEY,
  getLocale,
  isLocaleCode,
  setCurrentLocale,
  type LocaleCode
} from './locale'

type LocaleMessages = Record<string, any>

// `getLocale` owns the locale value (see ./locale.ts — a leaf module that never builds an
// i18n instance, so `api/client` can read the locale without dragging `createI18n` along).
// Re-exported because callers outside the API layer already import it from `@/i18n`.
export { getLocale }
export type { LocaleCode }

const localeLoaders: Record<LocaleCode, () => Promise<{ default: LocaleMessages }>> = {
  en: () => import('./locales/en'),
  zh: () => import('./locales/zh')
}

export const i18n = createI18n({
  legacy: false,
  locale: getLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {},
  // 禁用 HTML 消息警告 - 引导步骤使用富文本内容（driver.js 支持 HTML）
  // 这些内容是内部定义的，不存在 XSS 风险
  warnHtmlMessage: false
})

// Single guarantee that the two locale readers cannot drift: every write to the vue-i18n
// locale — `setLocale()` below, or any component reaching for `useI18n().locale` directly —
// pushes the same value into ./locale.ts. Synchronous flush matters: a request fired in the
// same tick as the switch must already carry the new `Accept-Language`.
// Guarded because specs may stub `createI18n` with an object that has no locale ref.
const localeRef = i18n.global.locale
if (isRef(localeRef)) {
  watch(localeRef, (locale) => setCurrentLocale(locale), { flush: 'sync' })
}

const loadedLocales = new Set<LocaleCode>()

export async function loadLocaleMessages(locale: LocaleCode): Promise<void> {
  if (loadedLocales.has(locale)) {
    return
  }

  const loader = localeLoaders[locale]
  const module = await loader()
  i18n.global.setLocaleMessage(locale, module.default)
  loadedLocales.add(locale)
}

export async function initI18n(): Promise<void> {
  const current = getLocale()
  await loadLocaleMessages(current)
  document.documentElement.setAttribute('lang', current)
}

export async function setLocale(locale: string): Promise<void> {
  if (!isLocaleCode(locale)) {
    return
  }

  await loadLocaleMessages(locale)
  i18n.global.locale.value = locale
  // Belt and braces alongside the watcher above, so the value is correct even if the
  // watcher is ever torn down or the locale ref is stubbed in a test.
  setCurrentLocale(locale)
  localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  document.documentElement.setAttribute('lang', locale)

  // 同步更新浏览器页签标题，使其跟随语言切换
  const { resolveRouteDocumentTitle } = await import('@/router/title')
  const { default: router } = await import('@/router')
  const { useAppStore } = await import('@/stores/app')
  const { useAuthStore } = await import('@/stores/auth')
  const { useAdminSettingsStore } = await import('@/stores/adminSettings')
  const route = router.currentRoute.value
  const appStore = useAppStore()
  const authStore = useAuthStore()
  const adminSettingsStore = useAdminSettingsStore()
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  document.title = resolveRouteDocumentTitle(route, appStore.siteName, customMenuItems)
}

export const availableLocales = [
  { code: 'en', name: 'English', flag: '🇺🇸' },
  { code: 'zh', name: '中文', flag: '🇨🇳' }
] as const

export default i18n
