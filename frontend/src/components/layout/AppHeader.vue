<template>
  <!-- Opaque surface + one hairline. Was `glass`: backdrop-blur over a
       translucent white, plus a half-opacity border. -->
  <header class="sticky top-0 z-30 border-b border-line bg-surface">
    <div class="flex h-16 items-center justify-between gap-2 px-2 sm:px-4 md:px-6">
      <!-- Left: mobile menu toggle + page title -->
      <div class="flex min-w-0 shrink items-center gap-2 sm:gap-4">
        <button
          type="button"
          class="btn-ghost btn-icon lg:hidden"
          :aria-label="t('common.toggleMenu')"
          @click="toggleMobileSidebar"
        >
          <Icon name="menu" size="md" />
        </button>

        <div class="hidden min-w-0 lg:block">
          <h1 class="truncate text-base font-semibold text-ink">{{ pageTitle }}</h1>
          <p v-if="pageDescription" class="truncate text-xs text-ink-tertiary">{{ pageDescription }}</p>
        </div>
      </div>

      <!-- Right: one row, three groups, separated by rules rather than six
           differently-styled widgets sitting next to each other. -->
      <div class="flex min-w-0 items-center gap-1 md:gap-2">
        <!-- Group 1: links. Collapses into an overflow menu below `md`, where
             the row would otherwise not fit. -->
        <template v-if="quickLinks.length">
          <div class="hidden items-center gap-1 md:flex">
            <component
              :is="link.external ? 'a' : 'router-link'"
              v-for="link in quickLinks"
              :key="link.key"
              v-bind="link.external ? { href: link.href, target: '_blank', rel: 'noopener noreferrer' } : { to: link.to }"
              class="header-link"
            >
              <Icon :name="link.icon" size="sm" />
              <span>{{ link.label }}</span>
            </component>
          </div>

          <div ref="overflowRef" class="relative md:hidden">
            <button
              type="button"
              class="btn-ghost btn-icon"
              :aria-label="t('common.more')"
              :aria-expanded="openMenu === 'overflow'"
              aria-haspopup="menu"
              @click="toggleMenu('overflow')"
            >
              <Icon name="more" size="md" />
            </button>
            <transition name="dropdown">
              <div v-if="openMenu === 'overflow'" class="dropdown right-0 mt-2 w-48" role="menu">
                <div class="py-1">
                  <component
                    :is="link.external ? 'a' : 'router-link'"
                    v-for="link in quickLinks"
                    :key="link.key"
                    v-bind="link.external ? { href: link.href, target: '_blank', rel: 'noopener noreferrer' } : { to: link.to }"
                    class="dropdown-item"
                    role="menuitem"
                    @click="closeMenu"
                  >
                    <Icon :name="link.icon" size="sm" />
                    {{ link.label }}
                  </component>
                </div>
              </div>
            </transition>
          </div>
        </template>

        <AnnouncementBell v-if="user" />
        <LocaleSwitcher />

        <!-- Group 2: account state. -->
        <template v-if="user">
          <div class="hidden h-5 w-px bg-line sm:block"></div>
          <SubscriptionProgressMini />

          <!--
            Balance was a hover-only popover (`group-hover:block`) on a plain
            div: unreachable by keyboard and, on touch, unreachable at all. It
            is a button with a real popover now.
          -->
          <div ref="balanceRef" class="relative hidden sm:block">
            <button
              type="button"
              class="flex items-center gap-2 rounded border border-line px-2.5 py-1 transition-colors duration-fast ease-out hover:bg-surface-hover"
              :aria-label="t('common.accountSummary')"
              :aria-expanded="openMenu === 'balance'"
              aria-haspopup="dialog"
              @click="toggleMenu('balance')"
            >
              <Icon name="wallet" size="sm" class="text-ink-tertiary" />
              <span class="font-mono text-sm tabular-nums text-ink">{{ formatHeaderMoney(availableBalance) }}</span>
              <StatusDot v-if="frozenBalance > 0" tone="warn" :label="balanceFrozenLabel" />
            </button>

            <transition name="dropdown">
              <div
                v-if="openMenu === 'balance'"
                class="dropdown right-0 mt-2 w-64 p-3"
                role="dialog"
                :aria-label="t('common.accountSummary')"
              >
                <dl class="space-y-2 text-xs">
                  <div class="flex items-center justify-between gap-4">
                    <dt class="text-ink-tertiary">{{ balanceAvailableText }}</dt>
                    <dd class="font-mono tabular-nums text-ink">{{ formatHeaderMoney(availableBalance) }}</dd>
                  </div>
                  <div class="flex items-center justify-between gap-4">
                    <dt class="text-ink-tertiary">{{ balanceFrozenText }}</dt>
                    <dd class="font-mono tabular-nums" :class="frozenBalance > 0 ? 'text-warn' : 'text-ink'">
                      {{ formatHeaderMoney(frozenBalance) }}
                    </dd>
                  </div>
                  <div class="flex items-center justify-between gap-4 border-t border-line pt-2">
                    <dt class="font-medium text-ink-secondary">{{ balanceTotalText }}</dt>
                    <dd class="font-mono font-medium tabular-nums text-ink">{{ formatHeaderMoney(totalBalance) }}</dd>
                  </div>
                </dl>
              </div>
            </transition>
          </div>
        </template>

        <!-- Group 3: identity. -->
        <div v-if="user" ref="userRef" class="relative">
          <button
            type="button"
            class="flex items-center gap-2 rounded p-1.5 transition-colors duration-fast ease-out hover:bg-surface-hover"
            :aria-label="t('common.userMenu')"
            :aria-expanded="openMenu === 'user'"
            aria-haspopup="menu"
            @click="toggleMenu('user')"
          >
            <span class="flex h-8 w-8 items-center justify-center overflow-hidden rounded bg-accent-tint text-2xs font-medium uppercase text-accent">
              <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="h-full w-full object-cover" />
              <template v-else>{{ userInitials }}</template>
            </span>
            <span class="hidden text-left md:block">
              <span class="block text-sm font-medium text-ink">{{ displayName }}</span>
              <span class="block text-2xs uppercase tracking-wide text-ink-tertiary">{{ user.role }}</span>
            </span>
            <Icon name="chevronDown" size="sm" class="hidden text-ink-tertiary md:block" />
          </button>

          <transition name="dropdown">
            <div v-if="openMenu === 'user'" class="dropdown right-0 mt-2 w-56" role="menu">
              <div class="border-b border-line px-4 py-3">
                <div class="text-sm font-medium text-ink">{{ displayName }}</div>
                <div class="truncate text-xs text-ink-tertiary">{{ user.email }}</div>
              </div>

              <!-- Balance has its own control from `sm` up. -->
              <div class="border-b border-line px-4 py-2 sm:hidden">
                <div class="text-2xs uppercase tracking-wide text-ink-tertiary">{{ t('common.balance') }}</div>
                <div class="font-mono text-sm tabular-nums text-ink">{{ formatHeaderMoney(availableBalance) }}</div>
                <div v-if="frozenBalance > 0" class="mt-0.5 font-mono text-xs tabular-nums text-warn">
                  {{ balanceFrozenText }} {{ formatHeaderMoney(frozenBalance) }}
                </div>
              </div>

              <div class="py-1">
                <router-link to="/profile" class="dropdown-item" role="menuitem" @click="closeMenu">
                  <Icon name="user" size="sm" />
                  {{ t('nav.profile') }}
                </router-link>

                <router-link to="/keys" class="dropdown-item" role="menuitem" @click="closeMenu">
                  <Icon name="key" size="sm" />
                  {{ t('nav.apiKeys') }}
                </router-link>

                <a
                  v-if="authStore.isAdmin"
                  href="https://github.com/Wei-Shaw/sub2api"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="dropdown-item"
                  role="menuitem"
                  @click="closeMenu"
                >
                  <Icon name="externalLink" size="sm" />
                  {{ t('nav.github') }}
                </a>
              </div>

              <div v-if="contactInfo" class="border-t border-line px-4 py-2.5">
                <div class="text-2xs uppercase tracking-wide text-ink-tertiary">{{ t('common.contactSupport') }}</div>
                <div class="mt-0.5 text-xs font-medium text-ink-secondary">{{ contactInfo }}</div>
              </div>

              <div v-if="showOnboardingButton" class="border-t border-line py-1">
                <button type="button" class="dropdown-item w-full" role="menuitem" @click="handleReplayGuide">
                  <Icon name="questionCircle" size="sm" />
                  {{ t('onboarding.restartTour') }}
                </button>
              </div>

              <div class="border-t border-line py-1">
                <button type="button" class="dropdown-item w-full text-danger" role="menuitem" @click="handleLogout">
                  <Icon name="login" size="sm" />
                  {{ t('nav.logout') }}
                </button>
              </div>
            </div>
          </transition>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import SubscriptionProgressMini from '@/components/common/SubscriptionProgressMini.vue'
import AnnouncementBell from '@/components/common/AnnouncementBell.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { resolveRouteLabel } from '@/router/title'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const adminSettingsStore = useAdminSettingsStore()
const onboardingStore = useOnboardingStore()

const user = computed(() => authStore.user)
const contactInfo = computed(() => appStore.contactInfo)
const docUrl = computed(() => sanitizeUrl(appStore.docUrl))
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const avatarUrl = computed(() => user.value?.avatar_url?.trim() || '')
const availableBalance = computed(() => Number(user.value?.balance || 0))
const frozenBalance = computed(() => Number(user.value?.frozen_balance || 0))
const totalBalance = computed(() => availableBalance.value + frozenBalance.value)

// These three keys post-date some deployed locale bundles; the fallback keeps
// the popover readable rather than printing a raw key path.
const balanceAvailableText = computed(() => (t('common.availableBalance') === 'common.availableBalance' ? '可用余额' : t('common.availableBalance')))
const balanceFrozenText = computed(() => (t('common.frozenBalance') === 'common.frozenBalance' ? '冻结金额' : t('common.frozenBalance')))
const balanceTotalText = computed(() => (t('common.totalBalance') === 'common.totalBalance' ? '总余额' : t('common.totalBalance')))
const balanceFrozenLabel = computed(() => `${balanceFrozenText.value} ${formatHeaderMoney(frozenBalance.value)}`)

const showOnboardingButton = computed(() => !authStore.isSimpleMode && user.value?.role === 'admin')

const userInitials = computed(() => {
  if (!user.value) return ''
  if (user.value.username) return user.value.username.substring(0, 2).toUpperCase()
  if (user.value.email) return user.value.email.split('@')[0].substring(0, 2).toUpperCase()
  return ''
})

const displayName = computed(() => {
  if (!user.value) return ''
  return user.value.username || user.value.email?.split('@')[0] || ''
})

/** Docs and Model Plaza: inline from `md` up, in the overflow menu below it. */
const quickLinks = computed(() => {
  const links: Array<{
    key: string
    label: string
    icon: 'book' | 'grid'
    external: boolean
    href?: string
    to?: { path: string; query: Record<string, string> }
  }> = []

  if (docUrl.value) {
    links.push({ key: 'docs', label: t('nav.docs'), icon: 'book', external: true, href: docUrl.value })
  } else {
    // No external documentation site configured, so the entry points at the
    // built-in public pages rather than vanishing from the header.
    links.push({
      key: 'docs',
      label: t('nav.docs'),
      icon: 'book',
      external: false,
      to: { path: '/docs', query: {} },
    })
  }
  if (user.value && modelPlazaEnabled.value) {
    links.push({
      key: 'plaza',
      label: t('nav.modelPlaza'),
      icon: 'grid',
      external: false,
      to: { path: '/model-plaza', query: { embedded: '1' } },
    })
  }
  return links
})

// One source of truth with the tab title, including the custom-page case.
const customMenuItems = computed(() => [
  ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
  ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
])

const pageTitle = computed(() => resolveRouteLabel(route, customMenuItems.value))

const pageDescription = computed(() => {
  const descKey = route.meta.descriptionKey as string
  if (descKey) return t(descKey)
  return (route.meta.description as string) || ''
})

/* ---------------------------------------------------------------------------
 * Menus
 *
 * One open menu at a time, one outside-click listener, one Escape handler,
 * rather than each widget bringing its own.
 * ------------------------------------------------------------------------- */

type MenuName = 'overflow' | 'balance' | 'user'

const openMenu = ref<MenuName | null>(null)
const overflowRef = ref<HTMLElement | null>(null)
const balanceRef = ref<HTMLElement | null>(null)
const userRef = ref<HTMLElement | null>(null)

function toggleMenu(name: MenuName) {
  openMenu.value = openMenu.value === name ? null : name
}

function closeMenu() {
  openMenu.value = null
}

function containerFor(name: MenuName): HTMLElement | null {
  return { overflow: overflowRef, balance: balanceRef, user: userRef }[name].value
}

function handleClickOutside(event: MouseEvent) {
  const open = openMenu.value
  if (!open) return
  const container = containerFor(open)
  if (container && !container.contains(event.target as Node)) closeMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !openMenu.value) return
  const container = containerFor(openMenu.value)
  closeMenu()
  // Escape must not strand focus on a node that just disappeared.
  container?.querySelector('button')?.focus()
}

function toggleMobileSidebar() {
  appStore.toggleMobileSidebar()
}

async function handleLogout() {
  closeMenu()
  try {
    await authStore.logout()
  } catch (error) {
    // Ignore logout errors — still redirect to login.
    console.error('Logout error:', error)
  }
  await router.push('/login')
}

function handleReplayGuide() {
  closeMenu()
  onboardingStore.replay()
}

function formatHeaderMoney(value: number) {
  if (!Number.isFinite(value)) return '$0.00'
  return `$${value.toFixed(2)}`
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.header-link {
  @apply flex items-center gap-1.5 rounded px-2 py-1.5 text-sm font-medium text-ink-secondary;
  @apply transition-colors duration-fast ease-out hover:bg-surface-hover hover:text-ink;
}

.dropdown-enter-active,
.dropdown-leave-active {
  transition:
    opacity var(--ds-dur-fast) var(--ds-ease-out),
    transform var(--ds-dur-fast) var(--ds-ease-out);
}

/* Was `scale(0.95) translateY(-4px)`. Menus do not zoom. */
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
