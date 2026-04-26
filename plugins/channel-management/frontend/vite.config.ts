/**
 * Plugin frontend bundle config (channel-management) — V2 SDK 改造版.
 *
 * V2 之前 (V1):
 *   - 通过 vite alias `@` 把 import 折回到 host frontend/src/, 让 plugin
 *     bundle 内联 host 共通组件
 *   - 自写 hostNodeModulesResolver / publicAssetStub rollup plugins 兜底
 *     transitive host deps 与 host public 资产
 *   - 弊端: bundle ~4.8MB, 内嵌大量 host 实现细节
 *
 * V2 改造:
 *   - 共通组件迁出 host, 由 @sub2api/plugin-sdk 提供, 通过 importmap 共享
 *   - plugin 不再 import host @/, 不需要 alias / fallback resolver / public stub
 *   - bundle 体积大幅缩减 (目标 < 1.5MB)
 *
 * Output:
 *   - dist/entry.js  — single-file ES bundle, default export { install(sdk) }
 *   - dist/entry.css — bundled CSS
 *
 * Externals (全部由 host importmap 提供):
 *   - vue / vue-router / vue-i18n / pinia / axios
 *   - @sub2api/plugin-sdk (host /api/v1/plugin-assets/__shared__/plugin-sdk.js)
 */
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  define: {
    __INTLIFY_JIT_COMPILATION__: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    cssCodeSplit: false,
    cssMinify: true,
    minify: 'esbuild',
    target: 'es2020',
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: () => 'entry.js',
    },
    rollupOptions: {
      external: [
        'vue',
        'vue-router',
        'vue-i18n',
        'pinia',
        'axios',
        '@sub2api/plugin-sdk',
      ],
      output: {
        inlineDynamicImports: true,
        assetFileNames: (assetInfo) => {
          if (assetInfo.name && assetInfo.name.endsWith('.css')) {
            return 'entry.css'
          }
          return 'assets/[name][extname]'
        },
      },
    },
  },
})
