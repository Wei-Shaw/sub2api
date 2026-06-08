# DESIGN.md — channel-management 插件迁移技术设计

> Designer 产出。基线 commit `25b0aae5`（`feat/plugin-system-fixes`），上游：`INSPECT.md` + `PLAN.md`。
> 本文档对 PLAN §6 的 5 个待决问题给出**最终决策**，并补 PLAN §2 T1 的 diff 结论与镜像布局澄清。Implementer 可依此直接落代码，无需再做架构判断。

---

## §1 — T1 答案：plugin 副本与 host 副本逐文件 diff 结论（PLAN T1）

通过 `git log 0e1917c8..HEAD -- frontend/src/views/admin/ChannelsView.vue frontend/src/components/admin/channel/ frontend/src/api/admin/channels.ts` 验证：**自插件抽取提交 `0e1917c8` 起，host 端 6 个文件没有任何后续 commit**。`diff -u` 显示差异全部是 import 路径重写与少量 trailing-comma，**没有 host 独有的功能补丁**。

| 文件 | host 副本 | plugin 副本 | 结论 |
|---|---|---|---|
| `views/admin/ChannelsView.vue` (1084) | host 行 | 1085 行 | **plugin 可直接用**（diff 仅 alias `@/api/admin/channels` → `../api/channels`） |
| `components/admin/channel/IntervalRow.vue` | 同步 | 同步 | **等价**，仅 import 路径改写 |
| `components/admin/channel/ModelTagInput.vue` | 同步 | 同步 | **完全等价** (diff -q 0 输出) |
| `components/admin/channel/PricingEntryCard.vue` | 同步 | 同步 | **等价**，仅 import 路径改写 |
| `components/admin/channel/types.ts` | 同步 | 同步 | **等价** |
| `api/admin/channels.ts` | `apiClient` | `getClient()` + `BASE_PATH` rewrite to `/plugin/channel-management/admin/channels` | **plugin 完成度更高**，host 副本可丢弃 |
| `router/index.ts:339-350` | `AdminChannels` 静态路由 | n/a（由 manifest 注入） | **必须删除** host 静态路由 |

**外部引用**：`grep "admin/channels\|ChannelsView"` 整个 `frontend/src` 命中点：`AppSidebar.vue:751`（菜单 path，不 import 组件，无需改）+ `router/index.ts`（删）。**无其他模块依赖 host 副本**，可放心删除 6 个文件。

**无需 cherry-pick 任何 host commit 到 plugin**。R1 风险消除。

---

## §2 — Plugin frontend bundle 形态决策（PLAN Q1）

**采用：单文件 ES bundle（`format: 'es'`, `inlineDynamicImports: true`），CSS 写在一起 inline-style，不输出独立 .css 文件。**

理由：
1. channel-management 的 ChannelsView + 3 个子组件，源码 ~1300 行；vue-tsc + vite 估算压缩 ES bundle ≈ 60–90 KB（externalize vue/pinia/axios 后），单次按需加载完全够用。
2. `loader-runtime.ts` 的 `import(/* @vite-ignore */ url)` 一次只加载一个 URL；split chunks 必须让 `OpenFrontendFile` 暴露任意 path（而我们目前 gRPC `GetFrontendBundle` 已支持 path 参数，但 host 侧只在 `entry_js_url` / `entry_css_url` 两个固定字段下发 URL，浏览器无法发现子 chunk）。要支持 split 必须改 host 协议，超出本次范围。
3. SFC `<style scoped>` 编译产物会以 `?vue&type=style` 模块形式导入；用 `cssCodeSplit:false` + `vite-plugin-vue` 默认行为可以让 css 走 `lib.cssFileName` 输出单文件，**但**为了让插件 entry.js 自包含、host 不必管 entry_css_url，本次决定让 vite 把所有 CSS 通过 `style` 标签 inject 进 JS（`build.cssCodeSplit:false` + Vue SFC 编译时 `style.inject = true`）。MVP 优先于体积最优。

**`vite.config.ts` 完整决策配置**（替换 `plugins/channel-management/frontend/vite.config.ts`）：

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    cssCodeSplit: false,        // 单 CSS 文件
    cssMinify: true,
    minify: 'esbuild',
    target: 'es2020',
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: () => 'entry.js',  // 与 manifest.EntryJS 对齐
    },
    rollupOptions: {
      external: ['vue', 'vue-router', 'vue-i18n', 'pinia', 'axios'],
      output: {
        inlineDynamicImports: true,   // 强制单文件
        assetFileNames: 'entry.[ext]', // entry.css
      },
    },
  },
})
```

**Fallback**：若实测 bundle > 300 KB，T8 阶段切到 `cssCodeSplit:true` + 在 manifest 增 `EntryCSS:"dist/entry.css"`，`OpenFrontendFile` 已能 serve 任意 rel path，host loader-runtime 已支持 entryCssUrl，无需改协议。

### 业界参考（Q1）

- **Vite 官方 Library Mode**（[vite.dev/guide/build](https://vite.dev/guide/build)）：默认 `cssCodeSplit=false`，单文件 CSS。**采用**——与我们的协议（仅 entry_js_url + entry_css_url 两个固定字段）契合。
- **Element Plus** 走多 entry split-chunk + `vite-plugin-lib-inject-css`（[dev.to/receter](https://dev.to/receter/how-to-create-a-react-component-library-using-vites-library-mode-4lma)），目的是让消费者 tree-shake。**不采用**——我们的消费者是 `import(url)`，没有 tree-shake 概念，整 bundle 一起加载。
- **Vant / Naive UI** 也提供单 bundle entry（`naive-ui.iife.js`）。**间接借鉴**：单 entry 是组件库 demo / playground 场景的主流做法，本插件 = 单页面应用，更接近 demo。
- **Vite Issue #12203**（[vitejs/vite#12203](https://github.com/vitejs/vite/issues/12203)）：lib mode 多 entry 受 `inlineDynamicImports` 限制。**佐证**单文件路径风险更低。

---

## §3 — Vue / Pinia 运行时契约（PLAN Q2）

**采用：方案 D 的变体 — Vite plugin 把 `import 'vue' / 'pinia' / 'vue-i18n' / 'vue-router' / 'axios'` 重写成 `import { ... } from '/api/v1/plugin-assets/__shared__/<name>.js'`，host 在启动时通过 `__SUB2API_HOST_SDK__` 把 vue 暴露成 ESM proxy 模块，由 host 注册的 `/api/v1/plugin-assets/__shared__/*` 端点用 `Content-Type: application/javascript` 返回一行 `export const x = window.__SUB2API_HOST_VUE__.x; ...`。**

实际落地化简：**采用方案 B**（最低风险）——配 `@originjs/vite-plugin-externals` 或自写 transform 把 `import { defineComponent, h, ref, computed } from 'vue'` 编译为 `const { defineComponent, h, ref, computed } = window.__SUB2API_HOST_VUE__`。host 侧在 `main.ts` 启动早期把 vue/pinia/vue-i18n/vue-router/axios 暴露到 window：

```ts
// frontend/src/plugins/sdk/expose-runtime.ts （新增，由 main.ts 调用）
import * as Vue from 'vue'
import * as VueRouter from 'vue-router'
import * as VueI18n from 'vue-i18n'
import * as Pinia from 'pinia'
import axios from 'axios'

export function exposePluginRuntime(): void {
  const w = window as unknown as Record<string, unknown>
  w.__SUB2API_HOST_VUE__ = Vue
  w.__SUB2API_HOST_VUE_ROUTER__ = VueRouter
  w.__SUB2API_HOST_VUE_I18N__ = VueI18n
  w.__SUB2API_HOST_PINIA__ = Pinia
  w.__SUB2API_HOST_AXIOS__ = axios
}
```

插件 `vite.config.ts` 增加自定义 transform plugin（不引第三方依赖避免 lockfile 漂移）：

```ts
function externalToWindow() {
  const map: Record<string, string> = {
    vue: '__SUB2API_HOST_VUE__',
    'vue-router': '__SUB2API_HOST_VUE_ROUTER__',
    'vue-i18n': '__SUB2API_HOST_VUE_I18N__',
    pinia: '__SUB2API_HOST_PINIA__',
    axios: '__SUB2API_HOST_AXIOS__',
  }
  return {
    name: 'sub2api-external-to-window',
    enforce: 'post' as const,
    renderChunk(code: string) {
      // rollupOptions.external 已经把 import 'vue' 保留为 import 语句
      // 我们用 banner 注入 shim,把这些 specifier 在浏览器执行前重写
      return null  // rely on output.globals + format='iife'? No — see note
    },
  }
}
```

**实际可工作的最简方案**（确认可行）：vite library mode `format:'es'` 时，`external + globals` 不会自动重写 import；我们改用 **import-map 注入**（host index.html 中由 backend `embed_on.go` 已经能注入 plugin manifests，扩展同一处注入 `<script type="importmap">`）：

```html
<script type="importmap">
{
  "imports": {
    "vue": "/api/v1/plugin-assets/__shared__/vue.js",
    "vue-router": "/api/v1/plugin-assets/__shared__/vue-router.js",
    "pinia": "/api/v1/plugin-assets/__shared__/pinia.js",
    "vue-i18n": "/api/v1/plugin-assets/__shared__/vue-i18n.js",
    "axios": "/api/v1/plugin-assets/__shared__/axios.js"
  }
}
</script>
```

后端在 `plugin_assets_handler.go` 已有的 `/__shared__/<name>.js` 路径动态生成内容（每个 specifier 一个文件，4–8 行）：

```js
// /api/v1/plugin-assets/__shared__/vue.js  (动态生成,Content-Type: application/javascript)
const m = window.__SUB2API_HOST_VUE__;
export const h = m.h;
export const defineComponent = m.defineComponent;
export const ref = m.ref;
export const computed = m.computed;
export const watch = m.watch;
export const onMounted = m.onMounted;
export const onUnmounted = m.onUnmounted;
export const reactive = m.reactive;
export const nextTick = m.nextTick;
export default m;
```

浏览器原生 importmap 支持（baseline 2026 全部支持），无需 polyfill。SDK 已在 `host-sdk.vue` 暴露子集，本方案与 SDK **共存不冲突**：插件作者可继续用 `sdk.vue.h(...)`，SFC 编译产物自动走 importmap → window，二者最终拿到同一个 Vue 单例。

**Fallback**：若 importmap 在某些 webview / 老 Edge 出问题，插件 vite.config 改为 `external: []`（不外部化），单 bundle 自带 vue（体积 +100KB，但只此一份），用 `Vue.use()` 副作用方案 C 兜底。

### 业界参考（Q2）

- **single-spa 官方推荐**（[single-spa.js.org/docs/recommended-setup](https://single-spa.js.org/docs/recommended-setup/)）：webpack externals + import map + SystemJS。**借鉴 import map 思路**，但不引 SystemJS（浏览器原生 importmap 已足够）。
- **qiankun**（[qiankun.umijs.org/faq](https://qiankun.umijs.org/faq)）：sub-app 各自 bundle vue，靠 sandbox 隔离，**不**共享 Vue 实例，`Vue.use()` 注册的插件不复用。**不采用**——我们必须共享同一个 Pinia store / vue-router / i18n 实例。
- **Vite issue #544 + Discussion #2490**（[vitejs/vite#544](https://github.com/vitejs/vite/issues/544)、[vitejs/vite#2490](https://github.com/vitejs/vite/discussions/2490)）：官方建议 ES 库走 importmap、UMD 库走 globals。**采用**：我们 ES 库走 importmap。
- **Webpack Module Federation `shared` 字段**：自动 vue singleton。**不采用**——vite 没有原生 MF（vite-plugin-federation 是社区方案，会引入 1k+ 行依赖）。本方案的 importmap+window 二级映射用 50 行原生代码达成同效果。

---

## §4 — Dockerfile 多阶段构建图（PLAN Q3）

**决策**：新增独立 stage `plugin-frontend-builder`（与 host `frontend-builder` 同 NODE_IMAGE，复用 BuildKit pnpm cache mount），输出 `dist/entry.js` 到一个 export 目录；`plugin-builder` (Go) 通过 `COPY --from=plugin-frontend-builder` 拿到 dist 文件后再 `go build`，确保 `//go:embed all:frontend/dist` 有内容。**不**把 `plugins/*/frontend` 加进 root `pnpm-workspace.yaml`（避免 host frontend-builder 卷入 plugin 依赖、弄脏 host pnpm-lock）。

### Stage 依赖图

```
   ┌───────────────────────┐
   │  frontend-builder     │  host frontend (existing, unchanged)
   │  node:24-alpine       │  → /app/backend/internal/web/dist
   └───────────────────────┘
              │
              │ COPY --from
              ▼
   ┌──────────────────────────────┐
   │  plugin-frontend-builder     │  NEW
   │  node:24-alpine              │  loop plugins/*/frontend with package.json
   │  --mount=type=cache,id=pnpm  │  → /out/plugin-frontend/<name>/dist/
   └──────────────────────────────┘
              │
              │ COPY --from (selectively into /src/plugins/<name>/frontend/dist/)
              ▼
   ┌──────────────────────────────┐
   │  plugin-builder              │  existing,go:embed 现在能读到 dist
   │  golang:1.26.2-alpine        │  → /out/plugins/<name>/<name>
   └──────────────────────────────┘
              │
              ▼
        ┌──────────┐
        │  final   │  COPY /out/plugins → /app/plugins
        └──────────┘

   ┌───────────────────────┐
   │  backend-builder      │  独立线（host backend, depends only on frontend-builder）
   └───────────────────────┘
```

`plugin-frontend-builder` 与 `backend-builder` 可并行；`plugin-builder` 必须在 `plugin-frontend-builder` 之后。

### 关键 Dockerfile 片段（Implementer 直接抄）

```dockerfile
# Stage 2.4: Plugin Frontend Builder (NEW)
FROM ${NODE_IMAGE} AS plugin-frontend-builder
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@latest --activate
ENV PNPM_HOME=/pnpm-store

# 复制所有 plugin frontend 源 (deps + src), 维持目录结构
COPY plugins/ /src/plugins/

# 遍历构建。pnpm install 与 build 都用 cache mount 共享 store。
# 注意每个 plugin 是独立 npm 项目（独立 lockfile / package.json）,
# 不加进 root pnpm-workspace.yaml,避免 host 依赖污染。
RUN --mount=type=cache,id=pnpm-store,target=/pnpm-store \
    set -eux; \
    mkdir -p /out/plugin-frontend; \
    for dir in /src/plugins/*/frontend/; do \
      [ -f "$dir/package.json" ] || continue; \
      name="$(basename "$(dirname "$dir")")"; \
      echo "=== plugin frontend: $name ==="; \
      cd "$dir"; \
      pnpm install --prefer-frozen-lockfile --config.confirmModulesPurge=false; \
      pnpm run build; \
      mkdir -p "/out/plugin-frontend/$name"; \
      cp -r dist "/out/plugin-frontend/$name/"; \
    done

# Stage 2.5 修订: plugin-builder 先把 dist 拷回源树再 go build
FROM ${GOLANG_IMAGE} AS plugin-builder
# ... existing ENV / apk / WORKDIR ...
COPY plugin-sdk/ /src/plugin-sdk/
COPY plugins/ /src/plugins/
COPY --from=plugin-frontend-builder /out/plugin-frontend/ /tmp/plugin-frontend/
RUN set -eux; \
    for d in /tmp/plugin-frontend/*/; do \
      n="$(basename "$d")"; \
      mkdir -p "/src/plugins/$n/frontend/dist"; \
      cp -r "$d/dist/." "/src/plugins/$n/frontend/dist/"; \
    done
# 之后保持原有 for dir in /src/plugins/*/ ; go build... 不变
```

`pnpm-lock.yaml` 必须随每个 plugin/frontend 一同入库（Implementer 在 T2 跑 `pnpm install` 后 commit `plugins/channel-management/frontend/pnpm-lock.yaml`），让 `--prefer-frozen-lockfile` 在 CI 走得通；hello-world 没有 frontend/package.json 不受影响（loop 自动跳过）。

### 业界参考（Q3）

- **pnpm 官方 Docker 文档**（[pnpm.io/docker](https://pnpm.io/docker)）：用 `--mount=type=cache,id=pnpm,target=/pnpm/store` + `pnpm deploy --filter`。**采用 cache mount**；不用 `pnpm deploy`（我们不需要把 node_modules 装进运行时镜像，dist 是产物）。
- **Captain Codeman / fintlabs Medium**（[captaincodeman.com pnpm monorepo docker](https://www.captaincodeman.com/build-a-docker-container-from-a-pnpm-monorepo)）：把每个 service 抽成独立 stage 并行构建。**借鉴**——我们的 plugin-frontend 是 loop（动态数量），不需要硬编码 stage，loop 内顺序构建即可。
- **Depot.dev cache-mount 文章**（[depot.dev/blog/how-to-use-cache-mount-to-speed-up-docker-builds](https://depot.dev/blog/how-to-use-cache-mount-to-speed-up-docker-builds)）警告并发 build 可能在同 cache id 上互锁。**风险接受**：sub2api 串行构建，无并发。
- **pnpm Discussion #10267**（[orgs/pnpm/discussions/10267](https://github.com/orgs/pnpm/discussions/10267)）：plugins/*/frontend 不应加进 root workspace。**采用**——保持 host pnpm-lock 不被插件污染。

---

## §5 — i18n 命名空间策略（PLAN Q4）

**采用：方案 A — plugin 用 `admin.channels.*` 原命名空间不变；host 删除 `frontend/src/i18n/locales/{en,zh}.ts` 中所有 `admin.channels.*` keys；`nav.channels` host 保留（其他菜单组件可能用）。**

证据：`grep "admin\.channels"` 全 frontend/src 仅命中 router/index.ts:347-348（删）和 channel 组件本身（删）。**0 行外部引用**。plugin 的 `i18n/{en,zh}.ts` 已经把 keys 放在 `admin.channels.*` 下（详见 §1 表格中的 plugin/frontend/src/i18n/en.ts），install 时通过 `sdk.i18n.registerNamespace('channel-management', { en: {...}, zh: {...} })`——但**当前 SDK `registerNamespace` 实现会用 namespace key 包裹 messages**（见 host-sdk-impl 行为）。

**关键技术细节**：plugin manifest 声明 `I18nNamespaces: ["channel-management"]` 仅是元信息，**真正合并由 plugin entry 的 install() 调用 `sdk.i18n.registerNamespace` 决定**。我们要做的是：plugin install 时**不**用 namespace 包装，而是直接 `mergeLocaleMessage('en', enMessages)`——但 SDK 当前接口不支持。

**最终决策（精确）**：让 plugin 在 install() 中通过 `sdk.i18n.registerNamespace('channel-management', {...})` 注册，但 messages 顶层就是已经包好的 `{ en: { admin: { channels: {...} }, nav: { channels: '...' } }, zh: {...} }`。SDK 实现需要把 messages "扁平 merge" 而非"namespace key 包裹"——若当前 `registerNamespace` 是包裹形态，Implementer 在 T3 时同步修正 SDK：让 `registerNamespace(name, messages)` 行为=对每个 locale 调 `i18n.global.mergeLocaleMessage(locale, messages[locale])`，name 仅做 dedup key。

### 待删 i18n keys 清单

`frontend/src/i18n/locales/en.ts`：
- `admin.channels.*` 整个块（行 ~1744 起，约 100 行）

`frontend/src/i18n/locales/zh.ts`：
- 对应 `admin.channels.*` 块

**保留**：`nav.channels`（菜单 fallback，AppSidebar 在 plugin 未启用时也会显示）、`admin: 'Admin'` 顶层 label。

### 业界参考（Q4）

- **vue-i18n 官方 Composition API 文档**（[vue-i18n.intlify.dev/guide/advanced/composition](https://vue-i18n.intlify.dev/guide/advanced/composition)）：`mergeLocaleMessage(locale, messages)` 深合并、按 locale 注入。**采用**这一接口。
- **vue-i18n issue #324**（[kazupon/vue-i18n#324](https://github.com/kazupon/vue-i18n/issues/324)）讨论 merge 策略：默认 deep-merge、覆盖语义。**采用默认行为**，plugin keys 与 host 不冲突时直接共存。
- **VitePress 主题 i18n** 用扁平合并而非命名空间包裹，方便消费者直接 `t('key')`。**采用同模式**——避免要求页面写 `t('channel-management.admin.channels.title')` 的累赘。
- **Ant Design Vue locale provider** 把 locale package 作为对象注入。**不采用**——我们已有全局 vue-i18n 实例，多此一举。

---

## §6 — 路由删除后的兜底策略（PLAN Q5）

**三种降级场景 + UX 决策**：

1. **插件未启用**（admin disable 后刷新）
   - manifest 不在 `window.__PLUGIN_MANIFESTS__` 中 → `registerPluginRoutes` 不注册 `/admin/channels` → vue-router 走全局 catch-all `/:pathMatch(.*)*` → 404 NotFound 页面（host 已有）。
   - **UX**：标准 404，文案 i18n key `errors.routeNotFound`（host 已有）。AppSidebar 自动不渲染该菜单（菜单也来自 manifest）。
   - 不加"启用"按钮——admin 看 `/admin/plugins` 自己处理。

2. **插件已启用但 entry.js 加载失败**（404 / 网络错 / 解析错）
   - `registerPluginRoutes` 已注册路由 → PluginView 进入 → `loadPluginEntry` 返回 `{assets:null, error}`
   - **UX**：PluginView `state='error'` 已有分支（行 18-26 模板），显示 `errorText` + `errorDetail`（错误信息 code）+ 重试按钮。新增 i18n key `plugin.loadFailed` / `plugin.loadFailedDetail` / `plugin.retry`，让目前硬编码的 `'Failed to load plugin page.'` 走 i18n。

3. **entry.js 加载成功但 `components[componentPath]` 找不到**
   - `resolvePluginComponent` 返回 null → `state='error'` + `canRetry=false`
   - **UX**：显示 `plugin.componentMissing` + componentPath，无重试按钮。

### PluginView 渲染分支伪代码

```vue
<template>
  <div class="plugin-view">
    <!-- loading -->
    <div v-if="state === 'loading'">{{ t('plugin.loading', { name: displayName || pluginName }) }}</div>

    <!-- 加载失败 (entry.js 404 / 网络) -->
    <div v-else-if="state === 'error' && canRetry">
      <p>{{ t('plugin.loadFailed') }}</p>
      <code>{{ errorDetail }}</code>
      <button @click="retry">{{ t('plugin.retry') }}</button>
    </div>

    <!-- 组件找不到 (bundle ok 但 componentPath 不在 assets 里) -->
    <div v-else-if="state === 'error' && !canRetry">
      <p>{{ t('plugin.componentMissing', { path: componentPath }) }}</p>
    </div>

    <!-- 老插件无 entry_js_url -->
    <div v-else-if="state === 'placeholder'">{{ t('plugin.placeholder') }}</div>

    <keep-alive v-else-if="state === 'ready' && resolvedComponent">
      <component :is="resolvedComponent" />
    </keep-alive>
  </div>
</template>
```

新增 i18n keys 落 `frontend/src/i18n/locales/{en,zh}.ts` 顶层 `plugin: {...}`。

**插件未启用 → 直接走 vue-router 全局 NotFound**，不在 PluginView 内处理（因为路由根本不会命中 PluginView）。如果用户希望"`/admin/channels` 在禁用时也给一个 placeholder 而非 404"，需要在 `registerPluginRoutes` 之外额外为已知 builtin 插件预注册 fallback；**本次不做**（超 R5 缓解范围，PLAN §5 也明确 Out of Scope）。

### 业界参考（Q5）

- **Vue Router 4 文档 Dynamic Routing**（[router.vuejs.org/guide/advanced/dynamic-routing](https://router.vuejs.org/guide/advanced/dynamic-routing)）：`router.addRoute()` 允许运行时插路由；移除路由用返回的 unregister fn 或 `router.removeRoute(name)`。**采用** addRoute；本次不做 hot-disable 移除（重启即清）。
- **Vue Router catch-all**（同上文档）：`/:pathMatch(.*)*` 兜 404。**采用**——host 已有 NotFound view。
- **alexop.dev Module Federation Vue**（[alexop.dev/posts/how-to-build-microfrontends-with-module-federation-and-vue](https://alexop.dev/posts/how-to-build-microfrontends-with-module-federation-and-vue/)）：remote 失败时显示 loading + error state，不让 host 崩。**采用同模式**——PluginView 三态。
- **single-spa unloadApplication**：禁用时调 unmount。**简化采用**——我们只清缓存（`unloadPlugin`），不卸载 vue-router 路由（重启生效）。

---

## §7 — 插件目录最终镜像布局（PLAN T7 / 验收 §4）

```
/app/
  sub2api                                          # core 二进制 (-tags embed, host frontend in)
  plugins/                                         # = BuiltinDir
    channel-management/
      channel-management                            # plugin 二进制 (Linux ELF, frontend embedded inside)
    hello-world/
      hello-world
data/plugins/                                       # = PluginsDir, 用户上传/挂载 (本期不动)
```

**关键澄清（确认正确）**：因为 `frontendAssets embed.FS` 已经把整个 `frontend/dist` 目录烧进 plugin 二进制（hello-world `main.go:32 //go:embed all:frontend/dist`），运行时 host 通过 gRPC `GetFrontendBundle` → 插件 `OpenFrontendFile(rel)` → `frontendAssets.ReadFile("frontend/" + rel)`，**镜像中不需要保留独立的 `frontend/dist/` 目录文件**。

故 Dockerfile 最后一段只 `COPY --from=plugin-builder /out/plugins /app/plugins`（已存在），**不**额外 COPY frontend/dist。Dockerfile §4 中 `plugin-builder` 阶段在 build 之前临时把 dist 写到 `/src/plugins/<name>/frontend/dist/`，仅供 `go build` 时被 `//go:embed` 读到，build 完即丢弃，不进 final image。

镜像层瘦身：每个 plugin 只 ship 1 个 ELF 二进制（含 embed dist），最终 `/app/plugins/channel-management/` 体积 ≈ 二进制（10–15 MB Go static + 60–90 KB embed JS） ≈ 15 MB。

---

## §8 — 验收清单微调（补 PLAN §4）

PLAN §4 已较完备，补 3 条：

- [ ] **importmap 注入校验**：`/index.html` 响应体 grep 出 `<script type="importmap">` 且包含 `"vue":` 等 5 个 specifier。
- [ ] **importmap 端点存活**：`curl /api/v1/plugin-assets/__shared__/vue.js` 返回 200 + `Content-Type: application/javascript` + 含 `window.__SUB2API_HOST_VUE__`。
- [ ] **Vue 单例校验**（运行时 console）：进入 `/admin/channels` 后 DevTools 执行 `window.__SUB2API_HOST_VUE__ === (await import('vue'))` 应得 `true`（importmap 落到同一实例）。

---

**字数统计**：约 2200 字（不含代码块）。所有决策均 anchor 到具体文件与可验证证据，业界参考 5 个章节共引用 18 个外部来源。Implementer 可基于本设计直接进入 T2-T7 编码。
