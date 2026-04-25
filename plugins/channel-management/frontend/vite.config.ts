import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: 'channel-management',
    },
    rollupOptions: {
      external: ['vue', 'vue-router', 'vue-i18n', 'pinia', 'axios'],
      output: {
        globals: {
          vue: 'Vue',
          'vue-router': 'VueRouter',
          'vue-i18n': 'VueI18n',
          pinia: 'Pinia',
          axios: 'axios',
        },
      },
    },
  },
})
