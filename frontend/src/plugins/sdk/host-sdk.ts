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
 */
import type { Component, Ref } from 'vue'
import type { RouteLocationNormalizedLoaded, RouteRecordRaw } from 'vue-router'
import type { AxiosInstance } from 'axios'
import type { User } from '@/types'

/** Host SDK 版本号，遵循 semver。 */
export const HOST_SDK_VERSION = '1.0.0'

/** 注入到 window 的全局变量名。 */
export const HOST_SDK_GLOBAL_KEY = '__SUB2API_HOST_SDK__'

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
 * 插件 entry bundle 必须 default export 的契约对象。
 *
 * 调用 install 后期望返回插件提供的 components/routes，
 * runtime loader 会把 routes 注入 vue-router、components 缓存供 PluginView 渲染。
 */
export interface PluginRuntimeModule {
  /**
   * 插件初始化钩子。
   * @param sdk host 暴露的 SDK 对象
   * @returns 该插件提供的运行时资产；目前用到的字段是 `components`（按 component_path 索引）
   *          以及可选的 `routes`（追加注入 vue-router）。
   */
  install(sdk: HostSdk): PluginRuntimeAssets | Promise<PluginRuntimeAssets>
}

/** install 返回的资产清单。 */
export interface PluginRuntimeAssets {
  /**
   * 由 component_path（manifest.routes[].component_path 的值）索引的 Vue 组件。
   * 当 host 命中带有 `meta.componentPath` 的路由时，会按这个映射查表渲染。
   */
  components?: Record<string, Component>
  /** 额外追加的 vue-router 路由（默认情况下 host 不会自动注入）。 */
  routes?: RouteRecordRaw[]
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
}

declare global {
  interface Window {
    [HOST_SDK_GLOBAL_KEY]?: HostSdk
  }
}
