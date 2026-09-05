import { createApp } from 'vue'
import App from './App.vue'
import { createRelayApi } from './api'
import './styles.css'

const root = document.getElementById('app')
const apiBase = root?.dataset.pluginApiBase || '/plugins/ai-relay/api'
createApp(App, { api: createRelayApi(apiBase) }).mount(root || '#app')
