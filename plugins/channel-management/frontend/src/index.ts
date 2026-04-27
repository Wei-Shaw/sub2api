/**
 * Channel Management plugin frontend runtime entry.
 *
 * 契约 (与 frontend/src/plugins/loader-runtime.ts 对齐):
 *   - default export 必须是 { install(sdk) }
 *   - install 返回 { components: Record<componentPath, Component> }
 *   - host 在用户访问 manifest 中声明的路由 (/admin/channels) 时, 通过
 *     PluginView 按 component_path 找到对应组件并渲染
 *
 * 共享上下文:
 *   - vue / vue-router / pinia / vue-i18n / axios 由 host 通过 importmap 暴露
 *     (见 frontend/src/plugins/sdk/expose-runtime.ts 与
 *      backend/internal/server/routes/plugin_assets.go 的 __shared__ 端点)
 *   - host 的 toast / auth / theme 通过 sdk.notify / sdk.auth / sdk.theme 访问
 *
 * 副作用:
 *   - install 时把插件的 i18n messages 扁平合并到 host vue-i18n 实例
 *   - install 时把 sdk.http.apiClient 注入 plugin 内部的 axios 单例 (api/client.ts)
 */
import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import './style.css'
import ChannelsView from './views/ChannelsView.vue'
import AvailableChannelsView from './views/user/AvailableChannelsView.vue'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

/** plugin 内部 dedup key, 与后端 manifest.I18nNamespaces 中的 namespace 名对齐. */
const I18N_NAMESPACE = 'channel-management'

function install(sdk: HostSdk): PluginRuntimeAssets {
  // 1. 注入 host axios 实例, 让 plugin 内部 channels API 调用复用 host 的
  //    auth header / 错误拦截器.
  //    类型断言: host 与 plugin 各自 resolve axios 类型 (host frontend node_modules
  //    与 plugin node_modules 的小版本可能不同), TS 严格模式下两份 InternalAxiosRequestConfig
  //    被视为不同 nominal type. 运行时是同一个 axios 实例 (importmap 共享),
  //    所以这里用 unknown 桥接是安全的.
  setClient(sdk.http.apiClient as unknown as AxiosInstance)

  // 1b. 把整个 SDK 句柄存入 plugin module-scope 单例, 让视图层通过 getSdk() 取到
  //     notify / auth / theme 等能力 (替代 V1 的 useAppStore/adminAPI 直引).
  setSdk(sdk)

  // 2. 把插件 i18n keys 扁平合并到 host vue-i18n 实例. registerNamespace 是
  //    幂等的 (同 namespace 重复 install 不会叠加).
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  // 3. 返回组件清单. component_path 必须与 plugin.go FrontendManifest.Routes
  //    中声明的 ComponentPath 对齐 (manifest 决定 host PluginView 用哪个 key
  //    取组件).
  return {
    components: {
      'ChannelsView.vue': ChannelsView,
      // V5 W9 — User-facing "Available Channels". manifest path:
      //   plugin.go FrontendManifest.Routes[].ComponentPath = "AvailableChannelsView.vue"
      'AvailableChannelsView.vue': AvailableChannelsView,
    },
  }
}

export default { install }

// Named export 作为兼容入口, loader-runtime.ts 的 pickModuleExport 同时支持
// `export default { install }` 与 `export function install`.
export { install }
