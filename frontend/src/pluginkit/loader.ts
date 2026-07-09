/**
 * 外部插件前端运行时加载器
 *
 * loadExternalPlugins(list) 是唯一入口（App.vue 在 enabled 清单每次落盘后
 * 调用）：对 enabled 且带 assets 的外部插件注入 <script>（IIFE），脚本内部
 * 调用 window.__SUB2API_PLUGIN_REGISTER__ 提交描述符完成注册。
 *
 * fail-closed 与隔离语义：
 *   - 注入前经 expectRuntimePlugin 登记期望 ID，并经 bindRuntimeScript 把
 *     <script> 元素与 ID 绑定；registerRuntime 要求注册发生在该脚本的同步
 *     求值期间且归属一致（契约：插件必须在 IIFE 顶层同步注册），任意脚本
 *     注册任意 ID / 异步抢注一律被拒；
 *   - 路由注入前做 name/path 冲突检查，插件路由不得顶替核心路由；
 *   - 单个脚本加载失败/超时仅 console.warn 后跳过，不影响其他插件与核心应用；
 *   - 脚本按插件 ID 只注入一次（含失败：本会话不重试，页面刷新自然重置）；
 *   - 停用（清单不再包含）时经 syncRuntimeActivation 移除路由与导航贡献，
 *     同一会话内重新启用直接复活缓存描述符，不重复注入脚本。
 */

import router from '@/router'
import type { RouteRecordRaw } from 'vue-router'
import { i18n } from '@/i18n'
import { buildApiUrl } from '@/api/url'
import {
  bindRuntimeScript,
  expectRuntimePlugin,
  hasRuntimeEntry,
  setRuntimeHost,
  syncRuntimeActivation
} from './registry'
import type { EnabledPluginEntry } from './types'

// 脚本加载超时：超过后视为失败（脚本若之后仍完成加载并注册，
// 期望 ID 仍在集合内，晚到的注册照常生效，仅告警时序不同）
const SCRIPT_LOAD_TIMEOUT_MS = 15_000

// 本会话已注入过脚本的插件 ID（成功与否均记录）
const injectedScriptIds = new Set<string>()

let runtimeHostBound = false

// collectRouteNames 递归收集路由树（含 children）里的全部命名。
function collectRouteNames(route: RouteRecordRaw, out: (string | symbol)[]): void {
  if (route.name != null) {
    out.push(route.name)
  }
  for (const child of route.children ?? []) {
    collectRouteNames(child, out)
  }
}

// collectRoutePaths 递归收集路由树的绝对路径（child 相对路径按 vue-router
// 语义拼接到父路径上；以 / 开头的 child 本身即绝对路径）。
function collectRoutePaths(route: RouteRecordRaw, parentPath: string, out: string[]): void {
  const abs = route.path.startsWith('/')
    ? route.path
    : `${parentPath.replace(/\/+$/, '')}/${route.path}`.replace(/\/{2,}/g, '/')
  out.push(abs)
  for (const child of route.children ?? []) {
    collectRoutePaths(child, abs, out)
  }
}

// assertNoRouteConflict 拒绝与既有路由（核心路由或先注入的插件路由）
// name/path 冲突的运行时注入：vue-router 对同名 addRoute 是替换语义，
// 不设防的插件路由可顶替 Login 等核心路由，且停用插件也不会恢复。
// 内建插件路由走静态路由表并有快照测试守护，此处是运行时注入面的等价防线。
function assertNoRouteConflict(route: RouteRecordRaw): void {
  const names: (string | symbol)[] = []
  collectRouteNames(route, names)
  for (const name of names) {
    if (router.hasRoute(name)) {
      throw new Error(`route name "${String(name)}" conflicts with an existing route`)
    }
  }
  const existingPaths = new Set(router.getRoutes().map((r) => r.path))
  const paths: string[] = []
  collectRoutePaths(route, '', paths)
  for (const path of paths) {
    if (existingPaths.has(path)) {
      throw new Error(`route path "${path}" conflicts with an existing route`)
    }
  }
}

// ensureRuntimeHost 把真实 router / i18n 以最小接口绑定进 registry。
// 绑定收敛在 loader 内：registry 与各接入点/测试不因此拖入整棵应用模块图。
function ensureRuntimeHost(): void {
  if (runtimeHostBound) {
    return
  }
  setRuntimeHost({
    addRoute: (route) => {
      assertNoRouteConflict(route)
      return router.addRoute(route)
    },
    mergeLocaleMessage: (locale, messages) => i18n.global.mergeLocaleMessage(locale, messages)
  })
  runtimeHostBound = true
}

// resolveAssetUrl 把后端下发的 assets 路径（/api/v1/...）按 API base 解析：
// 前后端分离部署（VITE_API_BASE_URL 为完整 URL）时指向后端源，绝对 URL 原样使用
function resolveAssetUrl(assets: string): string {
  if (/^https?:\/\//i.test(assets)) {
    return assets
  }
  return buildApiUrl(assets)
}

// injectPluginScript 注入单个插件脚本，onload/onerror/超时三态收敛为一次 settle
function injectPluginScript(id: string, assets: string): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const script = document.createElement('script')
    script.async = true
    // dataset 仅供 DOM 排障/测试定位；归属认定走 bindRuntimeScript 的元素
    // 身份绑定（DOM 属性可被页面内其他脚本伪造，元素身份不能）。
    script.dataset.pluginId = id
    bindRuntimeScript(script, id)

    let settled = false
    const timer = window.setTimeout(() => {
      settle(new Error(`script load timed out after ${SCRIPT_LOAD_TIMEOUT_MS}ms`))
    }, SCRIPT_LOAD_TIMEOUT_MS)

    function settle(error: Error | null): void {
      if (settled) {
        return
      }
      settled = true
      window.clearTimeout(timer)
      if (error === null) {
        resolve()
      } else {
        reject(error)
      }
    }

    script.onload = () => settle(null)
    script.onerror = () => settle(new Error(`failed to load ${script.src}`))
    script.src = resolveAssetUrl(assets)
    document.head.appendChild(script)
  })
}

/**
 * 按最新 enabled 清单加载/对账外部插件前端半场。
 *
 * 幂等：重复调用同一清单不产生新脚本注入；清单收缩时移除对应运行时贡献。
 * 不抛错：所有失败均降级为 console.warn（enabled 拉取链路不因插件受阻）。
 */
export async function loadExternalPlugins(plugins: readonly EnabledPluginEntry[]): Promise<void> {
  ensureRuntimeHost()

  const withAssets = plugins.filter(
    (plugin): plugin is EnabledPluginEntry & { assets: string } =>
      typeof plugin.id === 'string' &&
      plugin.id !== '' &&
      typeof plugin.assets === 'string' &&
      plugin.assets !== ''
  )

  // 先对账既有运行时状态：停用集合外的贡献并撤销其注册期望，
  // 再为新插件登记期望、注入脚本
  const enabledIds = new Set(withAssets.map((plugin) => plugin.id))
  syncRuntimeActivation(enabledIds)

  // 竞态补偿：脚本仍在网络加载途中就被停用的插件，其迟到注册已被期望
  // 撤销拒绝且无描述符缓存——注入标记留在集合里会让 re-enable 永远短路
  // （既不重注脚本也无缓存可复活，插件整会话失活）。对"已注入过、现不在
  // enabled 集合、且无缓存"的 ID 清除标记，重新启用时允许重新注入。
  for (const id of [...injectedScriptIds]) {
    if (!enabledIds.has(id) && !hasRuntimeEntry(id)) {
      injectedScriptIds.delete(id)
    }
  }

  const injections: Promise<void>[] = []
  for (const plugin of withAssets) {
    if (injectedScriptIds.has(plugin.id)) {
      continue
    }
    injectedScriptIds.add(plugin.id)
    expectRuntimePlugin(plugin.id)
    injections.push(
      injectPluginScript(plugin.id, plugin.assets).catch((error: unknown) => {
        console.warn(`[pluginkit] failed to load external plugin "${plugin.id}":`, error)
      })
    )
  }
  await Promise.all(injections)
}

/** 仅测试使用：清空脚本注入记录与宿主绑定标记 */
export function _resetLoaderForTest(): void {
  injectedScriptIds.clear()
  runtimeHostBound = false
}
