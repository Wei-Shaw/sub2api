/**
 * Plugin SDK build entry for @sub2api/plugin-sdk.
 *
 * 输出 dist/plugin-sdk.js (单文件 ES bundle), 供后端 servePluginSharedAsset
 * 在浏览器侧通过 importmap (`@sub2api/plugin-sdk` → /api/v1/plugin-assets/__shared__/plugin-sdk.js)
 * 提供给 plugin frontend bundle 使用.
 *
 * Externals:
 *   - vue / vue-i18n 由 plugin importmap (host singleton) 共享, 不打入 SDK bundle.
 *   - 其余 (@tanstack/vue-virtual 等) 一律 inline, plugin 只需依赖 importmap 这两个 specifier.
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
      // vue / vue-i18n 由 host importmap 共享 (singleton); 其余 (@tanstack/vue-virtual 等) 全部 bundle 进 plugin-sdk.js, 避免 plugin 侧 bare specifier 解析失败.
      external: ['vue', 'vue-i18n'],
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
