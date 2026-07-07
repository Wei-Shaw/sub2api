<template>
  <header class="relative z-20 border-b border-gray-100 bg-white/80 backdrop-blur-xl dark:border-dark-800 dark:bg-dark-950/80">
    <nav class="mx-auto flex h-16 max-w-7xl items-center px-4 sm:px-6">
      <!-- Left: Logo -->
      <router-link to="/home" class="flex shrink-0 items-center gap-2.5">
        <div class="h-8 w-8 overflow-hidden rounded-lg">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <span class="text-lg font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
      </router-link>

      <!-- Center: Nav Links with Icons -->
      <div class="flex flex-1 items-center justify-center gap-1">
        <router-link to="/home"
          :class="[
            'hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors sm:flex',
            activePath === '/home'
              ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
          ]">
          <Icon name="home" size="xs" />
          {{ t('home.nav.home') }}
        </router-link>
        <template v-for="link in navLinks" :key="link.path">
          <a v-if="link.external" :href="link.href" target="_blank" rel="noopener noreferrer"
            class="hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white sm:flex">
            <Icon :name="link.icon" size="xs" />
            {{ link.label }}
          </a>
          <router-link v-else :to="link.path"
            :class="[
              'hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors sm:flex',
              activePath === link.path
                ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
                : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
            ]">
            <Icon :name="link.icon" size="xs" />
            {{ link.label }}
          </router-link>
        </template>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
          class="hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white sm:flex">
          <Icon name="book" size="xs" />
          {{ t('home.docs') }}
        </a>
      </div>

      <!-- Right: Actions -->
      <div class="flex shrink-0 items-center gap-2">
        <LocaleSwitcher />
        <button @click="toggleTheme"
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-50 dark:text-dark-400 dark:hover:bg-dark-800">
          <Icon v-if="isDark" name="sun" size="sm" />
          <Icon v-else name="moon" size="sm" />
        </button>
        <router-link v-if="isAuthenticated" :to="dashboardPath"
          class="hidden ml-1 inline-flex items-center gap-2 rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100 sm:inline-flex">
          {{ t('home.dashboard') }}
          <Icon name="arrowRight" size="xs" :stroke-width="2" />
        </router-link>
        <template v-else>
          <router-link to="/login"
            class="hidden items-center rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition-all hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 sm:inline-flex">
            {{ t('home.login') }}
          </router-link>
          <router-link to="/register"
            class="hidden items-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100 sm:inline-flex">
            {{ t('home.getStarted') }}
          </router-link>
        </template>
        <button @click="mobileMenuOpen = !mobileMenuOpen"
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-50 dark:text-dark-400 dark:hover:bg-dark-800 sm:hidden">
          <Icon v-if="mobileMenuOpen" name="x" size="sm" />
          <Icon v-else name="menu" size="sm" />
        </button>
      </div>
    </nav>
    <div v-if="mobileMenuOpen" class="border-t border-gray-100 bg-white px-4 pb-4 pt-2 dark:border-dark-800 dark:bg-dark-950 sm:hidden">
      <div class="flex flex-col gap-1">
        <router-link to="/home" @click="mobileMenuOpen = false"
          :class="[
            'flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm transition-colors',
            activePath === '/home'
              ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
          ]">
          <Icon name="home" size="xs" />
          {{ t('home.nav.home') }}
        </router-link>
        <template v-for="link in navLinks" :key="link.path">
          <a v-if="link.external" :href="link.href" target="_blank" rel="noopener noreferrer" @click="mobileMenuOpen = false"
            class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white">
            <Icon :name="link.icon" size="xs" />
            {{ link.label }}
          </a>
          <router-link v-else :to="link.path"
            @click="mobileMenuOpen = false"
            :class="[
              'flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm transition-colors',
              activePath === link.path
                ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
                : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
            ]">
            <Icon :name="link.icon" size="xs" />
            {{ link.label }}
          </router-link>
        </template>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
          class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white">
          <Icon name="book" size="xs" />
          {{ t('home.docs') }}
        </a>
        <div class="my-1 border-t border-gray-100 dark:border-dark-800"></div>
        <router-link v-if="isAuthenticated" :to="dashboardPath" @click="mobileMenuOpen = false"
          class="flex items-center justify-center gap-2 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100">
          {{ t('home.dashboard') }}
          <Icon name="arrowRight" size="xs" :stroke-width="2" />
        </router-link>
        <template v-else>
          <router-link to="/login" @click="mobileMenuOpen = false"
            class="flex items-center justify-center rounded-lg border border-gray-300 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 transition-all hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700">
            {{ t('home.login') }}
          </router-link>
          <router-link to="/register" @click="mobileMenuOpen = false"
            class="flex items-center justify-center rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100">
            {{ t('home.getStarted') }}
          </router-link>
        </template>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  activePath?: string
}>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')

const isDark = ref(document.documentElement.classList.contains('dark'))
const mobileMenuOpen = ref(false)

const navLinks = computed(() => [
  { path: '/key-usage', label: t('keyUsage.title'), icon: 'key' as const },
  { path: '/monitoring', label: t('admin.monitoring.title'), icon: 'chart' as const, external: true, href: 'https://status.djoui.online' },
  { path: '/pricing', label: t('pricing.title'), icon: 'dollar' as const },
])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
  authStore.checkAuth()
  appStore.fetchPublicSettings()
})
</script>
