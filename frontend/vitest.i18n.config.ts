import { resolve } from 'node:path'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js'
    }
  },
  test: {
    environment: 'node',
    setupFiles: [],
    include: ['src/i18n/__tests__/localeKey{Completeness,Parity}.spec.ts'],
    exclude: ['node_modules', 'dist']
  }
})
