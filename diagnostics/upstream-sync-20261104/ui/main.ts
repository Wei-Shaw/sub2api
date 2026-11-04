import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import App from './App.vue'
import en from '../src/core/i18n/locales/en/admin/accounts'
import zh from '../src/core/i18n/locales/zh/admin/accounts'
import '../src/core/themes/style.css'
const locale = new URLSearchParams(location.search).get('lang') || 'zh'
createApp(App).use(createI18n({ legacy:false, locale, messages:{en:{admin:en},zh:{admin:zh}} })).mount('#app')
