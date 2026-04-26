/**
 * Plugin SDK build entry for @sub2api/plugin-sdk.
 *
 * 输出 dist/plugin-sdk.js (单文件 ES bundle), 供后端 servePluginSharedAsset
 * 在浏览器侧通过 importmap (`@sub2api/plugin-sdk` → /api/v1/plugin-assets/__shared__/plugin-sdk.js)
 * 提供给 plugin frontend bundle 使用.
 *
 * Externals:
 *   - vue / lucide-vue-next 由 plugin importmap (vue) + host 自身 (lucide) 提供,
 *     不打入 SDK bundle.
 *
 * emptyOutDir:false 防止覆盖 host frontend 的主 bundle.
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
    // host frontend 的主 build 输出在 ../backend/internal/web/dist (见 vite.config.ts).
    // SDK bundle 也写到同一目录, 让 embed FS 一并打包.
    outDir: '../backend/internal/web/dist',
    emptyOutDir: false,
    cssCodeSplit: false,
    cssMinify: true,
    minify: 'esbuild',
    target: 'es2020',
    lib: {
      entry: resolve(__dirname, 'packages/plugin-sdk/src/index.ts'),
      formats: ['es'],
      fileName: () => 'plugin-sdk.js',
    },
    rollupOptions: {
      // vue 通过 plugin importmap 共享; lucide-vue-next host 自己 bundle, plugin 不需要也能通过 host 全局命名空间获得 (本 V2 阶段未涉及).
      external: ['vue', 'vue-i18n', 'lucide-vue-next', '@tanstack/vue-virtual'],
      output: {
        inlineDynamicImports: true,
        assetFileNames: (assetInfo) => {
          if (assetInfo.name && assetInfo.name.endsWith('.css')) {
            return 'plugin-sdk.css'
          }
          return 'assets/[name][extname]'
        },
      },
    },
  },
})
