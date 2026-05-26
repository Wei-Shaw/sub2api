/**
 * 插件 entry.js 运行时加载器 (V2 — Shadow DOM 协议).
 *
 * 与 `loader.ts` 的分工:
 *   - `loader.ts` 处理 `window.__PLUGIN_MANIFESTS__` 的解析、路由占位注册;
 *     运行在应用启动早期, 不发起任何网络请求.
 *   - `loader-runtime.ts` 在用户真正访问插件路由时才被调用,
 *     按需加载远程 entry.js, 执行 install(sdk) 并缓存返回的 PluginRuntimeAssets.
 *     真正的"挂到 DOM"由 mount-plugin.ts 完成.
 *
 * V2 协议 (与 v1 components map 区别):
 *   - install 返回 `{ mount(shadowRoot, ctx) -> PluginInstance }`
 *   - host 给每个 plugin route 创建独立 ShadowRoot, 调 assets.mount
 *   - 不再由 host 把 plugin 组件直接渲染到 router-view
 *
 * 加载策略:
 *   - 优先用 dynamic import + `/* @vite-ignore *\/` 让 Vite 不在编译期解析 URL,
 *     这样可以加载任意域 / 任意路径的 ESM 模块.
 *   - 失败时不影响 host: loadPluginEntry 返回 { error }, PluginView 渲染降级页面+重试按钮.
 *
 * 隔离:
 *   - V2 默认通过 Shadow DOM (mount-plugin.ts) 实现 DOM/CSS 隔离.
 *   - 仍通过 importmap 共享 vue/pinia/router/i18n/axios/SDK 这 6 个 singleton.
 *   - manifest.isolation === 'iframe' 保留为占位, 暂不实现 (Shadow DOM 已覆盖大多数场景).
 */

import type { HostSdk, PluginRuntimeAssets, PluginRuntimeModule } from '@sub2api/plugin-sdk'
import { getHostSdk } from './sdk/host-sdk-window'

export type PluginIsolationMode = 'shared' | 'iframe'

/** 描述一次加载请求所需的最小信息; 来自 manifest 的字段子集. */
export interface PluginEntryDescriptor {
  /** 插件唯一名, 用于缓存 key 与日志. */
  pluginName: string
  /** 远程 entry.js URL (HTTP). */
  entryJsUrl: string
  /** 可选 entry.css URL — V2 不再由 loader-runtime 注入到 head, 由 mount-plugin
   *  在 ShadowRoot 内注入, 但仍透传一下让 caller 不需要再读 manifest. */
  entryCssUrl?: string
  /** 隔离模式; 未指定时按 'shared' 处理. */
  isolation?: PluginIsolationMode
}

interface CachedRuntime {
  /** install 返回的资产; 解析失败时为 null (缓存负面结果以避免反复请求). */
  assets: PluginRuntimeAssets | null
  /** 加载错误, assets=null 时配套填充. */
  error: Error | null
}

const cache = new Map<string, Promise<CachedRuntime>>()

/**
 * 按需加载插件 entry.js, 执行 install, 返回插件交付的资产 (含 mount 函数).
 * 同一个 plugin 多次调用会复用同一个 in-flight promise / 已完成结果,
 * 失败也会被缓存以避免循环重试, 调用方需通过 `unloadPlugin` 主动清空才能重试.
 *
 * 注意: install 只跑一次. 同一 plugin 的多个路由共享同一份 assets, 但每个路由
 * 独立调 assets.mount(shadowRoot, ctx), 所以 plugin 实例彼此隔离.
 */
export async function loadPluginEntry(desc: PluginEntryDescriptor): Promise<CachedRuntime> {
  if (!desc.pluginName || !desc.entryJsUrl) {
    return {
      assets: null,
      error: new Error('plugin entry descriptor missing pluginName or entryJsUrl'),
    }
  }
  const isolation: PluginIsolationMode = desc.isolation ?? 'shared'
  if (isolation === 'iframe') {
    // 留接口供未来实现; 当前 Shadow DOM 已足够.
    return {
      assets: null,
      error: new Error('plugin isolation mode "iframe" is not implemented yet'),
    }
  }

  const cached = cache.get(desc.pluginName)
  if (cached) {
    return cached
  }

  const promise = doLoad(desc).catch((err): CachedRuntime => {
    const error = err instanceof Error ? err : new Error(String(err))
    // eslint-disable-next-line no-console
    console.error('[plugin-loader-runtime] failed to load plugin entry', desc.pluginName, error)
    return { assets: null, error }
  })
  cache.set(desc.pluginName, promise)
  return promise
}

/**
 * 卸载缓存, 使下次 `loadPluginEntry` 重新执行远程加载.
 * 主要用于热禁用插件、或者用户点击"重试"按钮.
 */
export function unloadPlugin(pluginName: string): void {
  cache.delete(pluginName)
}

// ----------------------------------------------------------------------------
// 内部
// ----------------------------------------------------------------------------

async function doLoad(desc: PluginEntryDescriptor): Promise<CachedRuntime> {
  // 用 @vite-ignore 让 Vite 在 build 时不解析这个动态 import, 运行时直接当作普通 ESM URL.
  // 这样支持任意远程 URL (包括同源路由 /api/v1/plugin-assets/...).
  const moduleNs: unknown = await import(/* @vite-ignore */ desc.entryJsUrl)
  const runtimeModule = pickModuleExport(moduleNs)
  if (!runtimeModule) {
    throw new Error(`plugin "${desc.pluginName}" entry.js did not export an install() function`)
  }

  const sdk = getHostSdk()
  if (!sdk) {
    throw new Error('host SDK not attached; ensure attachHostSdkToWindow ran before plugin load')
  }
  const assets = await runtimeModule.install(sdk)
  if (!assets || typeof assets.mount !== 'function') {
    throw new Error(
      `plugin "${desc.pluginName}" install() did not return a mount(shadowRoot, ctx) function — V2 协议必需`,
    )
  }
  return { assets, error: null }
}

/**
 * ESM 模块可能通过 default export 也可能通过具名 export `install` 暴露契约.
 * 兼容两种形态以减少插件作者负担.
 */
function pickModuleExport(ns: unknown): PluginRuntimeModule | null {
  if (!ns || typeof ns !== 'object') {
    return null
  }
  const obj = ns as Record<string, unknown>
  // 形态 1: export default { install }
  const def = obj.default
  if (def && typeof def === 'object' && typeof (def as Record<string, unknown>).install === 'function') {
    return def as PluginRuntimeModule
  }
  // 形态 2: export function install
  if (typeof obj.install === 'function') {
    return obj as unknown as PluginRuntimeModule
  }
  return null
}

// 让外部模块可以静态使用这两个类型 / 工具, 避免 unused-import 警告
export type { HostSdk, PluginRuntimeAssets, PluginRuntimeModule }
