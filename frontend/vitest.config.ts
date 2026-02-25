import { defineConfig REDACTED from 'vitest/config'
import { resolve REDACTED from 'path'

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js'
    REDACTED
  REDACTED,
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['src/**/*.{test,specREDACTED.{js,ts,jsx,tsxREDACTED'],
    exclude: ['node_modules', 'dist'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*.{js,ts,vueREDACTED'],
      exclude: [
        'node_modules',
        'src/**/*.d.ts',
        'src/**/*.spec.ts',
        'src/**/*.test.ts',
        'src/main.ts'
      ],
      thresholds: {
        global: {
          statements: 80,
          branches: 80,
          functions: 80,
          lines: 80
        REDACTED
      REDACTED
    REDACTED
  REDACTED
REDACTED)
