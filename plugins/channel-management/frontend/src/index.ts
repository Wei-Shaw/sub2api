/**
 * Channel Management plugin frontend runtime entry (V2 — Shadow DOM 协议).
 *
 * 契约 (与 frontend/src/plugins/loader-runtime.ts + mount-plugin.ts 对齐):
 *   - default export 必须是 { install(sdk) }
 *   - install 返回 { mount(shadowRoot, ctx) -> PluginInstance }
 *   - mount 在收到的 ShadowRoot 内挂 plugin Vue app
 *   - PluginInstance.unmount 由 host 在路由切换 / 卸载时调用
 *
 * 共享上下文 (importmap 提供):
 *   - vue / vue-router / vue-i18n / pinia / axios — 与 host 同一份 singleton
 *   - @sub2api/plugin-sdk — host 编译的 SDK bundle (含共享组件)
 *
 * Plugin Vue app 通过 sdk.runtime.createApp 创建:
 *   - 自动 use(host pinia) — 共享 store registry
 *   - 自动 use(host i18n) — 共享 message dictionary
 *   - 自动注册 SDK 共享组件全局
 *   - 自动设置 errorHandler 走 sdk.notify.error
 *
 * 副作用:
 *   - install 时把 plugin i18n messages 扁平合并到 host vue-i18n 实例
 *   - install 时把 sdk.http.apiClient 注入 plugin 内部 axios 单例
 */
import type {
  HostSdk,
  PluginRuntimeAssets,
} from '@sub2api/plugin-sdk'
import { SDK_GLOBAL_COMPONENTS, createPluginMount } from '@sub2api/plugin-sdk'
import type { AxiosInstance } from 'axios'
import type { Component } from 'vue'
import './style.css'
import ChannelsView from './views/ChannelsView.vue'
import AvailableChannelsView from './views/user/AvailableChannelsView.vue'
import ChannelMonitorView from './views/admin/ChannelMonitorView.vue'
import ChannelStatusView from './views/user/ChannelStatusView.vue'
import { setClient } from './api/client'
import { setSdk } from './api/sdk'
import enMessages from './i18n/en'
import zhMessages from './i18n/zh'

/** plugin 内部 dedup key, 与后端 manifest.I18nNamespaces 中的 namespace 名对齐. */
const I18N_NAMESPACE = 'channel-management'

/** componentPath -> 对应的 Vue view 组件. */
const VIEWS: Record<string, Component> = {
  'ChannelsView.vue': ChannelsView,
  'AvailableChannelsView.vue': AvailableChannelsView,
  'ChannelMonitorView.vue': ChannelMonitorView,
  'ChannelStatusView.vue': ChannelStatusView,
}

/** SDK 共享组件全局注册表统一由 @sub2api/plugin-sdk 提供 (SDK_GLOBAL_COMPONENTS). */

function install(sdk: HostSdk): PluginRuntimeAssets {
  // 1. 注入 host axios 实例 — plugin channels API 调用复用 host 的 auth header / 错误拦截器.
  //    类型断言: host 与 plugin 各自 resolve axios 类型 (host frontend node_modules
  //    与 plugin node_modules 的小版本可能不同), TS 严格模式下两份 InternalAxiosRequestConfig
  //    被视为不同 nominal type. 运行时是同一个 axios 实例 (importmap 共享),
  //    所以这里用 unknown 桥接是安全的.
  setClient(sdk.http.apiClient as unknown as AxiosInstance)

  // 1b. 把整个 SDK 句柄存入 plugin module-scope 单例, 让视图层通过 getSdk() 取到
  //     notify / auth / theme 等能力.
  setSdk(sdk)

  // 2. plugin i18n keys 扁平合并到 host vue-i18n 实例. registerNamespace 是
  //    幂等的 (同 namespace 重复 install 不会叠加).
  sdk.i18n.registerNamespace(I18N_NAMESPACE, {
    en: enMessages,
    zh: zhMessages,
  })

  // 3. V2 协议: 返回 mount 函数, 由 host 在 ShadowRoot 内调用.
  return {
    mount: createPluginMount(sdk, VIEWS, 'plugin-channel-management', {
      globalComponents: SDK_GLOBAL_COMPONENTS,
    }),
  }
}

export default { install }

// Named export 作为兼容入口, loader-runtime.ts 的 pickModuleExport 同时支持
// `export default { install }` 与 `export function install`.
export { install }
