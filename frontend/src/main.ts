import { createApp REDACTED from 'vue'
import { createPinia REDACTED from 'pinia'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)

// 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
router.isReady().then(() => {
  app.mount('#app')
REDACTED)
