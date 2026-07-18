import { createApp REDACTED from 'vue'
import { createPinia REDACTED from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n REDACTED from './i18n'
import { useAppStore REDACTED from '@/stores/app'
import { updateFavicon REDACTED from '@/utils/branding'
import './style.css'

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
REDACTED

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteNameREDACTED - AI API Gateway`
  REDACTED
  updateFavicon(appStore.siteLogo)

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
REDACTED

bootstrap()
