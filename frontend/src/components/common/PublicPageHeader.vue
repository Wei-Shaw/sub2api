<template>
  <header class="relative z-20 border-b border-gray-100 bg-white/80 backdrop-blur-xl dark:border-dark-800 dark:bg-dark-950/80">
    <nav class="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6">
      <router-link to="/home" class="flex items-center gap-3">
        <div class="h-8 w-8 overflow-hidden rounded-lg">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <span class="text-lg font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
      </router-link>
      <div class="flex items-center gap-2">
        <router-link v-for="link in navLinks" :key="link.path" :to="link.path"
          :class="[
            'hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors sm:flex',
            activePath === link.path
              ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
          ]">
          {{ link.label }}
        </router-link>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
          class="hidden items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white sm:flex">
          {{ t('home.docs') }}
        </a>
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
        <router-link v-else to="/login"
          class="hidden ml-1 inline-flex items-center gap-2 rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100 sm:inline-flex">
          {{ t('home.login') }}
        </router-link>
        <button @click="mobileMenuOpen = !mobileMenuOpen"
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-50 dark:text-dark-400 dark:hover:bg-dark-800 sm:hidden">
          <Icon v-if="mobileMenuOpen" name="x" size="sm" />
          <Icon v-else name="menu" size="sm" />
        </button>
      </div>
    </nav>
    <div v-if="mobileMenuOpen" class="border-t border-gray-100 bg-white px-4 pb-4 pt-2 dark:border-dark-800 dark:bg-dark-950 sm:hidden">
      <div class="flex flex-col gap-1">
        <router-link v-for="link in navLinks" :key="link.path" :to="link.path"
          @click="mobileMenuOpen = false"
          :class="[
            'rounded-lg px-3 py-2.5 text-sm transition-colors',
            activePath === link.path
              ? 'font-medium text-gray-900 bg-gray-100 dark:text-white dark:bg-dark-800'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
          ]">
          {{ link.label }}
        </router-link>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
          class="rounded-lg px-3 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white">
          {{ t('home.docs') }}
        </a>
        <div class="my-1 border-t border-gray-100 dark:border-dark-800"></div>
        <router-link v-if="isAuthenticated" :to="dashboardPath" @click="mobileMenuOpen = false"
          class="inline-flex items-center justify-center gap-2 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100">
          {{ t('home.dashboard') }}
          <Icon name="arrowRight" size="xs" :stroke-width="2" />
        </router-link>
        <router-link v-else to="/login" @click="mobileMenuOpen = false"
          class="inline-flex items-center justify-center gap-2 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-all hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100">
          {{ t('home.login') }}
        </router-link>
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
  { path: '/key-usage', label: t('keyUsage.title') },
  { path: '/monitoring', label: t('admin.monitoring.title') },
  { path: '/pricing', label: t('pricing.title') },
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
