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
  /** 旧字段：i18n key。新插件应留空并使用 labels。 */
  label_key: string
  /** 旧字段：图标名或 SVG。新插件应使用 icon_svg。 */
  icon: string
  /** 完整 SVG markup（含 <svg> 外层标签），由前端 v-html 直接注入。 */
  icon_svg: string
  /** locale -> 已翻译的菜单标签，如 {"zh": "渠道管理", "en": "Channel Management"}。 */
  labels: Record<string, string>
  /**
   * locale -> 已翻译的菜单描述, 如 {"zh": "管理渠道", "en": "Manage channels"}。
   * AppHeader 在 titleKey/labels 兜底链之后用 descriptions 渲染副标题, 让
   * plugin view 可以省去 PluginPageLayout 自己的 title/description。
   */
  descriptions: Record<string, string>
  section: 'admin' | 'user' | string
  sort_order: number
  requires_admin: boolean
  hide_in_simple_mode: boolean
  feature_flag: string
  /** V5/W7 Placement DSL bucket, e.g. "admin/main"。空字符串退化到 "<section>/end"。 */
  placement_group: string
  /** V5/W7 Placement bucket 内的排序权重；与 placement_group 同时设置才生效。 */
  placement_order: number
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
  /**
   * 完整可访问的 entry.js URL (HTTP). 后端通过 /api/v1/plugin-assets/<plugin>/<file> 暴露.
   * 旧后端不输出该字段时为空字符串, 前端按"无远程 entry"降级到占位页.
   */
  entry_js_url: string
  /** 可选 entry.css URL, 与 entry_js_url 同源同前缀. */
  entry_css_url: string
  /** 'shared' / 'iframe', 默认 'shared'. */
  isolation: 'shared' | 'iframe'
  /**
   * 可选: 插件自定义的 settings 视图组件名 (映射到 entry.js 的 VIEWS map).
   * 非空时, host 的 PluginSettingsForm 会挂载该组件代替默认的 JSON-schema 表单;
   * 空字符串 / 未设置时走老的 schema 渲染流程.
   */
  settings_component_path: string
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
  /** 插件 display_name,用于 i18n 找不到时的兜底显示 */
  pluginDisplayName: string
  /** 原始 label_key,sidebar 在渲染前会尝试 t(labelKey) */
  labelKey: string
  /** 插件直接交付的 locale -> 翻译文本映射,sidebar 优先按当前 locale 取值 */
  labels: Record<string, string>
  /** 插件直接交付的 locale -> 描述文本映射, AppHeader 兜底渲染副标题 */
  descriptions?: Record<string, string>
  /** 原始 sort_order,合并到主菜单时用作排序权重 */
  sortOrder: number
  /** V5/W7 Placement DSL bucket，可选；缺省由消费方按 section 推导 fallback。 */
  placementGroup?: string
  /** V5/W7 Placement bucket 内的排序权重，与 placementGroup 同时设置才生效。 */
  placementOrder?: number
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
      items.push(toNavItem(manifest.name, manifest.display_name, menu))
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
      // Header 标题/描述兜底:
      //   plugin 没填 RouteDecl.Meta.titleKey/descriptionKey 时, 把同 path
      //   的 menu item labels / descriptions 透传到 route.meta.pluginLabels
      //   / pluginDescriptions, AppHeader 据此按当前 i18n locale 选标题与
      //   副标题. 再往后还有 display_name 兜底.
      const matchedMenu = findMenuItemByPath(manifest.menu_items, route.path)
      const fallbackLabels = matchedMenu ? matchedMenu.labels : {}
      const fallbackDescriptions = matchedMenu ? matchedMenu.descriptions : {}
      // RouteDecl.Meta is map<string,string>, but the host router treats
      // requiresAuth/requiresAdmin as booleans. Coerce "false"/"true" so
      // plugins can declare standalone callback pages (Stripe / Alipay /
      // WeChat returnURLs) without going through every host gate.
      const pluginMeta = { ...route.meta } as Record<string, unknown>
      if (typeof pluginMeta.requiresAuth === 'string') {
        pluginMeta.requiresAuth = pluginMeta.requiresAuth.toLowerCase() === 'true'
      }
      if (typeof pluginMeta.requiresAdmin === 'string') {
        pluginMeta.requiresAdmin = pluginMeta.requiresAdmin.toLowerCase() === 'true'
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
          pluginLabels: fallbackLabels,
          pluginDescriptions: fallbackDescriptions,
          ...pluginMeta,
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

/**
 * 按名字查找已注入的 plugin manifest. 用于 PluginView 在路由切换时拿到 entry_js_url.
 * 没找到时返回 null. 不会读 window.__PLUGIN_MANIFESTS__ 多次, 而是每次都重新解析,
 * 因为该全局变量是 SSR 注入只读的, 解析成本低且更安全.
 */
export function findPluginManifest(name: string): PluginManifest | null {
  if (!name) {
    return null
  }
  for (const m of getPluginManifests()) {
    if (m.name === name) {
      return m
    }
  }
  return null
}

// ------------------------------------------------------------------
// 内部工具
// ------------------------------------------------------------------

// 查找与给定 path 匹配的 menu item, 用于 AppHeader 标题/描述兜底.
// 当 parent 与某个 child 声明同一个 path (例如 channel-management 把
// "/admin/channels" 同时挂在 parent "渠道管理" 和 child "渠道定价" 上,
// 用 parent 仅作为 sidebar 分组容器, child 才是真实页面) 时, parent
// 的 labels 通常是分组名 (与原版 host route.meta.title 一致), child
// 的 descriptions 才是具体页面副标题 (与原版 descriptionKey 对齐).
// 因此匹配到 self 后, 若 self 缺 labels 或 descriptions, 用第一个同
// path 的 child 补全, 让 AppHeader 能同时拿到 title + description.
function findMenuItemByPath(items: PluginMenuItem[], path: string): PluginMenuItem | null {
  for (const item of items) {
    if (item.path === path) {
      return mergeMenuItemFromSamePathChild(item, path)
    }
    if (item.children && item.children.length > 0) {
      const child = findMenuItemByPath(item.children, path)
      if (child) {
        return child
      }
    }
  }
  return null
}

// 当 self 与某个 child 同 path 时, 把 child 的 labels / descriptions 中
// self 缺失的字段合并进来. self 的字段优先 (空 map 视为缺失). 不修改
// 输入对象, 仅在需要合并时返回新对象.
function mergeMenuItemFromSamePathChild(
  self: PluginMenuItem,
  path: string,
): PluginMenuItem {
  if (!self.children || self.children.length === 0) {
    return self
  }
  const samePathChild = self.children.find((c) => c.path === path)
  if (!samePathChild) {
    return self
  }
  const selfHasLabels = self.labels && Object.keys(self.labels).length > 0
  const selfHasDescriptions = self.descriptions && Object.keys(self.descriptions).length > 0
  const childHasLabels = samePathChild.labels && Object.keys(samePathChild.labels).length > 0
  const childHasDescriptions =
    samePathChild.descriptions && Object.keys(samePathChild.descriptions).length > 0
  if ((selfHasLabels || !childHasLabels) && (selfHasDescriptions || !childHasDescriptions)) {
    return self
  }
  return {
    ...self,
    labels: selfHasLabels ? self.labels : samePathChild.labels,
    descriptions: selfHasDescriptions ? self.descriptions : samePathChild.descriptions,
  }
}

function normalizeManifest(value: unknown): PluginManifest | null {
  if (!isRecord(value)) {
    return null
  }
  const name = stringField(value, 'name')
  if (!name) {
    return null
  }
  const isolationRaw = stringField(value, 'isolation')
  const isolation: 'shared' | 'iframe' = isolationRaw === 'iframe' ? 'iframe' : 'shared'
  return {
    name,
    display_name: stringField(value, 'display_name') || name,
    version: stringField(value, 'version'),
    description: stringField(value, 'description'),
    menu_items: normalizeMenuItems(value['menu_items']),
    routes: normalizeRoutes(value['routes']),
    entry_js_url: stringField(value, 'entry_js_url'),
    entry_css_url: stringField(value, 'entry_css_url'),
    isolation,
    settings_component_path: stringField(value, 'settings_component_path'),
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
    icon_svg: stringField(value, 'icon_svg'),
    labels: normalizeStringMap(value['labels']),
    descriptions: normalizeStringMap(value['descriptions']),
    section: stringField(value, 'section') || 'user',
    sort_order: numberField(value, 'sort_order'),
    requires_admin: booleanField(value, 'requires_admin'),
    hide_in_simple_mode: booleanField(value, 'hide_in_simple_mode'),
    feature_flag: stringField(value, 'feature_flag'),
    placement_group: stringField(value, 'placement_group'),
    placement_order: numberField(value, 'placement_order'),
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

// looksLikeSvg 判断字符串是否为可注入的 SVG 标记。仅在以 <svg 起始时认为是真实
// SVG markup,否则视为图标名/字体图标占位（当前没有图标库映射,只能丢弃）。
function looksLikeSvg(value: string): boolean {
  return /^\s*<svg[\s>]/i.test(value)
}

// pickIconSvg 解析菜单项的图标。优先使用 SDK 新增的 icon_svg；为兼容老插件,
// 当 icon_svg 为空但 icon 看起来像 SVG markup 时,把 icon 当 SVG 使用。
function pickIconSvg(menu: PluginMenuItem): string | undefined {
  if (menu.icon_svg && looksLikeSvg(menu.icon_svg)) {
    return menu.icon_svg
  }
  if (menu.icon && looksLikeSvg(menu.icon)) {
    return menu.icon
  }
  return undefined
}

function toNavItem(pluginName: string, pluginDisplayName: string, menu: PluginMenuItem): PluginNavItem {
  const labelKey = menu.label_key
  const item: PluginNavItem = {
    path: menu.path,
    // label 的实际 i18n 解析放到 sidebar(基于 labels + t(labelKey))。
    // 这里先存原始 key,同时把 plugin display_name 与 labels 一起带上,
    // 便于 sidebar 根据当前 locale 选择翻译文本或在翻译失败时兜底。
    label: labelKey,
    icon: null,
    iconSvg: pickIconSvg(menu),
    hideInSimpleMode: menu.hide_in_simple_mode,
    pluginName,
    pluginDisplayName,
    labelKey,
    labels: menu.labels,
    // descriptions 在 sidebar 不直接使用 (sidebar 不显示副标题), 但保留是
    // 为了与 labels 对称, 后续如有 tooltip / hover 等需求可直接读取。
    // AppHeader 走 route.meta.pluginDescriptions, 不依赖 PluginNavItem。
    descriptions: Object.keys(menu.descriptions || {}).length > 0 ? menu.descriptions : undefined,
    sortOrder: menu.sort_order,
    // V5/W7 Placement DSL — 透传给 sidebar mergeByPlacement 使用，缺省为
    // 空串时表示走 fallback bucket。
    placementGroup: menu.placement_group || undefined,
    placementOrder: menu.placement_group ? menu.placement_order : undefined,
  }
  if (menu.children && menu.children.length > 0) {
    item.children = menu.children.map((c) => toNavItem(pluginName, pluginDisplayName, c))
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
