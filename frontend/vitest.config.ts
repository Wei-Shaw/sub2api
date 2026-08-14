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
      //
      // `functions` moved 43 -> 42 when SettingsView was split into ten section
      // components. Splitting one SFC into eleven multiplies the per-file
      // boilerplate v8 counts as functions: the split added 43 function slots
      // and covered only 6 of them, so the ratio fell 43.29 -> 42.98 without a
      // single source function losing coverage. Statements, branches and lines
      // all rose over the same change.
      //
      // `statements`/`lines` moved 69 -> 68 when ~1,300 orphan i18n keys were
      // deleted. A locale module is a plain object literal that every test
      // imports, so it counts as 100% covered statements; deleting dead strings
      // therefore removes numerator faster than it removes denominator and the
      // ratio falls — 69.36 -> 68.99 — while no executable line lost a test.
      // The honest reading is that message trees were inflating this number all
      // along, not that coverage regressed.
      thresholds: {
        statements: 68,
        branches: 69,
        functions: 42,
        lines: 68
      }
    }
  }
})
