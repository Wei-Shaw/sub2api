<template>
  <aside
    class="sidebar"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { '-translate-x-full lg:translate-x-0': !mobileOpen }
    ]"
  >
    <!-- Logo/Brand -->
    <div class="sidebar-header">
      <!-- Custom Logo or Default Logo -->
      <div class="flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl shadow-glow">
        <img v-if="settingsLoaded" :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
      </div>
      <transition name="fade">
        <div v-if="!sidebarCollapsed" class="flex flex-col">
          <span class="text-lg font-bold text-gray-900 dark:text-white">
            {{ siteName }}
          </span>
          <!-- Version Badge -->
          <VersionBadge :version="siteVersion" />
        </div>
      </transition>
    </div>

    <!-- Navigation -->
    <nav class="sidebar-nav scrollbar-hide">
      <!-- Admin View: Admin menu first, then personal menu -->
      <template v-if="isAdmin">
        <!-- Admin Section -->
        <div class="sidebar-section">
          <template v-for="item in adminNavItems" :key="item.path">
            <!-- Collapsible group (has children) -->
            <template v-if="item.children?.length">
              <button
                type="button"
                class="sidebar-link mb-1 w-full"
                :class="{ 'sidebar-link-active': isGroupActive(item) && !isGroupExpanded(item) }"
                :title="sidebarCollapsed ? item.label : undefined"
                @click="handleGroupClick(item)"
              >
                <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
                <component v-else-if="item.icon" :is="item.icon" class="h-5 w-5 flex-shrink-0" />
                <transition name="fade">
                  <span v-if="!sidebarCollapsed" class="flex flex-1 items-center justify-between">
                    <span>{{ item.label }}</span>
                    <ChevronDownIcon class="h-4 w-4 flex-shrink-0 transition-transform duration-200" :class="isGroupExpanded(item) ? 'rotate-180' : ''" />
                  </span>
                </transition>
              </button>
              <!-- Children -->
              <div v-if="!sidebarCollapsed && isGroupExpanded(item)" class="mb-1 ml-4 border-l border-gray-200 pl-2 dark:border-dark-600">
                <router-link
                  v-for="child in item.children"
                  :key="child.path"
                  :to="child.path"
                  class="sidebar-link mb-0.5 py-1.5 text-sm"
                  :class="{ 'sidebar-link-active': route.path === child.path }"
                  @click="handleMenuItemClick(child.path)"
                >
                  <!-- Bug 修复: 子菜单也支持 plugin 注入的 iconSvg, 与 normal item 保持
                       一致的 v-if/v-else 分支顺序; 都没有时不渲染图标占位避免对齐错位。 -->
                  <span v-if="child.iconSvg" class="h-4 w-4 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(child.iconSvg)"></span>
                  <component v-else-if="child.icon" :is="child.icon" class="h-4 w-4 flex-shrink-0" />
                  <span>{{ child.label }}</span>
                </router-link>
              </div>
            </template>
            <!-- Normal item (no children) -->
            <router-link
              v-else
              :to="item.path"
              class="sidebar-link mb-1"
              :class="{ 'sidebar-link-active': isActive(item.path) }"
              :title="sidebarCollapsed ? item.label : undefined"
              :id="
                item.path === '/admin/accounts'
                  ? 'sidebar-channel-manage'
                  : item.path === '/admin/groups'
                    ? 'sidebar-group-manage'
                    : item.path === '/admin/redeem'
                      ? 'sidebar-wallet'
                      : undefined
              "
              @click="handleMenuItemClick(item.path)"
            >
              <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
              <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
              <transition name="fade">
                <span v-if="!sidebarCollapsed">{{ item.label }}</span>
              </transition>
            </router-link>
          </template>
        </div>

        <!-- Personal Section for Admin (hidden in simple mode) -->
        <div v-if="!authStore.isSimpleMode" class="sidebar-section">
          <div v-if="!sidebarCollapsed" class="sidebar-section-title">
            {{ t('nav.myAccount') }}
          </div>
          <div v-else class="mx-3 my-3 h-px bg-gray-200 dark:bg-dark-700"></div>

          <router-link
            v-for="item in personalNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path) }"
            :title="sidebarCollapsed ? item.label : undefined"
            :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <transition name="fade">
              <span v-if="!sidebarCollapsed">{{ item.label }}</span>
            </transition>
          </router-link>
        </div>
      </template>

      <!-- Regular User View -->
      <template v-else-if="!appStore.backendModeEnabled">
        <div class="sidebar-section">
          <router-link
            v-for="item in userNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path) }"
            :title="sidebarCollapsed ? item.label : undefined"
            :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <transition name="fade">
              <span v-if="!sidebarCollapsed">{{ item.label }}</span>
            </transition>
          </router-link>
        </div>
      </template>
    </nav>

    <!-- Bottom Section -->
    <div class="mt-auto border-t border-gray-100 p-3 dark:border-dark-800">
      <!-- Theme Toggle -->
      <button
        @click="toggleTheme"
        class="sidebar-link mb-2 w-full"
        :title="sidebarCollapsed ? (isDark ? t('nav.lightMode') : t('nav.darkMode')) : undefined"
      >
        <SunIcon v-if="isDark" class="h-5 w-5 flex-shrink-0 text-amber-500" />
        <MoonIcon v-else class="h-5 w-5 flex-shrink-0" />
        <transition name="fade">
          <span v-if="!sidebarCollapsed">{{
            isDark ? t('nav.lightMode') : t('nav.darkMode')
          }}</span>
        </transition>
      </button>

      <!-- Collapse Button -->
      <button
        @click="toggleSidebar"
        class="sidebar-link w-full"
        :title="sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
      >
        <ChevronDoubleLeftIcon v-if="!sidebarCollapsed" class="h-5 w-5 flex-shrink-0" />
        <ChevronDoubleRightIcon v-else class="h-5 w-5 flex-shrink-0" />
        <transition name="fade">
          <span v-if="!sidebarCollapsed">{{ t('nav.collapse') }}</span>
        </transition>
      </button>
    </div>
  </aside>

  <!-- Mobile Overlay -->
  <transition name="fade">
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-30 bg-black/50 lg:hidden"
      @click="closeMobile"
    ></div>
  </transition>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import { sanitizeSvg } from '@/utils/sanitize'
import { getPluginMenuItems, type PluginNavItem } from '@/plugins/loader'
import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'

interface NavItem {
  path: string
  label: string
  icon: unknown
  iconSvg?: string
  hideInSimpleMode?: boolean
  children?: NavItem[]
  /**
   * When true, the parent item only toggles the expand/collapse state and
   * does NOT navigate to its `path`. The `path` is purely a stable key.
   */
  expandOnly?: boolean
  /**
   * Optional feature flag getter. Returns false to hide the item; undefined/true to show.
   * Uses lenient semantics (undefined -> show) to avoid menu flicker before settings load.
   */
  featureFlag?: () => boolean | undefined
  /**
   * V5/W7 Placement DSL bucket the host chose for this entry. Plugin items
   * carry their plugin-supplied placement_group; host items carry one of the
   * fixed buckets defined alongside `baseItems`. Undefined falls back to
   * `<section>/end` inside mergeByPlacement.
   */
  group?: string
  /**
   * Bug C 修复: host base item 的离散排序权重，让插件能精细插入。
   * 每条 base 项预留 10 的 step (10/20/30...), 插件 manifest 的
   * Placement.Order 可以落在缝隙里 (如 70 就排在 accounts=60 与
   * announcements=80 之间)。未声明则 mergeByPlacement 退回到原 idx。
   *
   * TODO: 将这些离散值抽取到 frontend/src/plugins/anchors.ts 的命名常量
   * (NAV_ADMIN_ACCOUNTS_ORDER 等), plugin 通过 import 引用而非硬编码数字。
   */
  placementOrder?: number
}

// resolvePluginLabel 选择菜单项的显示文本,优先级：
//   1. item.labels[当前 locale] —— 插件直接交付的翻译,核心无需感知
//   2. item.labels.en —— 任何语言都接受英文兜底
//   3. t(item.labelKey) —— 旧路径,核心 i18n 字典里若有翻译则用之
//   4. labelKey 末段的 humanized 形式 + 插件 display_name
// 这样新插件只需声明 labels 就能显示正确文本,不需要核心维护翻译表。
function resolvePluginLabel(item: PluginNavItem): string {
  const labels = item.labels || {}
  const currentLocale = (locale.value || '').toLowerCase()
  if (currentLocale) {
    const exact = labels[currentLocale]
    if (exact) {
      return exact
    }
    // locale 形如 zh-CN / en-US 时,允许按主语言 (zh / en) 命中。
    const primary = currentLocale.split(/[-_]/)[0]
    if (primary && primary !== currentLocale && labels[primary]) {
      return labels[primary]
    }
  }
  if (labels.en) {
    return labels.en
  }

  const key = item.labelKey || item.label
  if (!key) {
    return item.pluginDisplayName || ''
  }
  // key 不像 i18n 路径（不含点），直接当字面量显示。
  if (!key.includes('.')) {
    return key
  }
  const translated = t(key)
  if (translated && translated !== key) {
    return translated
  }
  // 翻译失败：用 key 末段拼上插件 display_name 给用户一个能读懂的标签。
  const lastSegment = key.split('.').pop() || key
  const humanized = lastSegment
    .replace(/[-_]+/g, ' ')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/^./, (c) => c.toUpperCase())
  if (item.pluginDisplayName && humanized.toLowerCase() !== item.pluginDisplayName.toLowerCase()) {
    return `${item.pluginDisplayName} · ${humanized}`
  }
  return humanized || item.pluginDisplayName || key
}

// 把插件菜单项转成 NavItem,丢弃只供排序使用的元数据字段。
function pluginItemsAsNavItems(items: PluginNavItem[]): NavItem[] {
  return items.map((item) => ({
    path: item.path,
    label: resolvePluginLabel(item),
    icon: item.icon,
    iconSvg: item.iconSvg,
    hideInSimpleMode: item.hideInSimpleMode,
    children: item.children ? pluginItemsAsNavItems(item.children) : undefined,
    // V5/W7: 把插件声明的 placement_group 投射到 NavItem.group; 缺省时
    // mergeByPlacement 会按 section 推 `${section}/end` fallback。
    group: item.placementGroup,
  }))
}

// V5/W7 Placement DSL — 把 host 的 baseItems 与插件菜单按 group 合并:
//   1. 每条 base 项的 group 已在声明时硬编码; 插件项的 group 来自
//      placement_group, 缺省 fallback 到 `${sectionPrefix}/end`.
//   2. 按 group 分桶, 桶内按 (placementOrder asc, originalIndex asc) 排序;
//      base 项的 placementOrder 视为 originalIndex (保持声明顺序), 这样无插件
//      时输出与原数组完全一致 (回归保护).
//   3. 按预定义桶顺序拼接: main -> system -> end.
//
// sectionPrefix 决定 fallback bucket 与桶顺序, 'admin' 用 admin/main 系列,
// 'user' 没有 system 桶 (只有 main / end).
function mergeByPlacement(
  base: NavItem[],
  pluginItems: PluginNavItem[],
  sectionPrefix: 'admin' | 'user',
): NavItem[] {
  type Annotated = NavItem & { __order: number; __seq: number }
  const fallback = `${sectionPrefix}/end`
  const annotated: Annotated[] = []

  base.forEach((item, idx) => {
    annotated.push({
      ...item,
      group: item.group || fallback,
      // Bug C 修复: host 项支持显式 placementOrder, 否则退回 idx 保持
      // 声明顺序 (兼容未声明 placementOrder 的旧 baseItems)。
      __order: item.placementOrder ?? idx,
      __seq: idx,
    })
  })

  const baseLen = base.length
  const pluginNavs = pluginItemsAsNavItems(pluginItems)
  pluginNavs.forEach((item, idx) => {
    const original = pluginItems[idx]
    annotated.push({
      ...item,
      group: original?.placementGroup || fallback,
      __order: original?.placementOrder ?? 9999,
      __seq: baseLen + idx,
    })
  })

  // 桶顺序: admin = main → system → end; user = main → end (system 桶在
  // user section 里没有意义, 但即便插件错误声明也会被拼到 end 之前, 不影响
  // 视觉)。
  const order = sectionPrefix === 'admin'
    ? [`${sectionPrefix}/main`, `${sectionPrefix}/system`, `${sectionPrefix}/end`]
    : [`${sectionPrefix}/main`, `${sectionPrefix}/system`, `${sectionPrefix}/end`]

  const buckets = new Map<string, Annotated[]>()
  for (const g of order) buckets.set(g, [])
  // 收集未在 order 中列出的桶(防御性, 不应发生), 按出现顺序追加到末尾。
  const extraGroups: string[] = []
  for (const item of annotated) {
    const g = item.group || fallback
    if (!buckets.has(g)) {
      buckets.set(g, [])
      extraGroups.push(g)
    }
    buckets.get(g)!.push(item)
  }

  const result: NavItem[] = []
  const flush = (g: string) => {
    const list = buckets.get(g)
    if (!list || list.length === 0) return
    list.sort((a, b) => {
      if (a.__order !== b.__order) return a.__order - b.__order
      return a.__seq - b.__seq
    })
    for (const item of list) {
      const { __order, __seq, ...clean } = item
      void __order
      void __seq
      result.push(clean)
    }
  }
  for (const g of order) flush(g)
  for (const g of extraGroups) flush(g)
  return result
}

const { t, locale } = useI18n()

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const isAdmin = computed(() => authStore.isAdmin)
const isDark = ref(document.documentElement.classList.contains('dark'))

// Track which parent nav groups are expanded
const expandedGroups = ref<Set<string>>(new Set())

// Site settings from appStore (cached, no flicker)
const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => appStore.siteLogo)
const siteVersion = computed(() => appStore.siteVersion)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

// SVG Icon Components

// applyFeatureFlags recursively filters out items where featureFlag() === false (including children).
// Uses !== false lenient semantics: undefined (settings not loaded) or true both count as visible.
function applyFeatureFlags(items: NavItem[]): NavItem[] {
  const out: NavItem[] = []
  for (const item of items) {
    if (item.featureFlag && item.featureFlag() === false) continue
    if (item.children) {
      out.push({ ...item, children: applyFeatureFlags(item.children) })
    } else {
      out.push(item)
    }
  }
  return out
}

const DashboardIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z'
        })
      ]
    )
}

const KeyIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z'
        })
      ]
    )
}

const ChartIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z'
        })
      ]
    )
}

const GiftIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21 11.25v8.25a1.5 1.5 0 01-1.5 1.5H5.25a1.5 1.5 0 01-1.5-1.5v-8.25M12 4.875A2.625 2.625 0 109.375 7.5H12m0-2.625V7.5m0-2.625A2.625 2.625 0 1114.625 7.5H12m0 0V21m-8.625-9.75h18c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125h-18c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z'
        })
      ]
    )
}

const UserIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z'
        })
      ]
    )
}

const UsersIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z'
        })
      ]
    )
}

const FolderIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z'
        })
      ]
    )
}

const CreditCardIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z'
        })
      ]
    )
}

const GlobeIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418'
        })
      ]
    )
}

const ServerIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z'
        })
      ]
    )
}

const BellIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75V9a6 6 0 10-12 0v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0'
        })
      ]
    )
}

const TicketIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M16.5 6v.75m0 3v.75m0 3v.75m0 3V18m-9-5.25h5.25M7.5 15h3M3.375 5.25c-.621 0-1.125.504-1.125 1.125v3.026a2.999 2.999 0 010 5.198v3.026c0 .621.504 1.125 1.125 1.125h17.25c.621 0 1.125-.504 1.125-1.125v-3.026a2.999 2.999 0 010-5.198V6.375c0-.621-.504-1.125-1.125-1.125H3.375z'
        })
      ]
    )
}

const CogIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z'
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z'
        })
      ]
    )
}

const PluginIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M14.25 6.087c0-.355.186-.676.401-.959.221-.29.349-.634.349-1.003 0-1.036-1.007-1.875-2.25-1.875s-2.25.84-2.25 1.875c0 .369.128.713.349 1.003.215.283.401.604.401.959v0a.64.64 0 01-.657.643 48.39 48.39 0 01-4.163-.3c.186 1.613.293 3.25.315 4.907a.656.656 0 01-.658.663v0c-.355 0-.676-.186-.959-.401a1.647 1.647 0 00-1.003-.349c-1.036 0-1.875 1.007-1.875 2.25s.84 2.25 1.875 2.25c.369 0 .713-.128 1.003-.349.283-.215.604-.401.959-.401v0c.31 0 .555.26.532.57a48.039 48.039 0 01-.642 5.056c1.518.19 3.058.309 4.616.354a.64.64 0 00.657-.643v0c0-.355-.186-.676-.401-.959a1.647 1.647 0 01-.349-1.003c0-1.035 1.008-1.875 2.25-1.875 1.243 0 2.25.84 2.25 1.875 0 .369-.128.713-.349 1.003-.215.283-.4.604-.4.959v0c0 .333.277.599.61.58a48.1 48.1 0 005.427-.63 48.05 48.05 0 00.582-4.717.532.532 0 00-.533-.57v0c-.355 0-.676.186-.959.401-.29.221-.634.349-1.003.349-1.035 0-1.875-1.007-1.875-2.25s.84-2.25 1.875-2.25c.37 0 .713.128 1.003.349.283.215.604.401.96.401v0a.656.656 0 00.658-.663 48.422 48.422 0 00-.37-5.36c-1.886.342-3.81.574-5.766.689a.578.578 0 01-.61-.58v0z'
        })
      ]
    )
}

const SunIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z'
        })
      ]
    )
}

const MoonIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z'
        })
      ]
    )
}

const ChevronDoubleLeftIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm18.75 4.5-7.5 7.5 7.5 7.5m-6-15L5.25 12l7.5 7.5'
        })
      ]
    )
}


const ChevronDoubleRightIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm5.25 4.5 7.5 7.5-7.5 7.5m6-15 7.5 7.5-7.5 7.5'
        })
      ]
    )
}


const OrderIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15a2.25 2.25 0 012.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25zM6.75 12h.008v.008H6.75V12zm0 3h.008v.008H6.75V15zm0 3h.008v.008H6.75V18z'
        })
      ]
    )
}

const OrderListIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z'
        })
      ]
    )
}
const ShieldIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'
        })
      ]
    )
}

const ChevronDownIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm19.5 8.25-7.5 7.5-7.5-7.5'
        })
      ]
    )
}

// Public-settings flags go through the registry in utils/featureFlags.ts,
// which handles the opt-in vs opt-out fallback when settings haven't loaded
// yet. Admin-only flags (not in public settings) stay inline below.
const flagPayment = makeSidebarFlag(FeatureFlags.payment)
const flagAffiliate = makeSidebarFlag(FeatureFlags.affiliate)
const flagRiskControl = makeSidebarFlag(FeatureFlags.riskControl)
const flagOpsMonitoring = () => adminSettingsStore.opsMonitoringEnabled

// User navigation items (for regular users)
const userNavItems = computed((): NavItem[] => {
  const baseItems: NavItem[] = [
    { path: '/dashboard', label: t('nav.dashboard'), icon: DashboardIcon, group: 'user/main', placementOrder: 10 },
    { path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon, group: 'user/main', placementOrder: 20 },
    { path: '/usage', label: t('nav.usage'), icon: ChartIcon, hideInSimpleMode: true, group: 'user/main', placementOrder: 30 },
    { path: '/subscriptions', label: t('nav.mySubscriptions'), icon: CreditCardIcon, hideInSimpleMode: true, group: 'user/main', placementOrder: 40 },
    { path: '/orders', label: t('nav.myOrders'), icon: OrderListIcon, hideInSimpleMode: true, featureFlag: flagPayment, group: 'user/main', placementOrder: 47 },
    { path: '/redeem', label: t('nav.redeem'), icon: GiftIcon, hideInSimpleMode: true, group: 'user/end', placementOrder: 20 },
    { path: '/affiliate', label: t('nav.affiliate'), icon: UsersIcon, hideInSimpleMode: true, featureFlag: flagAffiliate, group: 'user/end', placementOrder: 25 },
    { path: '/profile', label: t('nav.profile'), icon: UserIcon, group: 'user/end', placementOrder: 30 },
    ...customMenuItemsForUser.value.map((item, idx): NavItem => ({
      path: `/custom/${item.id}`,
      label: item.label,
      icon: null,
      iconSvg: item.icon_svg,
      group: 'user/end',
      // 自定义菜单永远跟在固有 user/end 项之后 (>=100), 但相互之间保持
      // 用户配置的 sort_order 顺序 (这里用 idx, customMenuItemsForUser 已按
      // sort_order 排序)。
      placementOrder: 100 + idx,
    })),
  ]
  const merged = mergeByPlacement(baseItems, pluginUserNavItems.value, 'user')
  const filtered = applyFeatureFlags(merged)
  return authStore.isSimpleMode ? filtered.filter(item => !item.hideInSimpleMode) : filtered
})

// 插件菜单项,启动时从 window.__PLUGIN_MANIFESTS__ 读取一次,后续不会变化。
const pluginAdminNavItems = computed(() => getPluginMenuItems('admin'))
const pluginUserNavItems = computed(() => getPluginMenuItems('user'))

// Personal navigation items (for admin's "My Account" section, without Dashboard)
const personalNavItems = computed((): NavItem[] => {
  // Bug C: 与 userNavItems 同步; 没有 dashboard 所以从 placementOrder=20 起步。
  const baseItems: NavItem[] = [
    { path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon, group: 'user/main', placementOrder: 20 },
    { path: '/usage', label: t('nav.usage'), icon: ChartIcon, hideInSimpleMode: true, group: 'user/main', placementOrder: 30 },
    { path: '/subscriptions', label: t('nav.mySubscriptions'), icon: CreditCardIcon, hideInSimpleMode: true, group: 'user/main', placementOrder: 40 },
    { path: '/orders', label: t('nav.myOrders'), icon: OrderListIcon, hideInSimpleMode: true, featureFlag: flagPayment, group: 'user/main', placementOrder: 47 },
    { path: '/redeem', label: t('nav.redeem'), icon: GiftIcon, hideInSimpleMode: true, group: 'user/end', placementOrder: 20 },
    { path: '/affiliate', label: t('nav.affiliate'), icon: UsersIcon, hideInSimpleMode: true, featureFlag: flagAffiliate, group: 'user/end', placementOrder: 25 },
    { path: '/profile', label: t('nav.profile'), icon: UserIcon, group: 'user/end', placementOrder: 30 },
    ...customMenuItemsForUser.value.map((item, idx): NavItem => ({
      path: `/custom/${item.id}`,
      label: item.label,
      icon: null,
      iconSvg: item.icon_svg,
      group: 'user/end',
      placementOrder: 100 + idx,
    })),
  ]
  const merged = mergeByPlacement(baseItems, pluginUserNavItems.value, 'user')
  const filtered = applyFeatureFlags(merged)
  return authStore.isSimpleMode ? filtered.filter(item => !item.hideInSimpleMode) : filtered
})

// Custom menu items filtered by visibility
const customMenuItemsForUser = computed(() => {
  const items = appStore.cachedPublicSettings?.custom_menu_items ?? []
  return items
    .filter((item) => item.visibility === 'user')
    .sort((a, b) => a.sort_order - b.sort_order)
})

const customMenuItemsForAdmin = computed(() => {
  return adminSettingsStore.customMenuItems
    .filter((item) => item.visibility === 'admin')
    .sort((a, b) => a.sort_order - b.sort_order)
})

// Admin navigation items
const adminNavItems = computed((): NavItem[] => {
  // V5/W7 Placement DSL: 业务主菜单走 admin/main, 系统类(/admin/plugins、
  // /admin/usage) 走 admin/system, 系统设置 + 自定义菜单走 admin/end.
  // mergeByPlacement 把插件项按 placement_group 插入对应桶。
  // Bug C 修复: 每条 base 项预留 step≈10 的离散 placementOrder, 让插件能精细
  // 插入。channel-management 插件以 70 落在 accounts (60) 与 announcements (80)
  // 之间, 形成"账号管理 → 渠道管理 → 公告"的核心业务相邻顺序。
  //
  // TODO: 把这些数字抽到 frontend/src/plugins/anchors.ts 的命名常量
  // (NAV_ADMIN_DASHBOARD_ORDER=10 等), 让插件 import 引用而非硬编码数字。
  const baseItems: NavItem[] = [
    { path: '/admin/dashboard', label: t('nav.dashboard'), icon: DashboardIcon, group: 'admin/main', placementOrder: 10 },
    { path: '/admin/ops', label: t('nav.ops'), icon: ChartIcon, featureFlag: flagOpsMonitoring, group: 'admin/main', placementOrder: 20 },
    { path: '/admin/users', label: t('nav.users'), icon: UsersIcon, hideInSimpleMode: true, group: 'admin/main', placementOrder: 30 },
    { path: '/admin/groups', label: t('nav.groups'), icon: FolderIcon, hideInSimpleMode: true, group: 'admin/main', placementOrder: 40 },
    // /admin/channels entry provided by channel-management plugin manifest (with pricing + monitor submenus).
    // When plugin is not loaded, host provides a fallback entry here. Plugin uses placementOrder=70.
    { path: '/admin/subscriptions', label: t('nav.subscriptions'), icon: CreditCardIcon, hideInSimpleMode: true, group: 'admin/main', placementOrder: 50 },
    { path: '/admin/accounts', label: t('nav.accounts'), icon: GlobeIcon, group: 'admin/main', placementOrder: 60 },
    { path: '/admin/announcements', label: t('nav.announcements'), icon: BellIcon, group: 'admin/main', placementOrder: 80 },
    { path: '/admin/proxies', label: t('nav.proxies'), icon: ServerIcon, group: 'admin/main', placementOrder: 90 },
    { path: '/admin/risk-control', label: t('nav.riskControl'), icon: ShieldIcon, hideInSimpleMode: true, featureFlag: flagRiskControl, group: 'admin/main', placementOrder: 95 },
    { path: '/admin/redeem', label: t('nav.redeemCodes'), icon: TicketIcon, hideInSimpleMode: true, group: 'admin/main', placementOrder: 100 },
    { path: '/admin/promo-codes', label: t('nav.promoCodes'), icon: GiftIcon, hideInSimpleMode: true, group: 'admin/main', placementOrder: 110 },
    {
      path: '/admin/affiliates',
      label: t('nav.affiliateManagement'),
      icon: UsersIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      featureFlag: flagAffiliate,
      group: 'admin/main',
      placementOrder: 120,
      children: [
        { path: '/admin/affiliates/invites', label: t('nav.affiliateInviteRecords'), icon: UsersIcon },
        { path: '/admin/affiliates/rebates', label: t('nav.affiliateRebateRecords'), icon: OrderIcon },
        { path: '/admin/affiliates/transfers', label: t('nav.affiliateTransferRecords'), icon: CreditCardIcon },
      ],
    },
    { path: '/admin/plugins', label: t('nav.plugins'), icon: PluginIcon, hideInSimpleMode: true, group: 'admin/system', placementOrder: 10 },
    { path: '/admin/usage', label: t('nav.usage'), icon: ChartIcon, group: 'admin/system', placementOrder: 20 },
  ]

  // 简单模式下,基础项按 mergeByPlacement 合并插件,再在末尾补充 API 密钥和
  // 系统设置 + 自定义菜单 (这两类不参与桶排序, 始终位于尾部, 用 push 而非
  // 重新跑一次 mergeByPlacement, 保持原有视觉与单元测试断言)。
  if (authStore.isSimpleMode) {
    const visiblePlugins = pluginAdminNavItems.value.filter((pl) => !pl.hideInSimpleMode)
    const visibleBase = baseItems.filter(item => !item.hideInSimpleMode)
    const merged = applyFeatureFlags(mergeByPlacement(visibleBase, visiblePlugins, 'admin'))
    merged.push({ path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon, group: 'admin/end' })
    merged.push({ path: '/admin/settings', label: t('nav.settings'), icon: CogIcon, group: 'admin/end' })
    for (const cm of customMenuItemsForAdmin.value) {
      merged.push({ path: `/custom/${cm.id}`, label: cm.label, icon: null, iconSvg: cm.icon_svg, group: 'admin/end' })
    }
    return merged
  }

  // 完整模式: 先合并 base + plugin, 再在 admin/end 桶尾追加系统设置 / 自定义
  // 菜单。系统设置语义上属于 admin/end, 但既有界面期待"始终在末尾", 所以仍
  // 用 push 而非声明在 baseItems 里。
  const merged = applyFeatureFlags(mergeByPlacement(baseItems, pluginAdminNavItems.value, 'admin'))
  merged.push({ path: '/admin/settings', label: t('nav.settings'), icon: CogIcon, group: 'admin/end' })
  for (const cm of customMenuItemsForAdmin.value) {
    merged.push({ path: `/custom/${cm.id}`, label: cm.label, icon: null, iconSvg: cm.icon_svg, group: 'admin/end' })
  }
  return merged
})

function toggleSidebar() {
  appStore.toggleSidebar()
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

function handleMenuItemClick(itemPath: string) {
  if (mobileOpen.value) {
    setTimeout(() => {
      appStore.setMobileOpen(false)
    }, 150)
  }

  // Map paths to tour selectors
  const pathToSelector: Record<string, string> = {
    '/admin/groups': '#sidebar-group-manage',
    '/admin/accounts': '#sidebar-channel-manage',
    '/keys': '[data-tour="sidebar-my-keys"]'
  }

  const selector = pathToSelector[itemPath]
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

// collectAllNavPaths 收集当前所有 sidebar nav 项及其 children 的 path 集合,
// 用于判断"最长前缀匹配"。包含 admin / personal / user / 子菜单全部条目。
function collectAllNavPaths(): string[] {
  const out: string[] = []
  const lists = [adminNavItems.value, personalNavItems.value, userNavItems.value]
  for (const list of lists) {
    for (const item of list) {
      out.push(item.path)
      if (item.children) {
        for (const child of item.children) {
          out.push(child.path)
        }
      }
    }
  }
  return out
}

// isActive 判断当前 nav item 是否应该高亮。
// 决策：使用"最长前缀匹配"——只有当前 path 是 route.path 的最长可用 nav 前缀时才返回 true。
// 这样既保留"访问深层路由时父菜单仍高亮"的体验,又避免父子菜单同时高亮(bug 修复)。
//   - 严格相等 → 始终 true
//   - startsWith 命中,但存在更长的 nav path 也命中 → false (让那个更具体的项独占高亮)
//   - startsWith 命中,且无更长匹配 → true
function isActive(path: string): boolean {
  if (route.path === path) return true
  if (!route.path.startsWith(path + '/')) return false
  const allPaths = collectAllNavPaths()
  return !allPaths.some(
    (p) => p !== path && p.length > path.length && (route.path === p || route.path.startsWith(p + '/'))
  )
}

function isGroupActive(item: NavItem): boolean {
  if (!item.children) return false
  return item.children.some(child => route.path === child.path)
}

function isGroupExpanded(item: NavItem): boolean {
  return expandedGroups.value.has(item.path) || isGroupActive(item)
}

/**
 * Click handler for collapsible parent items.
 * - When sidebar is collapsed: do nothing (children are not visible).
 * - When `expandOnly` is true: only toggle expand state.
 * - Otherwise: navigate to the parent path and ensure the group is expanded.
 */
function handleGroupClick(item: NavItem) {
  if (sidebarCollapsed.value) return
  if (item.expandOnly) {
    toggleGroup(item)
    return
  }
  // Push to path and ensure expanded
  if (route.path !== item.path) {
    router.push(item.path)
  }
  if (!expandedGroups.value.has(item.path)) {
    expandedGroups.value.add(item.path)
  }
}

function toggleGroup(item: NavItem) {
  if (expandedGroups.value.has(item.path)) {
    expandedGroups.value.delete(item.path)
  } else {
    expandedGroups.value.add(item.path)
  }
}

// Initialize theme
const savedTheme = localStorage.getItem('theme')
if (
  savedTheme === 'dark' ||
  (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
) {
  isDark.value = true
  document.documentElement.classList.add('dark')
}

// Fetch admin settings (for feature-gated nav items like Ops).
watch(
  isAdmin,
  (v) => {
    if (v) {
      adminSettingsStore.fetch()
    }
  },
  { immediate: true }
)

onMounted(() => {
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* Custom SVG icon in sidebar: constrain size without overriding uploaded SVG colors */
.sidebar-svg-icon {
  color: currentColor;
}

.sidebar-svg-icon :deep(svg) {
  display: block;
  width: 1.25rem;
  height: 1.25rem;
}
</style>
