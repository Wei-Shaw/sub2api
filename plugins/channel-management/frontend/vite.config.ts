/**
 * Plugin frontend bundle config (channel-management).
 *
 * Output:
 *   - dist/entry.js  — single-file ES bundle, default export { install(sdk) }
 *   - dist/entry.css — bundled CSS (via cssCodeSplit:false + assetFileNames)
 *
 * Externals:
 *   - vue / vue-router / pinia / vue-i18n / axios are externalised. Host
 *     injects an <script type="importmap"> mapping these specifiers to
 *     /api/v1/plugin-assets/__shared__/<name>.js so the browser resolves
 *     them at runtime to the host's already-loaded singletons.
 *
 * Alias / resolve strategy:
 *   - `@` resolves into the host frontend src tree. This lets the plugin
 *     re-use host's common components (DataTable, Select, ...) and stores
 *     by inlining their compiled output into dist/entry.js.
 *   - Vite's default node_modules lookup walks up from this directory and
 *     would miss the host's `frontend/node_modules`. We add `HOST_FRONTEND_NM`
 *     to `resolve.modules` (via custom alias entries below) so transitive
 *     deps like @tanstack/vue-virtual or @vueuse/core (used by host common
 *     components but not declared in plugin's package.json) resolve to the
 *     host install.
 *
 * Trade-off: bundle is larger than ideal because it ships transitive host
 * dependencies. MVP-first; T8 (optional) can move host common components
 * into a separately-shared bundle.
 */
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

const HOST_FRONTEND_SRC = resolve(__dirname, '../../../frontend/src')
const HOST_FRONTEND_NODE_MODULES = resolve(__dirname, '../../../frontend/node_modules')

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: [
      // host frontend src 别名 (与 host vite.config.ts 一致)
      { find: '@', replacement: HOST_FRONTEND_SRC },
    ],
    // 让 plugin build 命中 host frontend 的 node_modules. Vite 默认 root 是
    // __dirname (plugin/frontend/), 默认只 walk 到 plugin/frontend/node_modules
    // 与 worktree root /node_modules. 显式追加 host frontend/node_modules,
    // 这样 host 共通组件携带的 @tanstack/vue-virtual / @vueuse/core 等依赖
    // 都能被解析到 host 已安装的版本.
    preserveSymlinks: false,
  },
  // Vite 用 cwd 推断 node_modules 链, root 必须保持 plugin 目录 (我们的
  // entry/源文件在这里). 通过 server/ssr 的 resolve.dedupe + custom resolver
  // 来添加 host node_modules.
  optimizeDeps: {
    // build 阶段不走 optimizeDeps (那是 dev mode 的预构建); 此处只是为了让
    // dev 调试 (vite dev) 时能找到依赖.
    esbuildOptions: {
      conditions: ['module', 'browser', 'default'],
    },
  },
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
      external: ['vue', 'vue-router', 'vue-i18n', 'pinia', 'axios'],
      // 自定义 resolveId, 失败时去 host node_modules 兜底.
      plugins: [hostNodeModulesResolver(HOST_FRONTEND_NODE_MODULES), publicAssetStub()],
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

/**
 * Rollup plugin: 把 plugin 自身 node_modules 找不到的 specifier 兜底解析到
 * host frontend/node_modules. 只匹配 bare specifier (不以 . 或 / 起头),
 * 不影响相对/绝对路径或 vite 内部协议 (\\0 \\\\ virtual:).
 *
 * 思路: 直接在 host 的 node_modules 里查 package.json + main/module/exports,
 * 太复杂. 这里偷懒, 通过 require.resolve.paths 让 Node 在 host 路径里找,
 * 命中后用绝对文件路径返回给 rollup.
 */
/**
 * Rollup plugin: stub host public-asset references (e.g. WechatServiceButton.vue
 * 引用 `/wechat-qr.jpg`). Plugin bundle 不携带 host public/, 这些 URL 在运行时
 * 由 host 服务器同源提供, 浏览器最终能加载. 此处把 rollup resolve 阶段的引用
 * 替换为空 data: URL, 让 build 通过.
 */
function publicAssetStub() {
  const STUB_ID = '\0sub2api-host-public-stub'
  return {
    name: 'sub2api-host-public-stub',
    resolveId(source: string) {
      if (!source) return null
      // 只匹配以 / 起头且看起来像 host public 资产 (.jpg/.png/.svg 等)
      if (
        source.startsWith('/') &&
        !source.startsWith('//') &&
        /\.(jpg|jpeg|png|svg|gif|webp|ico)$/i.test(source)
      ) {
        return STUB_ID
      }
      return null
    },
    load(id: string) {
      if (id === STUB_ID) {
        // 输出运行时仍然解析为 host 同源相对路径的字符串. plugin runtime 与
        // host 同源 (/api/v1/plugin-assets/<plugin>/dist/entry.js), 所以
        // <img src="/wechat-qr.jpg"> 在浏览器最终请求 /wechat-qr.jpg 由 host
        // 服务器响应.
        return 'export default ""'
      }
      return null
    },
  }
}

function hostNodeModulesResolver(hostNm: string) {
  return {
    name: 'sub2api-host-nm-fallback',
    enforce: 'post' as const,
    async resolveId(source: string, importer: string | undefined) {
      // bare specifier 才走兜底
      if (!source || source.startsWith('.') || source.startsWith('/') || source.includes('\0')) {
        return null
      }
      if (source.startsWith('@/') || source.startsWith('virtual:')) {
        return null
      }
      // externalised 的不动
      const externals = ['vue', 'vue-router', 'vue-i18n', 'pinia', 'axios']
      if (externals.includes(source) || externals.some((e) => source.startsWith(e + '/'))) {
        return null
      }
      try {
        // Node 内置 require.resolve 在 ESM 下不直接可用, 我们用 import.meta.resolve
        // 或自写 lookup. 这里用最朴素的 createRequire.
        const { createRequire } = await import('module')
        const req = createRequire(hostNm + '/_dummy.js')
        const resolved = req.resolve(source)
        return resolved
      } catch {
        return null
      }
    },
  }
}
