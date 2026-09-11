<template>
  <div class="product-shell min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <aside
      class="fixed inset-y-0 left-0 z-50 flex w-64 -translate-x-full flex-col border-r border-gray-200 bg-white transition-transform duration-200 dark:border-dark-800 dark:bg-dark-900 lg:translate-x-0"
      :class="{ 'translate-x-0': mobileOpen }"
    >
      <div class="flex h-20 items-center gap-3 border-b border-gray-100 px-6 dark:border-dark-800">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3" @click="closeMobile">
          <span class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700">
            <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-semibold text-gray-950 dark:text-white">{{ siteName }}</span>
        </RouterLink>
        <button
          type="button"
          class="btn-ghost btn-icon ml-auto lg:hidden"
          :aria-label="t('common.close')"
          @click="closeMobile"
        >
          <Icon name="x" size="md" />
        </button>
      </div>

      <nav class="flex-1 overflow-y-auto px-3 py-6" :aria-label="t('product.nav.label')">
        <p class="px-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-400 dark:text-dark-500">
          {{ t('product.nav.section') }}
        </p>
        <div class="mt-3 space-y-1">
          <RouterLink
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="product-nav-link"
            :class="{ 'product-nav-link-active': route.path === item.to }"
            @click="closeMobile"
          >
            <Icon :name="item.icon" size="md" />
            <span>{{ t(item.label) }}</span>
          </RouterLink>
        </div>

        <div class="mt-8 border-t border-gray-100 pt-6 dark:border-dark-800">
          <p class="px-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-400 dark:text-dark-500">
            {{ t('product.nav.accountSection') }}
          </p>
          <RouterLink
            :to="dashboardPath"
            class="product-nav-link mt-3"
            @click="closeMobile"
          >
            <Icon name="home" size="md" />
            <span>{{ t('product.nav.console') }}</span>
          </RouterLink>
          <RouterLink
            v-if="isAuthenticated"
            to="/keys"
            class="product-nav-link"
            @click="closeMobile"
          >
            <Icon name="key" size="md" />
            <span>{{ t('product.nav.apiKeys') }}</span>
          </RouterLink>
        </div>
      </nav>

      <div class="border-t border-gray-100 p-4 dark:border-dark-800">
        <div v-if="isAuthenticated" class="mb-3 rounded-xl bg-primary-50 px-3 py-2.5 dark:bg-primary-900/20">
          <div class="flex items-center justify-between gap-2 text-xs text-primary-700 dark:text-primary-300">
            <span>{{ t('product.balance') }}</span>
            <Icon name="creditCard" size="sm" />
          </div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-primary-900 dark:text-primary-100">
            {{ formatBalance(userBalance) }}
          </div>
        </div>
        <RouterLink
          v-if="isAuthenticated"
          to="/profile"
          class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-gray-600 transition hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          @click="closeMobile"
        >
          <Icon name="user" size="sm" />
          <span class="truncate">{{ displayName }}</span>
        </RouterLink>
        <RouterLink
          v-else
          :to="{ path: '/login', query: { redirect: route.fullPath } }"
          class="btn btn-primary w-full"
          @click="closeMobile"
        >
          <Icon name="login" size="sm" />
          {{ t('product.login') }}
        </RouterLink>
      </div>
    </aside>

    <div v-if="mobileOpen" class="fixed inset-0 z-40 bg-black/40 lg:hidden" @click="closeMobile"></div>

    <div class="min-h-screen lg:pl-64">
      <header class="sticky top-0 z-30 border-b border-gray-200/80 bg-gray-50/90 backdrop-blur-xl dark:border-dark-800 dark:bg-dark-950/90">
        <div class="flex h-16 items-center justify-between gap-3 px-4 sm:px-6 lg:px-8">
          <div class="flex min-w-0 items-center gap-3">
            <button
              type="button"
              class="btn-ghost btn-icon lg:hidden"
              :aria-label="t('common.toggleMenu')"
              @click="toggleMobile"
            >
              <Icon name="menu" size="md" />
            </button>
            <div class="min-w-0">
              <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ currentLabel }}</p>
              <p class="hidden truncate text-xs text-gray-500 dark:text-dark-400 sm:block">{{ t('product.headerHint') }}</p>
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-2 sm:gap-3">
            <LocaleSwitcher />
            <RouterLink
              v-if="isAuthenticated"
              to="/profile"
              class="hidden items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-gray-600 transition hover:bg-white hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white sm:flex"
            >
              <span class="flex h-7 w-7 items-center justify-center rounded-lg bg-primary-100 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-200">{{ userInitial }}</span>
              <span class="max-w-[120px] truncate">{{ displayName }}</span>
            </RouterLink>
            <RouterLink
              v-else
              :to="{ path: '/login', query: { redirect: route.fullPath } }"
              class="btn btn-primary btn-sm"
            >
              <Icon name="login" size="sm" />
              {{ t('product.login') }}
            </RouterLink>
          </div>
        </div>
      </header>

      <main class="mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { sanitizeUrl } from '@/utils/url'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount } from '@/components/payment/currency'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const mobileOpen = computed(() => appStore.mobileOpen)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '风合智联')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const userBalance = computed(() => authStore.user?.balance ?? 0)
const displayName = computed(() => authStore.user?.username || authStore.user?.email?.split('@')[0] || t('product.account'))
const userInitial = computed(() => displayName.value.charAt(0).toUpperCase())

const navItems = [
  { to: '/experience', icon: 'beaker' as const, label: 'product.nav.experience' },
  { to: '/model-plaza', icon: 'grid' as const, label: 'product.nav.modelPlaza' },
  { to: '/pricing', icon: 'dollar' as const, label: 'product.nav.pricing' },
]

const currentLabel = computed(() => {
  const item = navItems.find((entry) => entry.to === route.path)
  return item ? t(item.label) : t('product.nav.section')
})

function toggleMobile() {
  appStore.setMobileOpen(!mobileOpen.value)
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

function formatBalance(value: number) {
  return formatPaymentAmount(value, DEFAULT_PAYMENT_CURRENCY)
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.product-nav-link {
  @apply flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white;
}

.product-nav-link-active {
  @apply bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200;
}
</style>
