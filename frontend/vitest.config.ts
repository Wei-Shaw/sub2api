import { defineConfig } from 'vitest/config'
import { resolve } from 'path'
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
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/__tests__/setup.ts'],
    include: ['src/**/*.{test,spec}.{js,ts,jsx,tsx}'],
    exclude: ['node_modules', 'dist'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*.{js,ts,vue}'],
      exclude: [
        'node_modules',
        'src/**/*.d.ts',
        'src/**/*.spec.ts',
        'src/**/*.test.ts',
        'src/main.ts'
      ],
      // Vitest 2 reads threshold keys at the top level of `thresholds`. The
      // previous shape nested them under `global`, which Vitest 2 treats as a
      // per-glob threshold group for files matching the literal glob "global" —
      // zero files matched, so the gate passed at any coverage. Measured floor
      // when this was fixed: 69.34 statements / 70 branches / 43.94 functions /
      // 69.34 lines. The numbers below sit just under measured so ordinary
      // churn does not flap the build; raise them at the end of each tier.
      thresholds: {
        statements: 69,
        branches: 69,
        functions: 43,
        lines: 69
      }
    }
  }
})
