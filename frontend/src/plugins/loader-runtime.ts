/**
 * 插件 entry.js 运行时加载器。
 *
 * 与 `loader.ts` 的分工：
 *   - `loader.ts` 处理 `window.__PLUGIN_MANIFESTS__` 的解析、路由占位注册;
 *     运行在应用启动早期，不发起任何网络请求。
 *   - `loader-runtime.ts` 在用户真正访问插件路由时才被调用,
 *     按需加载远程 entry.js,执行插件 install(sdk) 并缓存返回的组件清单。
 *
 * 加载策略：
 *   - 优先用 dynamic import + `/* @vite-ignore *\/` 让 Vite 不在编译期解析 URL,
 *     这样可以加载任意域/任意路径的 ESM 模块。
 *   - 失败时不影响 host:loadPluginEntry 的 promise reject,
 *     PluginView 渲染降级页面+重试按钮。
 *
 * 隔离：
 *   - `'shared'`(默认):与 host 共享 window/Vue/router/i18n。
 *     主用 `window.__SUB2API_HOST_SDK__` 作为唯一通道。
 *   - `'iframe'`:占位,当前直接抛 not implemented;留好接口让后续替换。
 */

import type { Component, defineAsyncComponent } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import type { HostSdk, PluginRuntimeAssets, PluginRuntimeModule } from '@sub2api/plugin-sdk'
import { getHostSdk } from './sdk/host-sdk-window'

export type PluginIsolationMode = 'shared' | 'iframe'

/** 描述一次加载请求所需的最小信息;来自 manifest 的字段子集。 */
export interface PluginEntryDescriptor {
  /** 插件唯一名,用于缓存 key 与日志。 */
  pluginName: string
  /** 远程 entry.js URL（HTTP）。 */
  entryJsUrl: string
  /** 可选 entry.css URL，运行时通过 <link rel="stylesheet"> 注入。 */
  entryCssUrl?: string
  /** 隔离模式;未指定时按 'shared' 处理。 */
  isolation?: PluginIsolationMode
}

interface CachedRuntime {
  /** install 返回的资产；解析失败时为 null（缓存负面结果以避免反复请求）。 */
  assets: PluginRuntimeAssets | null
  /** 加载错误,assets=null 时配套填充。 */
  error: Error | null
}

const cache = new Map<string, Promise<CachedRuntime>>()
const cssLoaded = new Set<string>()

/**
 * 按需加载插件 entry.js,执行 install,返回插件交付的资产。
 * 同一个 plugin 多次调用会复用同一个 in-flight promise / 已完成结果,
 * 失败也会被缓存以避免循环重试,调用方需通过 `unloadPlugin` 主动清空才能重试。
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
    // 留接口供未来实现:此处直接报错,调用方走错误降级。
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
 * 卸载缓存,使下次 `loadPluginEntry` 重新执行远程加载。
 * 主要用于热禁用插件、或者用户点击"重试"按钮。
 */
export function unloadPlugin(pluginName: string): void {
  cache.delete(pluginName)
}

/**
 * 在已加载的资产中按 component_path 查找组件。
 * 若 component_path 为空或资产缺失,返回 null（调用方自行降级）。
 */
export function resolvePluginComponent(
  assets: PluginRuntimeAssets | null,
  componentPath: string,
): Component | null {
  if (!assets || !assets.components) {
    return null
  }
  if (!componentPath) {
    // 没指定 component_path 时,允许约定:取唯一一个组件(若只注册了 1 个)。
    const keys = Object.keys(assets.components)
    if (keys.length === 1) {
      return assets.components[keys[0]] ?? null
    }
    return null
  }
  return assets.components[componentPath] ?? null
}

// ----------------------------------------------------------------------------
// 内部
// ----------------------------------------------------------------------------

async function doLoad(desc: PluginEntryDescriptor): Promise<CachedRuntime> {
  // 注入 css(尽力而为,失败不影响 js 加载)
  if (desc.entryCssUrl) {
    injectCssOnce(desc.pluginName, desc.entryCssUrl)
  }

  // 用 @vite-ignore 让 Vite 在 build 时不解析这个动态 import,运行时直接当作普通 ESM URL。
  // 这样支持任意远程 URL（包括同源路由 /api/v1/plugin-assets/...）。
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
  return { assets: assets ?? {}, error: null }
}

/**
 * ESM 模块可能通过 default export 也可能通过具名 export `install` 暴露契约。
 * 兼容两种形态以减少插件作者负担。
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

function injectCssOnce(pluginName: string, href: string): void {
  if (cssLoaded.has(pluginName)) {
    return
  }
  cssLoaded.add(pluginName)
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = href
  link.dataset.pluginName = pluginName
  document.head.appendChild(link)
}

// 让外部模块可以静态使用这两个类型 / 工具,避免 unused-import 警告
export type { Component, defineAsyncComponent }
export type { RouteRecordRaw }
export type { HostSdk, PluginRuntimeAssets, PluginRuntimeModule }
