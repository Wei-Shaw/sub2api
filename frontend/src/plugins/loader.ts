/**
 * 插件清单加载器
 *
 * 后端通过 `embed_on.go` 在 index.html 注入 `window.__PLUGIN_MANIFESTS__`,
 * 本模块负责安全读取这一全局变量,并把插件菜单/路由暴露给前端框架。
 *
 * 注意:
 * - 所有读取都是只读且 defensive 的,任何字段缺失都会被忽略而非抛错。
 * - 当前阶段每个插件只注册一个占位路由 `/plugins/:pluginName/:pageName?`,
 *   真正的页面渲染由 `views/plugin/PluginView.vue` 处理。
 */

import type { Router, RouteRecordRaw } from 'vue-router'

/** 插件菜单项,与后端 proto 中的 MenuItem 保持一致 */
export interface PluginMenuItem {
  path: string
  label_key: string
  icon: string
  section: 'admin' | 'user' | string
  sort_order: number
  requires_admin: boolean
  hide_in_simple_mode: boolean
  feature_flag: string
  children?: PluginMenuItem[]
}

/** 插件路由定义 */
export interface PluginRouteDefinition {
  path: string
  name: string
  component_path: string
  meta: Record<string, string>
}

/** 插件清单,与 backend/internal/plugin/grpc 中 ManifestResponse 的前端可见部分对齐 */
export interface PluginManifest {
  name: string
  display_name: string
  version: string
  description: string
  menu_items: PluginMenuItem[]
  routes: PluginRouteDefinition[]
}

/** 给 AppSidebar 使用的菜单条目结构,字段命名与现有 NavItem 一致 */
export interface PluginNavItem {
  path: string
  label: string
  icon: null
  iconSvg?: string
  hideInSimpleMode?: boolean
  /** 插件名,便于后续按插件维度做隐藏/排序 */
  pluginName: string
  /** 原始 sort_order,合并到主菜单时用作排序权重 */
  sortOrder: number
  children?: PluginNavItem[]
}

declare global {
  // eslint-disable-next-line @typescript-eslint/consistent-type-definitions
  interface Window {
    __PLUGIN_MANIFESTS__?: unknown
  }
}

/**
 * 读取 window.__PLUGIN_MANIFESTS__ 并做严格的类型校验。
 * 返回的 manifest 数组保证字段类型正确,但内容信任后端注入。
 */
export function getPluginManifests(): PluginManifest[] {
  const raw = typeof window !== 'undefined' ? window.__PLUGIN_MANIFESTS__ : undefined
  if (!Array.isArray(raw)) {
    return []
  }

  const out: PluginManifest[] = []
  for (const item of raw) {
    const manifest = normalizeManifest(item)
    if (manifest) {
      out.push(manifest)
    }
  }
  return out
}

/**
 * 获取指定 section 的所有插件菜单项,已扁平转换为 NavItem 兼容结构。
 * 同 section 内按 sort_order 升序排列;sort_order 相同则按 plugin name + path 维持稳定顺序。
 */
export function getPluginMenuItems(section: 'admin' | 'user'): PluginNavItem[] {
  const manifests = getPluginManifests()
  const items: PluginNavItem[] = []

  for (const manifest of manifests) {
    for (const menu of manifest.menu_items) {
      if (menu.section !== section) {
        continue
      }
      items.push(toNavItem(manifest.name, menu))
    }
  }

  items.sort((a, b) => {
    if (a.sortOrder !== b.sortOrder) {
      return a.sortOrder - b.sortOrder
    }
    if (a.pluginName !== b.pluginName) {
      return a.pluginName.localeCompare(b.pluginName)
    }
    return a.path.localeCompare(b.path)
  })

  return items
}

/**
 * 把每个插件的路由注册到 vue-router。
 *
 * 当前实现:
 * - 每个 manifest.routes 中声明的路由都映射到 `PluginView.vue`,通过 `meta.pluginName`
 *   和 `meta.componentPath` 让组件知道要渲染哪个插件的哪个页面。
 * - 同时为每个插件兜底注册一条 `/plugins/:pluginName/:pageName?` 路由,
 *   方便外部链接直接进入插件占位页。
 *
 * 真正的远程组件加载/iframe 注入留到后续迭代。
 */
export function registerPluginRoutes(router: Router): void {
  const manifests = getPluginManifests()
  if (manifests.length === 0) {
    return
  }

  const PluginViewLoader = () => import('@/views/plugin/PluginView.vue')

  for (const manifest of manifests) {
    for (const route of manifest.routes) {
      if (!route.path) {
        continue
      }
      const record: RouteRecordRaw = {
        path: route.path,
        name: route.name || `Plugin_${manifest.name}_${route.path}`,
        component: PluginViewLoader,
        meta: {
          requiresAuth: true,
          pluginName: manifest.name,
          pluginDisplayName: manifest.display_name,
          componentPath: route.component_path,
          ...route.meta,
        },
      }
      // 避免与已注册路由名重名:同名时跳过并给出 console 警告。
      if (record.name && router.hasRoute(record.name as string)) {
        // eslint-disable-next-line no-console
        console.warn(`[plugin loader] route name conflict, skipping: ${String(record.name)}`)
        continue
      }
      router.addRoute(record)
    }

    // 兜底通配路由,使 /plugins/<name> 总能跳到占位页。
    const fallbackName = `Plugin_${manifest.name}_Fallback`
    if (!router.hasRoute(fallbackName)) {
      router.addRoute({
        path: `/plugins/${manifest.name}/:pageName?`,
        name: fallbackName,
        component: PluginViewLoader,
        meta: {
          requiresAuth: true,
          pluginName: manifest.name,
          pluginDisplayName: manifest.display_name,
        },
      })
    }
  }
}

// ------------------------------------------------------------------
// 内部工具
// ------------------------------------------------------------------

function normalizeManifest(value: unknown): PluginManifest | null {
  if (!isRecord(value)) {
    return null
  }
  const name = stringField(value, 'name')
  if (!name) {
    return null
  }
  return {
    name,
    display_name: stringField(value, 'display_name') || name,
    version: stringField(value, 'version'),
    description: stringField(value, 'description'),
    menu_items: normalizeMenuItems(value['menu_items']),
    routes: normalizeRoutes(value['routes']),
  }
}

function normalizeMenuItems(value: unknown): PluginMenuItem[] {
  if (!Array.isArray(value)) {
    return []
  }
  const out: PluginMenuItem[] = []
  for (const item of value) {
    const menu = normalizeMenuItem(item)
    if (menu) {
      out.push(menu)
    }
  }
  return out
}

function normalizeMenuItem(value: unknown): PluginMenuItem | null {
  if (!isRecord(value)) {
    return null
  }
  const path = stringField(value, 'path')
  if (!path) {
    return null
  }
  return {
    path,
    label_key: stringField(value, 'label_key') || path,
    icon: stringField(value, 'icon'),
    section: stringField(value, 'section') || 'user',
    sort_order: numberField(value, 'sort_order'),
    requires_admin: booleanField(value, 'requires_admin'),
    hide_in_simple_mode: booleanField(value, 'hide_in_simple_mode'),
    feature_flag: stringField(value, 'feature_flag'),
    children: normalizeMenuItems(value['children']),
  }
}

function normalizeRoutes(value: unknown): PluginRouteDefinition[] {
  if (!Array.isArray(value)) {
    return []
  }
  const out: PluginRouteDefinition[] = []
  for (const item of value) {
    if (!isRecord(item)) {
      continue
    }
    const path = stringField(item, 'path')
    if (!path) {
      continue
    }
    out.push({
      path,
      name: stringField(item, 'name'),
      component_path: stringField(item, 'component_path'),
      meta: normalizeStringMap(item['meta']),
    })
  }
  return out
}

function toNavItem(pluginName: string, menu: PluginMenuItem): PluginNavItem {
  const item: PluginNavItem = {
    path: menu.path,
    // label_key 暂时直接显示;真正的 i18n 解析放到后续迭代,届时由 plugin 提供 i18n 资源。
    label: menu.label_key,
    icon: null,
    iconSvg: menu.icon || undefined,
    hideInSimpleMode: menu.hide_in_simple_mode,
    pluginName,
    sortOrder: menu.sort_order,
  }
  if (menu.children && menu.children.length > 0) {
    item.children = menu.children.map((c) => toNavItem(pluginName, c))
  }
  return item
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringField(obj: Record<string, unknown>, key: string): string {
  const v = obj[key]
  return typeof v === 'string' ? v : ''
}

function numberField(obj: Record<string, unknown>, key: string): number {
  const v = obj[key]
  if (typeof v === 'number' && Number.isFinite(v)) {
    return v
  }
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : 0
  }
  return 0
}

function booleanField(obj: Record<string, unknown>, key: string): boolean {
  return obj[key] === true
}

function normalizeStringMap(value: unknown): Record<string, string> {
  if (!isRecord(value)) {
    return {}
  }
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(value)) {
    if (typeof v === 'string') {
      out[k] = v
    }
  }
  return out
}
