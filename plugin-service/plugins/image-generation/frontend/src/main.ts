import { createApp } from 'vue'
import App from './App.vue'
import { createImageGenerationI18n } from './i18n'
import './styles/app.css'

createApp(App).use(createImageGenerationI18n()).mount('#app')
