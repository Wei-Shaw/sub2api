/**
 * 前端插件注册表
 *
 * builtinPlugins 是唯一的编译期装配清单（对齐后端
 * internal/plugins/builtin.go 的角色）：新增前端插件 = 在清单中加一行。
 *
 * 三个聚合函数是四个接入点的唯一数据来源：
 *   - pluginRoutes()   → router/index.ts 静态路由表末尾展开
 *   - pluginNav()      → AppSidebar admin/user 导航计算属性 concat
 *   - pluginMessages() → i18n locale 懒加载完成后 merge
 *
 * 清单为空时三者均返回空贡献，前端行为与未引入插件系统时一致
 * （由 pluginkit/__tests__ 守护测试锁定）。
 */

import { shallowRef } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import type { PluginDescriptor, PluginNavItem, PluginSettingsLoader } from './types'
import { contentModerationPlugin } from '@/plugins/content-moderation'
import { demoPlugin } from '@/plugins/demo'

/** ⭐ 唯一编译期装配清单 */
export const builtinPlugins: PluginDescriptor[] = [contentModerationPlugin, demoPlugin]

// activePlugins 是聚合函数实际读取的清单；生产环境恒等于 builtinPlugins，
// 仅守护测试可经 _setPluginsForTest 注入/清空，用于锁定"零行为变更"。
let activePlugins: readonly PluginDescriptor[] = builtinPlugins

/** 仅测试使用：注入替代清单（传 [] 即模拟"未装配任何插件"） */
export function _setPluginsForTest(plugins: readonly PluginDescriptor[]): void {
  activePlugins = plugins
}

/** 仅测试使用：恢复编译期装配清单 */
export function _resetPluginsForTest(): void {
  activePlugins = builtinPlugins
}

// stampPluginId 以 descriptor.id 递归覆写路由记录（含 children）的 meta.pluginId：
// 插件路由无论在哪一层手写 pluginId 均无效，守卫门控的归属不可伪造。
function stampPluginId(route: RouteRecordRaw, pluginId: string): RouteRecordRaw {
  const stamped = {
    ...route,
    meta: { ...route.meta, pluginId }
  } as RouteRecordRaw
  if (route.children && route.children.length > 0) {
    stamped.children = route.children.map((child) => stampPluginId(child, pluginId))
  }
  return stamped
}

/**
 * 全部插件路由（meta.pluginId 已自动补写，供路由守卫做 enabled 门控）。
 *
 * 仅覆盖编译期装配的内建插件：外部插件的路由在运行时经 registerRuntime
 * 走 router.addRoute 动态注入，不经过静态路由表。
 */
export function pluginRoutes(): RouteRecordRaw[] {
  const routes: RouteRecordRaw[] = []
  for (const plugin of activePlugins) {
    for (const route of plugin.routes ?? []) {
      routes.push(stampPluginId(route, plugin.id))
    }
  }
  return routes
}

/**
 * 指定侧（admin/user）的插件导航项，仅包含 enabled 集合内的插件
 * （内建清单 + 已激活的外部运行时插件）。
 * 按 order 升序（缺省 0），同权重保持清单注册顺序（Array.sort 稳定）。
 */
export function pluginNav(side: 'admin' | 'user', enabled: ReadonlySet<string>): PluginNavItem[] {
  const items: PluginNavItem[] = []
  for (const plugin of [...activePlugins, ...activeRuntimePlugins.value]) {
    if (!enabled.has(plugin.id)) continue
    const nav = side === 'admin' ? plugin.adminNav : plugin.userNav
    if (nav) {
      items.push(...nav)
    }
  }
  return items.sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
}

/**
 * 指定语言的全部插件文案，聚合为 { plugins: { <id>: <messages> } }，
 * 由 i18n 接入点 mergeLocaleMessage 挂载到 plugins.<id>.* 命名空间。
 *
 * 含已激活的外部运行时插件：locale 懒加载晚于外部插件注册时（如注册后才
 * 切换语言），setLocaleMessage 覆盖式写入后经本函数重新 merge 补回。
 */
export function pluginMessages(locale: 'zh' | 'en'): Record<string, unknown> {
  const byId: Record<string, object> = {}
  for (const plugin of [...activePlugins, ...activeRuntimePlugins.value]) {
    const messages = plugin.i18n?.[locale]
    if (messages) {
      byId[plugin.id] = messages
    }
  }
  return { plugins: byId }
}

/**
 * 指定插件声明的设置面板懒加载器；未声明（或未注册的 ID）返回 null。
 * 内建与运行时注册的外部插件走同一查找，插件管理页据此渲染"设置"入口——
 * 入口的有无完全由描述符声明驱动，宿主不对任何插件 ID 做特判。
 */
export function pluginSettingsLoader(id: string): PluginSettingsLoader | null {
  for (const plugin of [...activePlugins, ...activeRuntimePlugins.value]) {
    if (plugin.id === id) {
      return plugin.settings ?? null
    }
  }
  return null
}

// ==================== 外部插件运行时注册 ====================

/**
 * 运行时宿主能力，由 loader 在注入外部插件脚本前绑定。
 *
 * registry 不直接 import @/router 与 @/i18n（避免把整棵应用模块图拖进
 * 所有引用 registry 的接入点与测试），以最小接口注入所需能力。
 */
export interface RuntimeHost {
  /** router.addRoute 的等价签名：注入一条顶级路由，返回该路由的移除函数 */
  addRoute(route: RouteRecordRaw): () => void
  /** i18n.global.mergeLocaleMessage 的等价签名 */
  mergeLocaleMessage(locale: 'zh' | 'en', messages: Record<string, unknown>): void
}

interface RuntimeEntry {
  descriptor: PluginDescriptor
  /** 贡献是否已施加（路由已注入、导航/文案参与聚合） */
  active: boolean
  /** 激活期间持有的路由移除函数（stop 时逐一调用） */
  removeRoutes: Array<() => void>
}

let runtimeHost: RuntimeHost | null = null

// 已注册的外部插件描述符缓存（含已停用条目：脚本无法卸载，
// 同一会话内重新启用时直接复活缓存，不重复注入脚本）
const runtimeEntries = new Map<string, RuntimeEntry>()

// 允许注册的外部插件 ID：loader 注入脚本前登记，registerRuntime 回调时核对。
// 这是运行时注册的安全闸——任意脚本经 window 全局注册任意 ID 一律被拒。
const expectedRuntimeIds = new Set<string>()

// loader 注入的 <script> 元素 → 其承载的插件 ID。元素身份由 WeakMap 认定，
// 不读任何 DOM 属性：页面内其他脚本既不能把自造元素塞进映射，也无法靠
// 伪造 data-* 属性冒充（同 JS 上下文内这是能做到的最强归属绑定）。
const runtimeScriptOwners = new WeakMap<HTMLScriptElement, string>()

// 已激活外部插件的响应式投影：pluginNav/pluginMessages 读取它，
// AppSidebar 的计算属性与 locale 懒加载 merge 由此感知运行时增删
const activeRuntimePlugins = shallowRef<readonly PluginDescriptor[]>([])

function recomputeActiveRuntimePlugins(): void {
  const actives: PluginDescriptor[] = []
  for (const entry of runtimeEntries.values()) {
    if (entry.active) {
      actives.push(entry.descriptor)
    }
  }
  activeRuntimePlugins.value = actives
}

/** 绑定运行时宿主能力（loader 首次加载外部插件前调用；重复绑定以最新为准） */
export function setRuntimeHost(host: RuntimeHost): void {
  runtimeHost = host
}

/** loader 注入插件脚本前登记期望 ID：仅登记过的 ID 允许经 registerRuntime 注册 */
export function expectRuntimePlugin(id: string): void {
  expectedRuntimeIds.add(id)
}

/**
 * 报告指定 ID 是否已有运行时描述符缓存（含已停用条目）。
 * loader 据此区分"注册成功后停用"（缓存在，re-enable 直接复活）与
 * "注入途中被停用、注册被拒"（无缓存，需允许重新注入脚本）。
 */
export function hasRuntimeEntry(id: string): boolean {
  return runtimeEntries.has(id)
}

/** loader 注入前把 <script> 元素与插件 ID 绑定：registerRuntime 以元素身份认定归属 */
export function bindRuntimeScript(script: HTMLScriptElement, id: string): void {
  runtimeScriptOwners.set(script, id)
}

function activateEntry(id: string, entry: RuntimeEntry): void {
  if (entry.active || runtimeHost === null) {
    return
  }
  for (const route of entry.descriptor.routes ?? []) {
    // 宿主 addRoute 对与既有路由（核心或其他插件）name/path 冲突的注入抛错：
    // 逐条降级为 warn+跳过，坏路由不得顶替核心路由，也不拖垮同插件其余贡献。
    try {
      entry.removeRoutes.push(runtimeHost.addRoute(stampPluginId(route, id)))
    } catch (error) {
      console.warn(`[pluginkit] plugin "${id}" route "${route.path}" rejected:`, error)
    }
  }
  entry.active = true
  recomputeActiveRuntimePlugins()
}

function deactivateEntry(entry: RuntimeEntry): void {
  if (!entry.active) {
    return
  }
  for (const removeRoute of entry.removeRoutes) {
    removeRoute()
  }
  entry.removeRoutes = []
  entry.active = false
  recomputeActiveRuntimePlugins()
}

/**
 * 外部插件的运行时注册回调（挂载为 window.__SUB2API_PLUGIN_REGISTER__）。
 *
 * fail-closed：descriptor 非法 / ID 未在期望集合内 / 归属校验不过 /
 * 宿主未绑定，一律 console.warn 后忽略（不抛错，插件脚本的失败不得中断
 * 核心应用）；同 ID 重复注册幂等（首次注册生效，后续调用 warn 后忽略）。
 *
 * 归属契约：注册必须发生在 loader 注入的 <script> 同步求值期间——此时
 * document.currentScript 即该元素，且其经 bindRuntimeScript 绑定的 ID 必须
 * 等于 descriptor.id。异步回调（.then/setTimeout）里 currentScript 为 null，
 * 注册一律被拒：否则恶意插件可在异步时机抢注他插件的 ID（绑定校验只对
 * 同步调用生效的旧方案即毁于此）。
 *
 * 路由 meta.pluginId 复用 stampPluginId 强制覆写为 descriptor.id，
 * 与内建插件同一套"归属不可伪造"语义，守卫门控照常生效。
 */
export function registerRuntime(descriptor: PluginDescriptor): void {
  if (!descriptor || typeof descriptor.id !== 'string' || descriptor.id === '') {
    console.warn('[pluginkit] registerRuntime: invalid plugin descriptor, ignored')
    return
  }
  const id = descriptor.id
  if (!expectedRuntimeIds.has(id)) {
    console.warn(`[pluginkit] registerRuntime: "${id}" is not an expected external plugin, ignored`)
    return
  }
  const script = document.currentScript
  const boundId = script instanceof HTMLScriptElement ? runtimeScriptOwners.get(script) : undefined
  if (boundId !== id) {
    console.warn(
      `[pluginkit] registerRuntime: "${id}" must register synchronously from its own injected script` +
        (boundId !== undefined ? ` (caller script belongs to "${boundId}")` : ''),
      ', ignored'
    )
    return
  }
  if (runtimeHost === null) {
    console.warn(`[pluginkit] registerRuntime: runtime host not bound, "${id}" ignored`)
    return
  }
  if (runtimeEntries.has(id)) {
    // 幂等但不静默：真实注册已在册时的再次注册值得留痕（排障线索）。
    console.warn(`[pluginkit] registerRuntime: "${id}" already registered, duplicate ignored`)
    return
  }
  const entry: RuntimeEntry = { descriptor, active: false, removeRoutes: [] }
  runtimeEntries.set(id, entry)
  activateEntry(id, entry)
  // 文案对两种语言各 merge 一次立即生效；此后的 locale 懒加载路径会经
  // pluginMessages()（含运行时插件）覆盖式补回，两条路径收敛到同一结果。
  // 停用时不反向卸载文案（vue-i18n 无 unmerge），残留键无导航/路由可达，无害。
  if (descriptor.i18n) {
    for (const locale of ['zh', 'en'] as const) {
      runtimeHost.mergeLocaleMessage(locale, { plugins: { [id]: descriptor.i18n[locale] } })
    }
  }
}

/**
 * 按最新的"enabled 且带前端资产"的外部插件 ID 集合对账运行时激活状态
 * （loader 在每次 enabled 清单落盘后调用）：
 *   - 集合外：移除路由与导航贡献（描述符保留缓存），并撤销注册期望
 *     （在途脚本晚到的注册将被拒绝，fail-closed）；
 *   - 集合内的缓存条目：重新激活（脚本已加载过，无需重新注入）。
 */
export function syncRuntimeActivation(enabledIds: ReadonlySet<string>): void {
  for (const id of [...expectedRuntimeIds]) {
    if (!enabledIds.has(id)) {
      expectedRuntimeIds.delete(id)
    }
  }
  for (const [id, entry] of runtimeEntries) {
    if (enabledIds.has(id)) {
      activateEntry(id, entry)
    } else {
      deactivateEntry(entry)
    }
  }
}

/** 仅测试使用：清空全部运行时注册状态（宿主绑定/条目缓存/期望集合） */
export function _resetRuntimeForTest(): void {
  for (const entry of runtimeEntries.values()) {
    deactivateEntry(entry)
  }
  runtimeEntries.clear()
  expectedRuntimeIds.clear()
  runtimeHost = null
  activeRuntimePlugins.value = []
}

// 全局注册入口：外部插件 IIFE 脚本经它提交描述符。模块求值即挂载
// （任何接入点 import registry 都会就位）；期望集合为空时一切注册被拒，
// 未安装外部插件时该全局的存在不产生任何行为变更。
if (typeof window !== 'undefined') {
  window.__SUB2API_PLUGIN_REGISTER__ = registerRuntime
}
