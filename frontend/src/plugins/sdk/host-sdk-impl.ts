/**
 * 把 Host SDK 接口连到 host 的 stores / i18n / router / notify。
 *
 * 拆分成 impl 与 window 注入两步：
 *   - impl：把 reactive ref 挂上 host 的实际数据源
 *   - window：保持 SSR-safe 的注入入口（仅在浏览器执行）
 *
 * 理由：
 *   - 测试时可直接 import { createHostSdk } 并喂 mock store/router；
 *   - 主入口 main.ts 只负责一行 `attachHostSdkToWindow(...)` 的副作用调用。
 */
import {
  computed,
  createApp,
  defineComponent,
  h,
  onMounted,
  onUnmounted,
  ref,
  watch,
  type Component,
  type Ref,
  type WritableComputedRef,
} from 'vue'
import type { Router } from 'vue-router'
import { storeToRefs, type Pinia } from 'pinia'
import { i18n, getLocale } from '@/i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { apiClient } from '@/api/client'
import { keysAPI } from '@/api/keys'
import { userGroupsAPI } from '@/api/groups'
import {
  HOST_SDK_VERSION,
  PLUGIN_TELEPORT_TARGET,
  type HostFont,
  type HostI18n,
  type HostNotify,
  type HostRouter,
  type HostRuntime,
  type HostRuntimeAppOptions,
  type HostSdk,
  type HostTheme,
  type HostAuth,
  type HostHttp,
  type HostImages,
  type HostKeys,
  type SdkApiKey,
  type HostVue,
  type PluginAppInstance,
  type ThemeMode,
  type FontSize,
  type UploadedImage,
} from './host-sdk'

const THEME_STORAGE_KEY = 'theme'
const DEFAULT_PRIMARY_COLOR = '#3b82f6' // tailwind blue-500，host 当前以 dark/light 切换为主，没有定制主色
const DEFAULT_FONT_FAMILY =
  'Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'

/**
 * 创建主题能力。
 *
 * Host 主题状态当前散落在 main.ts 与 AppSidebar.vue 中（直接 `document.documentElement.classList.toggle('dark')`），
 * 没有集中 store。这里通过 MutationObserver + ref 把 DOM 状态同步成 reactive，
 * 让插件 watch `mode` 自动响应外部切换。
 */
function createTheme(): HostTheme {
  const initialMode: ThemeMode = document.documentElement.classList.contains('dark') ? 'dark' : 'light'
  const mode = ref<ThemeMode>(initialMode)
  const primaryColor = ref<string>(DEFAULT_PRIMARY_COLOR)

  // 监听 DOM class 变化，让外部 host 通过 toggle('dark') 切换时插件也能感知。
  if (typeof MutationObserver !== 'undefined') {
    const observer = new MutationObserver(() => {
      const next: ThemeMode = document.documentElement.classList.contains('dark') ? 'dark' : 'light'
      if (next !== mode.value) {
        mode.value = next
      }
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  }

  function set(next: ThemeMode): void {
    mode.value = next
    document.documentElement.classList.toggle('dark', next === 'dark')
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next)
    } catch {
      // localStorage 不可用时忽略，主题只在当前会话生效
    }
  }

  function toggle(): void {
    set(mode.value === 'dark' ? 'light' : 'dark')
  }

  return { mode, primaryColor, toggle, set }
}

/**
 * Font 能力当前 host 没有实现，仅暴露默认值供插件读取。
 * 后续 host 增加字号选择器后，可在这里把 ref 接到对应 store。
 */
function createFont(): HostFont {
  return {
    size: ref<FontSize>('base'),
    family: ref<string>(DEFAULT_FONT_FAMILY),
  }
}

/**
 * 创建 i18n 能力。
 *
 * - `t` 直接代理 `i18n.global.t`，保持 vue-i18n 行为一致。
 * - `currentLocale` 把 vue-i18n 的 locale 包装成 vue ref，方便插件 watch。
 * - `registerNamespace` 把插件交付的多语言资源 merge 到 vue-i18n。
 */
function createI18n(): HostI18n {
  // i18n.global.locale 在 legacy:false 模式下是 WritableComputedRef<string>；这里只需要读。
  const localeRef = i18n.global.locale as WritableComputedRef<string>
  const currentLocale = ref<string>(localeRef.value || getLocale())

  watch(
    () => localeRef.value,
    (next) => {
      currentLocale.value = next
    },
    { immediate: false },
  )

  function t(key: string, params?: Record<string, unknown>): string {
    if (params && Object.keys(params).length > 0) {
      return i18n.global.t(key, params as Record<string, unknown>)
    }
    return i18n.global.t(key)
  }

  // namespacesRegistered 用于幂等去重: 同一 namespace 重复注册 (例如热重载或重新 mount)
  // 时只 merge 一次, 避免 mergeLocaleMessage 在同一 key 上叠加多份。
  const namespacesRegistered = new Set<string>()

  function registerNamespace(namespace: string, messages: Record<string, Record<string, unknown>>): void {
    if (!namespace) {
      return
    }
    if (namespacesRegistered.has(namespace)) {
      return
    }
    namespacesRegistered.add(namespace)
    // 扁平 merge 策略 (DESIGN §5 决议):
    //   - namespace 仅作为 dedup key, 不作为 i18n message 的 key 包裹层
    //   - messages[locale] 顶层结构原样 deep-merge 到 vue-i18n 该 locale 下
    //   - 这样插件可以直接复用 host 已有的 key (如 admin.channels.*) 或贡献新 key,
    //     无需在调用 t() 时强制写 namespace 前缀
    for (const [locale, ns] of Object.entries(messages)) {
      if (!ns || typeof ns !== 'object') {
        continue
      }
      i18n.global.mergeLocaleMessage(locale, ns as Record<string, unknown>)
    }
  }

  function onChange(callback: (locale: string) => void): () => void {
    return watch(currentLocale, (next) => callback(next))
  }

  return { t, currentLocale, registerNamespace, onChange }
}

/**
 * 创建通知能力。直接复用 useAppStore 暴露的 toast 方法。
 *
 * 注意：useAppStore 必须在 pinia 安装之后才能调用，因此 createHostSdk 也必须在
 * main.ts 完成 `app.use(pinia)` 之后再执行。
 */
function createNotify(): HostNotify {
  const appStore = useAppStore()
  return {
    success: (msg, duration) => appStore.showSuccess(msg, duration ?? 3000),
    error: (msg, duration) => appStore.showError(msg, duration ?? 5000),
    warning: (msg, duration) => appStore.showWarning(msg, duration ?? 4000),
    info: (msg, duration) => appStore.showInfo(msg, duration ?? 3000),
  }
}

/** 包装 vue-router 暴露给插件，避免插件直接持有 router 实例。 */
function createRouter(router: Router): HostRouter {
  return {
    push: (path: string) => router.push(path),
    back: () => router.back(),
    currentRoute: router.currentRoute as Ref<HostRouter['currentRoute']['value']>,
  }
}

/**
 * 创建 auth 能力。直接 storeToRefs 拿到 user/isAdmin 等 reactive 引用。
 */
function createAuth(): HostAuth {
  const authStore = useAuthStore()
  // storeToRefs 返回的是 reactive computed/ref；此处需要用类型断言适配 HostAuth 的 Ref 形态
  const refs = storeToRefs(authStore)
  return {
    user: refs.user as unknown as HostAuth['user'],
    isAdmin: refs.isAdmin as unknown as HostAuth['isAdmin'],
    isAuthenticated: refs.isAuthenticated as unknown as HostAuth['isAuthenticated'],
  }
}

function createHttp(): HostHttp {
  return { apiClient }
}

function createVue(): HostVue {
  return { h, defineComponent, ref, computed, watch, onMounted, onUnmounted }
}

/**
 * 创建 host runtime 能力. 内部 closure 持有 host pinia 实例, 让 plugin 通过
 * sdk.runtime.createApp 拿到一个已经 use(pinia)/use(i18n) 的独立 Vue app.
 *
 * 关键不变量:
 *   - 多个 plugin 共享同一个 pinia 实例 — store registry 是全局的, 但 store
 *     state 仍按 store id 组织, 不会因为 plugin 启动而互相覆盖
 *   - i18n 也是同一个实例, 让 sdk.i18n.t 在 plugin app 内的 useI18n() 也能用
 *   - 错误隔离: app.config.errorHandler 兜住 plugin 渲染抛错, 不冒泡到 host 主 app
 */
function createRuntime(pinia: Pinia, router: Router): HostRuntime {
  function createPluginApp(
    rootComponent: Component,
    target: Element,
    options?: HostRuntimeAppOptions,
  ): PluginAppInstance {
    const app = createApp(rootComponent)
    app.use(pinia)
    app.use(i18n)
    // Share the host's vue-router so plugin views can call useRoute() /
    // useRouter() and have route.query / router.push({...}) work natively.
    // Without this, plugin pages mounted at host routes (e.g. /recharge)
    // would see useRoute() return undefined, breaking any view migrated
    // from the host SPA.
    app.use(router)
    if (options?.components) {
      for (const [name, component] of Object.entries(options.components)) {
        app.component(name, component)
      }
    }
    // 注入 Teleport 落地容器:
    //   mount-plugin 在 ShadowRoot 内放了 .plugin-shadow-portal (root 的同级兄弟),
    //   SDK 内的 BaseDialog / Select 等组件 inject(PLUGIN_TELEPORT_TARGET) 拿到
    //   它当 <Teleport :to> 目标. 这样 overlay 不会跑出 ShadowRoot 污染 host head,
    //   也不会被 plugin 组件树里 overflow:hidden 祖先裁切.
    //   target 不在 ShadowRoot (host 自身用法 / 测试) 时退化为 'body', SDK 组件
    //   inject 端有同名 fallback, 行为与改造前一致.
    const rootNode = target.getRootNode()
    if (rootNode instanceof ShadowRoot) {
      const portal = rootNode.querySelector<HTMLElement>('.plugin-shadow-portal')
      if (portal) {
        app.provide(PLUGIN_TELEPORT_TARGET, portal)
      }
    }
    app.config.errorHandler = (err, instance, info) => {
      if (options?.onError) {
        options.onError(err, instance, info)
        return
      }
      // 默认: 让 plugin 错误以 toast 形式呈现, 不污染 host 状态.
      const message = err instanceof Error ? err.message : String(err)
      // eslint-disable-next-line no-console
      console.error('[plugin runtime] vue error', message, info, err)
      try {
        const appStore = useAppStore()
        appStore.showError(`Plugin error: ${message}`)
      } catch {
        // useAppStore 失败 (理论上不会, pinia 已 use), 静默
      }
    }
    app.mount(target)
    return {
      app,
      unmount(): void {
        app.unmount()
      },
    }
  }
  return { createApp: createPluginApp }
}

/**
 * 创建 API key 查询能力。复用 host 的 keysAPI / userGroupsAPI。
 *
 * listActive 过滤掉非 active 和已过期的 key，并映射为 SDK 最小类型。
 */
function createKeys(): HostKeys {
  return {
    async listActive(): Promise<SdkApiKey[]> {
      const res = await keysAPI.list(1, 100, { status: 'active' })
      const now = Date.now()
      return (res.items || [])
        .filter((k) => {
          if (k.status !== 'active') return false
          if (k.expires_at && new Date(k.expires_at).getTime() <= now) return false
          return true
        })
        .map((k): SdkApiKey => ({
          id: k.id,
          name: k.name,
          key: k.key,
          status: k.status,
          expires_at: k.expires_at ?? null,
          group: k.group
            ? {
                id: k.group.id,
                name: k.group.name,
                platform: k.group.platform,
                subscription_type: k.group.subscription_type ?? '',
                rate_multiplier: k.group.rate_multiplier ?? 1,
              }
            : undefined,
        }))
    },
    async getUserGroupRates(): Promise<Record<number, number>> {
      return userGroupsAPI.getUserGroupRates()
    },
  }
}

/**
 * 创建图片上传能力。
 *
 * 通过 host 的 axios 实例 (apiClient) 走 multipart/form-data 上传到
 * /admin/uploads/image, 后端返回 `{ url: "/api/v1/uploads/<hash>.png" }`,
 * apiClient 的 response interceptor 已经把外层 envelope 拆掉, 所以
 * `res.data` 直接就是 UploadedImage shape.
 *
 * Auth: apiClient 拦截器会带上 admin 登录后的 JWT; 后端 admin 中间件按
 * 既有规则鉴权。普通用户的 plugin view 调用本能力将收到 401/403, 由
 * caller 通过 extractApiErrorMessage 呈现给用户。
 */
function createImages(): HostImages {
  return {
    async upload(file: File): Promise<UploadedImage> {
      const fd = new FormData()
      fd.append('file', file)
      // 不显式设 Content-Type, 让浏览器自动加上 multipart boundary。
      const res = await apiClient.post<UploadedImage>('/admin/uploads/image', fd)
      return res.data
    },
  }
}

/**
 * 工厂函数：组装一个新的 HostSdk 实例。需要 router 与 pinia 已经创建.
 *
 * pinia 必须显式传入而不是 useStore() 模式, 因为 createHostSdk 调用时机要求
 * pinia 实例对象本身可被 plugin runtime 复用 (app.use(pinia)).
 */
export function createHostSdk(router: Router, pinia: Pinia): HostSdk {
  return {
    version: HOST_SDK_VERSION,
    theme: createTheme(),
    font: createFont(),
    i18n: createI18n(),
    notify: createNotify(),
    router: createRouter(router),
    auth: createAuth(),
    http: createHttp(),
    vue: createVue(),
    runtime: createRuntime(pinia, router),
    keys: createKeys(),
    images: createImages(),
  }
}
