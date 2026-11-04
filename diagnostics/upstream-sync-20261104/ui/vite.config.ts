import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({ plugins:[vue()], server:{host:'0.0.0.0', port:4173}, css:{postcss:'/app/frontend/postcss.config.js'} })
