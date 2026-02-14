import { createApp REDACTED from 'vue'
import { createPinia REDACTED from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n REDACTED from './i18n'
import { useAppStore REDACTED from '@/stores/app'
import './style.css'

async function bootstrap() {
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

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
REDACTED

bootstrap()
