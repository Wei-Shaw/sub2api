import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import { attachHostSdkToWindow } from '@/plugins/sdk/host-sdk-window'
import { exposePluginRuntime } from '@/plugins/sdk/expose-runtime'
import './style.css'

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

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
    document.title = `${appStore.siteName} - AI API Gateway`
  }

  await initI18n()

  app.use(router)
  app.use(i18n)

  // Pinia + i18n + router 都已安装,可以创建并挂载 host SDK 给插件 entry.js 使用。
  // 必须在 router.isReady 之前完成,因为插件路由可能马上就被首屏导航命中。
  attachHostSdkToWindow(router, pinia)

  // 把 vue / pinia / vue-router / vue-i18n / axios 单例暴露到 window 上,
  // 让 plugin entry.js (通过 importmap → /__shared__/<name>.js → window
  // 三跳) 拿到与 host 相同的单例. 这一步必须在任何 plugin entry import
  // 之前完成 (loader-runtime 在用户访问插件路由时才触发, 所以这里就够了).
  exposePluginRuntime()

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
