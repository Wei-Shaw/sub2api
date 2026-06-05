/**
 * Plugin SDK build entry for @sub2api/plugin-sdk.
 *
 * 输出 dist/plugin-sdk.js (单文件 ES bundle), 供后端 servePluginSharedAsset
 * 在浏览器侧通过 importmap (`@sub2api/plugin-sdk` → /api/v1/plugin-assets/__shared__/plugin-sdk.js)
 * 提供给 plugin frontend bundle 使用.
 *
 * Externals 必须与 backend `pluginImportMap` 暴露的 host singleton 严格一致:
 *   vue / vue-i18n / vue-router / pinia / axios
 *
 * 任何 importmap 暴露但 externals 没标的 specifier 都是「埋雷」 — SDK 后续若加入
 * 该 specifier 的值层 import, 会被 inline 进 bundle 形成第二份实例, host/plugin
 * 两侧 useRouter() / store / axios 拦截器全部失配. 保持 5 个 singleton 全 external,
 * 让契约面对齐, 而不是依赖「当前 SDK 没用到所以没爆」的运气.
 *
 * 不在此列的库 (@tanstack/vue-virtual 等) 一律 inline, plugin 不需要额外 importmap entry.
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
    // SDK bundle 在浏览器侧执行 (importmap 提供给 plugin), 必须把 Node 全局 process.env
    // 静态替换成字面量, 否则 "process is not defined" 会在 plugin install 时直接 throw.
    // vue / vue-i18n / pinia 等的 source 在生产模式下都包含 `process.env.NODE_ENV` 判断,
    // 它们的 host 主 bundle 由 vite build 时被替换, 但 SDK 这条 lib build 路径不替换.
    'process.env.NODE_ENV': JSON.stringify('production'),
    // bare `process.env` (没有进一步访问) 用空对象兜底, 防止运行时 ReferenceError.
    'process.env': '{}',
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
      external: ['vue', 'vue-i18n', 'vue-router', 'pinia', 'axios'],
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
