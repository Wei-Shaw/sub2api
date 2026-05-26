/**
 * Host SDK contract exposed to plugin frontend bundles.
 *
 * 设计目标：
 *   - 插件 entry.js 在运行时拿到这个对象后，可以使用 host 的主题、i18n、通知、
 *     路由、用户身份与 HTTP 客户端，而不需要自己维护一份 Pinia/vue-i18n。
 *   - 所有 reactive 状态（主题模式、当前 locale 等）以 Ref 形式暴露，
 *     插件 watch/effect 可以自动响应 host 的变化。
 *   - 通知与 HTTP 客户端走 host 已有实现，避免插件各自实现 toast/请求拦截器。
 *
 * 兼容性：
 *   - `version` 字段表示 SDK 协议版本，插件可以据此做 feature gating。
 *   - 后续新增能力请追加字段（小版本），删改字段必须升大版本。
 *
 * V2 (Shadow DOM) 协议 — 当前主线:
 *   - install(sdk) 返回 PluginRuntimeAssets, 必须包含 mount(shadowRoot, ctx)
 *   - host PluginView 给每个 plugin route 创建独立 ShadowRoot, 调 plugin.mount
 *   - plugin 通过 sdk.runtime.createApp(rootComponent, targetEl) 获得已 wire 好
 *     pinia/i18n 的独立 Vue app, 把它挂到 shadow root 内的 div
 *   - plugin 卸载由 host 调返回的 PluginInstance.unmount() 触发
 *
 *   设计目的: 让 plugin DOM 与 host DOM / CSS / Vue tree 物理隔离, plugin 渲染
 *   抛错 / 注入全局样式都不会影响 host 主页面.
 */
import type { App, Component, Ref } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { AxiosInstance } from 'axios'

/**
 * Minimal User shape used by HostAuth.
 *
 * SDK 包不能 import host alias `@/types`，因此在 SDK 内部定义最小可用形状。
 * 实际 host 注入的是完整 `User` 类型 (`@/types`)，结构兼容此 shape。
 * 插件如需更丰富的字段，可通过 `[key: string]: unknown` 索引签名访问。
 */
export interface User {
  id: number
  username: string
  email?: string
  role?: string
  [key: string]: unknown
}

/** Host SDK 版本号，遵循 semver。 */
export const HOST_SDK_VERSION = '1.0.0'

/** 注入到 window 的全局变量名。 */
export const HOST_SDK_GLOBAL_KEY = '__SUB2API_HOST_SDK__'

/**
 * Vue provide/inject key — host 通过 sdk.runtime.createApp 给插件 app 注入一个
 * 「Teleport 落地容器」, 让 SDK 内部的 BaseDialog / Select 等组件把 overlay
 * 传送到 ShadowRoot 内的 portal div, 而不是 host document.body.
 *
 * 为什么必须用 Symbol.for (全局 symbol registry):
 *   plugin-sdk 在浏览器里有两份模块实例 —
 *     1) host 主 bundle 走源码 import (vite alias / pnpm workspace)
 *     2) plugin entry 走 importmap → /api/v1/plugin-assets/__shared__/plugin-sdk.js
 *   普通 `Symbol()` 会在两份 bundle 各自生成不同 identity, provide/inject 错配,
 *   inject 永远拿到 fallback. `Symbol.for(key)` 走全局 registry, 跨 bundle 同 key
 *   返回同一个 symbol, 才能让 host provide / plugin inject 命中.
 *
 * Fallback 约定:
 *   inject(PLUGIN_TELEPORT_TARGET, 'body') — host 自身使用 SDK 组件 (没经 plugin
 *   runtime, 不会 provide) 时回落到 document.body, 行为与改造前一致.
 */
export const PLUGIN_TELEPORT_TARGET = Symbol.for('sub2api.plugin-teleport-target')

export type ThemeMode = 'light' | 'dark'
export type FontSize = 'sm' | 'base' | 'lg'
export type ToastType = 'success' | 'error' | 'warning' | 'info'

/** 主题相关能力。`mode` reactive，host 切换主题时插件会自动响应。 */
export interface HostTheme {
  /** 当前主题模式（light / dark），reactive。 */
  mode: Ref<ThemeMode>
  /** 主色，reactive。当前 host 没有定制主色，固定一个默认值。 */
  primaryColor: Ref<string>
  /** 切换 light / dark。 */
  toggle(): void
  /** 显式设置主题模式。 */
  set(mode: ThemeMode): void
}

/** 字体相关能力。如果 host 没有实际控制，提供合理默认值并保持 reactive。 */
export interface HostFont {
  /** 字号档位，reactive。 */
  size: Ref<FontSize>
  /** 字体家族，reactive。 */
  family: Ref<string>
}

/** 简化版 i18n 接口，对接 vue-i18n。 */
export interface HostI18n {
  /** 翻译函数，与 vue-i18n 的 `t()` 等价。 */
  t(key: string, params?: Record<string, unknown>): string
  /** 当前 locale，reactive。 */
  currentLocale: Ref<string>
  /**
   * 注册插件自己的 i18n messages, 扁平合并到 host vue-i18n 实例。
   *
   * - `namespace` 仅做幂等去重 key (同一 plugin 重复 install 时跳过重复 merge), 不会成为
   *   message 树的一层。
   * - `messages` 形如 `{ en: { admin: { channels: { ... } } }, zh: { ... } }` —— 顶层 key
   *   是 locale code, 二层及以下原样 deep-merge 到对应 locale 下。
   * - 插件后续可直接 `t('admin.channels.title')` 访问 (无需 namespace 前缀)。
   * - 同名 key 与 host 冲突时 vue-i18n 走 deep-merge 默认覆盖语义。
   */
  registerNamespace(namespace: string, messages: Record<string, Record<string, unknown>>): void
  /**
   * 监听 locale 变化。回调在 locale 切换后同步触发，参数为新 locale 值。
   * 返回取消监听函数，调用后不再收到后续通知。
   *
   * 等价于 `watch(sdk.i18n.currentLocale, cb)`，但不要求插件了解 Vue 的 watch API。
   */
  onChange(callback: (locale: string) => void): () => void
}

/** 通知（toast）能力，对接 host 的 toast 系统。 */
export interface HostNotify {
  success(message: string, duration?: number): void
  error(message: string, duration?: number): void
  warning(message: string, duration?: number): void
  info(message: string, duration?: number): void
}

/** 路由能力，直接复用 host 的 vue-router 实例。 */
export interface HostRouter {
  /** 跳转到指定 path。等价于 `router.push(path)`。 */
  push(path: string): Promise<unknown>
  /** 后退一步。 */
  back(): void
  /** 当前路由，reactive。 */
  currentRoute: Ref<RouteLocationNormalizedLoaded>
}

/** 鉴权能力，只读句柄。插件不应通过 SDK 修改用户状态。 */
export interface HostAuth {
  /** 当前登录用户，未登录时为 null，reactive。 */
  user: Ref<User | null>
  /** 当前用户是否为管理员，reactive。 */
  isAdmin: Ref<boolean>
  /** 当前用户是否已登录，reactive。 */
  isAuthenticated: Ref<boolean>
}

/** HTTP 能力，复用 host 的 axios 实例（含 token 拦截器、错误处理）。 */
export interface HostHttp {
  /** 与 host 同源的 axios 实例。 */
  apiClient: AxiosInstance
}

/**
 * 暴露 host 的 Vue 运行时给插件使用, 让插件 bundle 不需要打包 Vue.
 * 仅暴露常用 API; 插件需要更多 Vue 接口时, 应自己引入 vue (但会增加 bundle 体积).
 */
export interface HostVue {
  /** Vue 的 h() 渲染函数. */
  h: typeof import('vue').h
  /** 定义组件. */
  defineComponent: typeof import('vue').defineComponent
  /** ref(). */
  ref: typeof import('vue').ref
  /** computed(). */
  computed: typeof import('vue').computed
  /** watch(). */
  watch: typeof import('vue').watch
  /** onMounted / onUnmounted. */
  onMounted: typeof import('vue').onMounted
  onUnmounted: typeof import('vue').onUnmounted
}

/**
 * createApp 选项, 给 plugin 控制全局 component / 错误边界.
 */
export interface HostRuntimeAppOptions {
  /** 额外的全局组件 (例如 plugin 想把 SDK 共享组件 Icon/DataTable 等批量注册). */
  components?: Record<string, Component>
  /**
   * 错误处理. 不传时默认走 sdk.notify.error, 保证错误不静默吞掉.
   * 抛错的 plugin component 不会冒泡到 host 主 app, 因为是独立 createApp 实例.
   */
  onError?: (err: unknown, instance: unknown, info: string) => void
}

/**
 * Plugin 通过 sdk.runtime.createApp 获得的实例句柄. host 卸载时调 unmount().
 */
export interface PluginAppInstance {
  /** 真正的 Vue App 实例, 仅供 plugin 内部高级用法 (如 provide/inject). */
  app: App
  /** 卸载. host 不需要直接调用 — host 调 PluginRuntimeAssets.mount 返回的 PluginInstance.unmount(). */
  unmount(): void
}

/**
 * Host runtime 能力. 让 plugin 不需要自己 createApp / use(pinia) / use(i18n).
 *
 * createApp 的实现内部:
 *   1) 用 host 的 vue.createApp (importmap 共享, 与 host main app 是同一个 createApp 函数)
 *   2) app.use(host pinia)  — 共享 store registry
 *   3) app.use(host i18n)   — 共享 message dictionary
 *   4) 注册 options.components 为全局组件
 *   5) 设置 app.config.errorHandler 走 sdk.notify.error (除非 options.onError 覆盖)
 *   6) app.mount(target)
 *
 * target 可以是 ShadowRoot 内的 Element. CSS 样式由 caller (mount-plugin) 注入到
 * shadow root, 此处不负责样式.
 */
export interface HostRuntime {
  createApp(rootComponent: Component, target: Element, options?: HostRuntimeAppOptions): PluginAppInstance
}

/**
 * Host 在调 plugin.mount 时传给 plugin 的上下文.
 * plugin 据此决定渲染哪个 view (同一 plugin 多个路由复用一份 entry bundle, 由 componentPath 区分).
 */
export interface PluginRuntimeContext {
  /** 对齐 manifest.routes[].component_path 的值. */
  componentPath: string
  /** 当前路由对象的浅快照. plugin 可读 query / params, 不应直接 mutate. */
  route: RouteLocationNormalizedLoaded
  /** plugin 名, 主要用于 plugin 内部 logging. */
  pluginName: string
}

/**
 * Host 调 plugin.mount 后获得的句柄. host 切路由时会调 unmount().
 */
export interface PluginInstance {
  unmount(): void
}

/**
 * 插件 entry bundle 必须 default export 的契约对象 (V2).
 *
 *   default export { install }
 *   install(sdk) -> PluginRuntimeAssets
 *   assets.mount(shadowRoot, ctx) -> PluginInstance
 */
export interface PluginRuntimeModule {
  install(sdk: HostSdk): PluginRuntimeAssets | Promise<PluginRuntimeAssets>
}

/** install 返回的资产清单 (V2). */
export interface PluginRuntimeAssets {
  /**
   * Host 给 plugin 一个独立 ShadowRoot, 让 plugin 在内部完成自己的 createApp + mount.
   * 返回 PluginInstance, host 切路由 / 卸载时调 unmount().
   *
   * Implementation note: plugin 通常借助 sdk.runtime.createApp 完成此步, 而不是
   * 自己 import { createApp } from 'vue' (后者也能用, importmap 共享同一份 vue,
   * 但需要自己 use pinia/i18n).
   */
  mount(shadowRoot: ShadowRoot, ctx: PluginRuntimeContext): PluginInstance | Promise<PluginInstance>

  /**
   * Raw Vue components for host-tree rendering (NOT Shadow DOM).
   *
   * Unlike mount() which renders inside an isolated ShadowRoot, formComponents
   * are rendered directly in the host Vue tree. This is necessary for account
   * form components that need template ref access from the host (e.g. validate(),
   * getPayload()) and must participate in the host's form lifecycle.
   *
   * key 对应 AccountTypeDeclaration.form_component_path, value 是 Vue Component.
   * host 在打开创建/编辑账号弹窗时, 优先从这里取表单组件, 未找到时降级到
   * 内置表单 (BUILTIN_FORMS) 或通用 JSON schema 表单 (PluginForm.vue).
   *
   * 插件 install() 时把编译好的 .vue 组件放进 map 即可, 不需要额外注册.
   */
  formComponents?: Record<string, Component>

  /**
   * 可选: 插件提供的分组配置组件映射.
   *
   * key 对应 GroupConfigDeclaration.form_component_path, value 是 Vue Component.
   * 逻辑与 formComponents 对称: host 优先取此 map, 降级到内置组件或 PluginGroupConfig.vue.
   */
  groupConfigComponents?: Record<string, Component>


  /**
   * 可选: 插件提供的账号测试组件映射.
   *
   * key 格式为 `<platform>:<accountType>` 或 `<platform>:*` (通配),
   * value 是 Vue Component. host 的 AccountTestModal 在打开测试弹窗时,
   * 优先从此 map 取组件, 未找到时降级到内置测试面板.
   *
   * 测试组件在 host Vue tree 中直接渲染 (与 formComponents 对称),
   * 接收标准 props: testContext (SdkTestContext).
   * 必须 expose AccountTestExposed { startTest, abort, isRunning }.
   */
  testComponents?: Record<string, Component>

  /**
   * 可选: 插件提供的账号用量展示组件映射.
   *
   * key 格式为 `<platform>:<accountType>` 或 `<platform>:*` (通配),
   * value 是 Vue Component. host 的 AccountUsageCell 在渲染用量列时,
   * 优先从此 map 取组件, 未找到时降级到通用的 KeyAccountStats.
   *
   * 用量组件在 host Vue tree 中直接渲染 (与 formComponents 对称),
   * 接收标准 props: account, usageInfo, loading, error, todayStats, todayStatsLoading, activeQueryLoading.
   * 支持 emit 'active-query' 事件触发主动查询.
   */
  usageComponents?: Record<string, Component>
}

/** Minimal group shape attached to an API key, for plugin consumption. */
export interface SdkApiKeyGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
}

/** Minimal API key shape exposed to plugins. Excludes internal fields (user_id, ip_whitelist, quota, rate limits). */
export interface SdkApiKey {
  id: number
  name: string
  key: string
  status: string
  expires_at: string | null
  group?: SdkApiKeyGroup
}

/** User API key access. Returns keys belonging to the currently authenticated user. */
export interface HostKeys {
  /** Fetch the current user's active, non-expired API keys. */
  listActive(): Promise<SdkApiKey[]>
  /** Fetch the current user's custom group rate multipliers. */
  getUserGroupRates(): Promise<Record<number, number>>
}

/**
 * Result of a successful image upload. The URL is host-relative (no origin)
 * and is safe to embed directly in `<img :src>` or persist in a settings
 * record. The host serves these URLs as public, cacheable assets — do not
 * store sensitive imagery here.
 */
export interface UploadedImage {
  /** Public URL of the stored image (e.g. `/api/v1/uploads/<hash>.png`). */
  url: string
}

/**
 * Image upload capability. Plugins use this to push user-selected files to
 * the host, which persists them under a dedupe-by-content path and returns
 * a stable URL.
 *
 * Constraints (mirrored on the backend):
 *   - File ≤ 5 MiB
 *   - MIME type ∈ {image/jpeg, image/png, image/webp, image/gif, image/svg+xml}
 *   - Auth: requires the plugin to be running inside the admin app context
 *     (the host axios client carries the admin session token / api key).
 *
 * Errors surface via promise rejection using the host's standard API error
 * shape; plugins should funnel through their `extractApiErrorMessage` helper
 * to display a user-facing message.
 */
export interface HostImages {
  /**
   * Upload a single image file. Resolves with the public URL of the stored
   * image on success. Throws on validation / auth / network failure.
   */
  upload(file: File): Promise<UploadedImage>
}

/**
 * Mask an API key for display, showing only head/tail characters.
 *
 * - Keys <= 12 chars: `${first4}***`
 * - Keys > 12 chars: `${first6}...${last4}`
 */
export function maskApiKey(key: string): string {
  if (!key) return ''
  if (key.length <= 12) return `${key.slice(0, 4)}***`
  return `${key.slice(0, 6)}...${key.slice(-4)}`
}

/** Host SDK 完整接口。 */
export interface HostSdk {
  /** SDK 协议版本号。 */
  version: string
  theme: HostTheme
  font: HostFont
  i18n: HostI18n
  notify: HostNotify
  router: HostRouter
  auth: HostAuth
  http: HostHttp
  /** Host 的 Vue 运行时. 插件可借用避免重复打包 Vue. */
  vue: HostVue
  /** Host 提供的 Vue app 工厂, 自动 use pinia/i18n + 注册全局组件. */
  runtime: HostRuntime
  /** 当前用户的 API key 查询能力. */
  keys: HostKeys
  /** 图片上传能力（admin only）. */
  images: HostImages
}

declare global {
  interface Window {
    [HOST_SDK_GLOBAL_KEY]?: HostSdk
  }
}
