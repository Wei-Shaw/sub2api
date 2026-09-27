import { createI18n } from 'vue-i18n'

type LocaleCode = 'en' | 'zh' | 'vi'

type LocaleMessages = Record<string, any>

const LOCALE_KEY = 'sub2api_locale'
const DEFAULT_LOCALE: LocaleCode = 'en'

// Destructured so the bundler tree-shakes the full default export (and the admin namespace) out of the chunk.
const localeLoaders: Record<LocaleCode, () => Promise<LocaleMessages>> = {
  en: () => import('./locales/en').then(({ baseMessages }) => baseMessages),
  zh: () => import('./locales/zh').then(({ baseMessages }) => baseMessages),
  vi: () => import('./locales/vi').then(({ baseMessages }) => baseMessages)
}

// admin.* is ~72% of each bundle and only needed once signed in, so it is a separate chunk.
const adminLocaleLoaders: Record<LocaleCode, () => Promise<{ default: LocaleMessages }>> = {
  en: () => import('./locales/en/admin'),
  zh: () => import('./locales/zh/admin'),
  vi: () => import('./locales/vi/admin')
}

function isLocaleCode(value: string): value is LocaleCode {
  return value === 'en' || value === 'zh' || value === 'vi'
}

function getDefaultLocale(): LocaleCode {
  const saved = localStorage.getItem(LOCALE_KEY)
  if (saved && isLocaleCode(saved)) {
    return saved
  }

  const browserLang = navigator.language.toLowerCase()
  if (browserLang.startsWith('zh')) {
    return 'zh'
  }
  if (browserLang.startsWith('vi')) {
    return 'vi'
  }

  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: getDefaultLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {},
  // 禁用 HTML 消息警告 - 引导步骤使用富文本内容（driver.js 支持 HTML）
  // 这些内容是内部定义的，不存在 XSS 风险
  warnHtmlMessage: false
})

const loadedLocales = new Set<LocaleCode>()
const loadedAdminLocales = new Set<LocaleCode>()
// Set by the first signed-in navigation; from then on setLocale() also loads admin.* for the new locale.
let adminMessagesNeeded = false

export async function loadLocaleMessages(locale: LocaleCode): Promise<void> {
  if (!loadedLocales.has(locale)) {
    const loader = localeLoaders[locale]
    i18n.global.setLocaleMessage(locale, await loader())
    loadedLocales.add(locale)
  }

  if (adminMessagesNeeded && !loadedAdminLocales.has(locale)) {
    const module = await adminLocaleLoaders[locale]()
    i18n.global.mergeLocaleMessage(locale, { admin: module.default })
    loadedAdminLocales.add(locale)
  }
}

/**
 * Loads the admin.* namespace for the current locale. Signed-in pages need it even for
 * non-admins (AppHeader role label, GroupBadge…), so the router awaits it before entering them.
 */
export async function loadAdminLocaleMessages(): Promise<void> {
  adminMessagesNeeded = true
  await loadLocaleMessages(getLocale())
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
  localStorage.setItem(LOCALE_KEY, locale)
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

export function getLocale(): LocaleCode {
  const current = i18n.global.locale.value
  return isLocaleCode(current) ? current : DEFAULT_LOCALE
}

export const availableLocales = [
  { code: 'en', name: 'English', flag: '🇺🇸' },
  { code: 'zh', name: '中文', flag: '🇨🇳' },
  { code: 'vi', name: 'Tiếng Việt', flag: '🇻🇳' }
] as const

export default i18n
