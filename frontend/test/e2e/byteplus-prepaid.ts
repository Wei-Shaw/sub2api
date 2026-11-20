// Manual browser fixture only. Vite's production entry does not include this
// page, and the API server is an integration-test binary with a fake vendor.
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Harness from './BytePlusPrepaidHarness.vue'
import { createI18n } from 'vue-i18n'
import { apiClient } from '../../src/api/client'
import english from '../../src/i18n/locales/en/admin/byteplusPrepaid'
import chinese from '../../src/i18n/locales/zh/admin/byteplusPrepaid'
import '../../src/style.css'

apiClient.defaults.baseURL = '/api/v1'
apiClient.interceptors.request.clear()
apiClient.defaults.headers.common.Authorization = 'Bearer fixture-admin-not-production'

const app = createApp(Harness)
app.use(createPinia())
app.use(createI18n({ legacy: false, locale: 'en', messages: { en: { admin: english }, zh: { admin: chinese } } }))
app.mount('#app')
