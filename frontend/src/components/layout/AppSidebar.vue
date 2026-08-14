<template>
  <aside
    ref="sidebarRef"
    class="sidebar"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { '-translate-x-full lg:translate-x-0': !mobileOpen }
    ]"
    :role="drawerBlocking ? 'dialog' : undefined"
    :aria-modal="drawerBlocking ? 'true' : undefined"
    :aria-label="t('common.primaryNavigation')"
    @keydown="onKeydown"
  >
    <!-- Brand -->
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': sidebarCollapsed }">
      <router-link
        :to="homePath"
        class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded transition-opacity hover:opacity-80"
      >
        <img v-if="settingsLoaded" :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
      </router-link>
      <div
        class="sidebar-brand"
        :class="{ 'sidebar-brand-collapsed': sidebarCollapsed }"
        :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
      >
        <router-link :to="homePath" class="sidebar-brand-title text-base font-semibold text-ink transition-colors hover:text-accent">
          {{ siteName }}
        </router-link>
        <VersionBadge :version="siteVersion" />
      </div>
    </div>

    <!-- Navigation -->
    <nav ref="sidebarNavRef" class="sidebar-nav scrollbar-hide">
      <div v-for="section in sections" :key="section.key" class="sidebar-section">
        <div
          v-if="section.title"
          class="sidebar-section-title"
          :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >
          <span
            class="sidebar-section-title-text"
            :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }"
          >
            {{ section.title }}
          </span>
        </div>

        <template v-for="item in section.items" :key="item.path">
          <!-- Collapsible group -->
          <template v-if="item.children?.length">
            <button
              type="button"
              class="sidebar-link mb-1 w-full"
              :class="{
                'sidebar-link-active': isGroupActive(item) && !isGroupExpanded(item),
                'sidebar-link-collapsed': sidebarCollapsed
              }"
              :title="sidebarCollapsed ? item.label : undefined"
              :aria-expanded="isGroupExpanded(item)"
              @click="handleGroupClick(item)"
            >
              <NavItemIcon :item="item" />
              <span
                class="sidebar-label sidebar-label-flex"
                :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
                :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
              >
                <span class="min-w-0 truncate">{{ item.label }}</span>
                <Icon
                  name="chevronDown"
                  size="sm"
                  class="flex-shrink-0 transition-transform duration-base ease-out"
                  :class="isGroupExpanded(item) ? 'rotate-180' : ''"
                />
              </span>
            </button>

            <div v-if="!sidebarCollapsed && isGroupExpanded(item)" class="mb-1 ml-4 border-l border-line pl-2">
              <router-link
                v-for="child in item.children"
                :key="child.path"
                :id="child.domId"
                :to="child.path"
                class="sidebar-link mb-0.5 py-1.5 text-sm"
                :class="{ 'sidebar-link-active': route.path === child.path }"
                :data-tour="child.tourAnchor"
                @click="handleMenuItemClick(child)"
              >
                <NavItemIcon :item="child" size="sm" />
                <span>{{ child.label }}</span>
              </router-link>
            </div>
          </template>

          <!-- Leaf -->
          <router-link
            v-else
            :id="item.domId"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined"
            :data-tour="item.tourAnchor"
            @click="handleMenuItemClick(item)"
          >
            <NavItemIcon :item="item" />
            <span
              class="sidebar-label"
              :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
              :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
            >{{ item.label }}</span>
          </router-link>
        </template>
      </div>
    </nav>

    <!-- Utilities -->
    <div class="mt-auto border-t border-line p-3">
      <button
        type="button"
        class="sidebar-link mb-2 w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? themeLabel : undefined"
        @click="toggleTheme"
      >
        <Icon :name="isDark ? 'sun' : 'moon'" size="md" class="flex-shrink-0" />
        <span
          class="sidebar-label"
          :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >{{ themeLabel }}</span>
      </button>

      <button
        type="button"
        class="sidebar-link w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
        @click="toggleSidebar"
      >
        <Icon :name="sidebarCollapsed ? 'chevronRight' : 'chevronLeft'" size="md" class="flex-shrink-0" />
        <span
          class="sidebar-label"
          :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
          :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
        >{{ t('nav.collapse') }}</span>
      </button>
    </div>
  </aside>

  <!-- Mobile scrim -->
  <transition name="fade">
    <div v-if="mobileOpen" class="fixed inset-0 z-30 bg-black/50 lg:hidden" @click="closeMobile"></div>
  </transition>
</template>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import { navIcons } from '@/components/icons/nav'
import { sanitizeSvg } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
import { useTheme } from '@/composables/useTheme'
import { buildAdminNavItems, buildSelfNavItems, finalizeNav, tourSelectorFor } from './navTree'
import type { NavContext, NavItem } from './navTree'

/**
 * Presentation only. The nav tree lives in `navTree.ts` and the icons are real
 * components under `components/icons/nav/`; this file used to carry both, at
 * 1,084 lines, ~500 of which were `h('svg', ...)` render functions.
 */

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const { isDark, toggleTheme } = useTheme()

const sidebarRef = ref<HTMLElement | null>(null)
const sidebarNavRef = ref<HTMLElement | null>(null)
const expandedGroups = ref<Set<string>>(new Set())

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const isAdmin = computed(() => authStore.isAdmin)
const homePath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const themeLabel = computed(() => (isDark.value ? t('nav.lightMode') : t('nav.darkMode')))

const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteVersion = computed(() => appStore.siteVersion)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

/**
 * One nav item icon: the sanitised custom SVG when an admin uploaded one,
 * otherwise the registry component named by `item.icon`.
 */
const NavItemIcon = (props: { item: NavItem; size?: 'sm' | 'md' }) => {
  const box = props.size === 'sm' ? 'h-4 w-4 flex-shrink-0' : 'h-5 w-5 flex-shrink-0'
  if (props.item.iconSvg) {
    return h('span', { class: `${box} sidebar-svg-icon`, innerHTML: sanitizeSvg(props.item.iconSvg) })
  }
  const component = props.item.icon ? navIcons[props.item.icon] : null
  return component ? h(component, { class: box }) : null
}

// Public-settings flags resolve through the registry in utils/featureFlags.ts,
// which owns the opt-in vs opt-out fallback while settings are still loading.
// Admin-only flags are absent from public settings, so they stay inline.
const navFlags = {
  channelMonitor: makeSidebarFlag(FeatureFlags.channelMonitor),
  payment: makeSidebarFlag(FeatureFlags.payment),
  availableChannels: makeSidebarFlag(FeatureFlags.availableChannels),
  affiliate: makeSidebarFlag(FeatureFlags.affiliate),
  riskControl: makeSidebarFlag(FeatureFlags.riskControl),
  opsMonitoring: () => adminSettingsStore.opsMonitoringEnabled,
  adminPayment: () => adminSettingsStore.paymentEnabled,
  batchImageAccess: () => canUseBatchImage.value,
}

const navContext = computed((): NavContext => ({
  t,
  flags: navFlags,
  userCustomMenuItems: appStore.cachedPublicSettings?.custom_menu_items ?? [],
  adminCustomMenuItems: adminSettingsStore.customMenuItems,
  simpleMode: authStore.isSimpleMode,
}))

interface NavSection {
  key: string
  title?: string
  items: NavItem[]
}

const sections = computed((): NavSection[] => {
  const ctx = navContext.value

  if (isAdmin.value) {
    const out: NavSection[] = [{ key: 'admin', items: buildAdminNavItems(ctx) }]
    if (!ctx.simpleMode) {
      out.push({
        key: 'personal',
        title: t('nav.myAccount'),
        items: finalizeNav(buildSelfNavItems(ctx, false), ctx.simpleMode),
      })
    }
    return out
  }

  // Backend mode exposes no user-facing navigation at all.
  if (appStore.backendModeEnabled) return []

  return [{ key: 'user', items: finalizeNav(buildSelfNavItems(ctx, true), ctx.simpleMode) }]
})

function toggleSidebar() {
  appStore.toggleSidebar()
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

/**
 * Advance the onboarding tour when the clicked item IS the current step target.
 * The selector comes from the item now, not from a hand-maintained path map.
 */
function handleMenuItemClick(item: NavItem) {
  const selector = tourSelectorFor(item)
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function isGroupActive(item: NavItem): boolean {
  if (!item.children) return false
  return item.children.some((child) => route.path === child.path)
}

function isGroupExpanded(item: NavItem): boolean {
  return expandedGroups.value.has(item.path) || isGroupActive(item)
}

function toggleGroup(item: NavItem) {
  if (expandedGroups.value.has(item.path)) expandedGroups.value.delete(item.path)
  else expandedGroups.value.add(item.path)
}

/**
 * Parent click semantics:
 * - collapsed rail: nothing, since the children are not visible;
 * - `expandOnly`: toggle only (/admin/channels, /admin/security-audit);
 * - otherwise navigate to the parent path and expand (/admin/orders).
 */
function handleGroupClick(item: NavItem) {
  if (sidebarCollapsed.value) return
  if (item.expandOnly) {
    toggleGroup(item)
    return
  }
  if (route.path !== item.path) {
    void router.push(item.path)
  }
  expandedGroups.value.add(item.path)
}

/* ---------------------------------------------------------------------------
 * Mobile drawer
 *
 * Below `lg` the sidebar is an off-canvas dialog laid over the page, and it had
 * none of the behaviour that implies: Tab walked straight out into the page
 * behind the scrim, Escape did nothing, and nothing announced it as modal.
 * ------------------------------------------------------------------------- */

const isDesktop = useMediaQuery('(min-width: 1024px)')
const drawerBlocking = computed(() => mobileOpen.value && !isDesktop.value)

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])'
let lastFocused: HTMLElement | null = null

function focusables(): HTMLElement[] {
  if (!sidebarRef.value) return []
  return Array.from(sidebarRef.value.querySelectorAll<HTMLElement>(FOCUSABLE))
}

function onKeydown(event: KeyboardEvent) {
  if (!drawerBlocking.value || event.key !== 'Tab') return
  const items = focusables()
  if (!items.length) return

  const first = items[0]
  const last = items[items.length - 1]
  const active = document.activeElement as HTMLElement | null

  if (event.shiftKey && (active === first || !sidebarRef.value?.contains(active))) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}

// Escape is bound on the document, not the aside: focus can sit on the scrim.
function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && drawerBlocking.value) closeMobile()
}

watch(drawerBlocking, async (blocking) => {
  if (blocking) {
    lastFocused = document.activeElement as HTMLElement | null
    await nextTick()
    focusables()[0]?.focus()
  } else if (lastFocused) {
    lastFocused.focus?.()
    lastFocused = null
  }
})

// Closing on navigation belongs to the router, not to a 150ms setTimeout in the
// click handler: that timing raced any navigation slower than itself, and it
// missed every close that did not begin with a sidebar click.
const stopAfterEach = router.afterEach(() => {
  if (appStore.mobileOpen) appStore.setMobileOpen(false)
})

// Admin settings gate the Ops and payment nav entries.
watch(
  isAdmin,
  (value) => {
    if (value) adminSettingsStore.fetch()
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', onDocumentKeydown)
  void refreshBatchImageAccess()
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
  // Restore sidebar scroll position after a route change re-mounts the component
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onDocumentKeydown)
  stopAfterEach()
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  white-space: nowrap;
  transition:
    max-width var(--ds-dur-slow) var(--ds-ease-std),
    opacity var(--ds-dur-base) var(--ds-ease-std),
    transform var(--ds-dur-base) var(--ds-ease-std);
  max-width: 12rem;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity var(--ds-dur-base) var(--ds-ease-std),
    transform var(--ds-dur-base) var(--ds-ease-std);
}

/* Collapsed rail: the section label becomes a rule, so grouping survives. */
.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(var(--ds-line));
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity var(--ds-dur-base) var(--ds-ease-std);
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
  transition-delay: 0.08s;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width var(--ds-dur-slow) var(--ds-ease-std),
    opacity var(--ds-dur-fast) var(--ds-ease-std),
    transform var(--ds-dur-fast) var(--ds-ease-std);
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

/* Custom SVG icon: constrain the box without overriding uploaded colors. */
.sidebar-svg-icon {
  color: currentColor;
}

.sidebar-svg-icon :deep(svg) {
  display: block;
  width: 1.25rem;
  height: 1.25rem;
}
</style>
