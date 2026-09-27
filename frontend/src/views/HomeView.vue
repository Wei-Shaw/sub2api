<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    data-testid="default-home"
    class="relative grid min-h-screen overflow-hidden bg-[#f7f7f5] text-gray-950 dark:bg-[#090909] dark:text-white"
  >
    <div
      class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_center,rgba(255,255,255,0.96)_0,rgba(247,247,245,0.84)_34%,rgba(229,231,229,0.72)_100%)] dark:bg-[radial-gradient(circle_at_center,rgba(38,38,38,0.82)_0,rgba(9,9,9,0.94)_55%,rgba(0,0,0,1)_100%)]"
    ></div>

    <div class="relative z-10 grid min-h-screen grid-rows-[auto_1fr_auto] px-5 py-6 sm:px-8 lg:px-12">
      <!-- Header -->
      <header class="relative mx-auto h-9 w-full max-w-6xl">
        <div
          class="absolute left-0 top-0 hidden max-w-[48vw] items-center rounded-lg border border-gray-200 bg-white/70 px-3 py-2 text-xs font-medium text-gray-700 shadow-sm backdrop-blur sm:inline-flex dark:border-dark-700 dark:bg-dark-900/70 dark:text-dark-300"
        >
          <span>{{ localText("开始使用", "Start with") }}</span>
          <span class="brand-serif ml-1 truncate text-gray-950 dark:text-white">{{ siteName }}</span>
        </div>

        <router-link to="/home" class="absolute left-0 top-0 flex h-9 min-w-[112px] items-center text-xl font-bold text-black dark:text-white sm:hidden">
          <BrandWordmark :name="siteName" />
        </router-link>

        <div class="absolute right-0 top-0 flex items-center gap-2">
          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Model Plaza Link -->
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-black px-3 text-xs font-semibold text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-black dark:hover:bg-gray-200"
          >
            <span>{{ t('home.dashboard') }}</span>
            <Icon name="arrowRight" size="xs" :stroke-width="2" />
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex h-9 items-center rounded-lg bg-black px-3 text-xs font-semibold text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-black dark:hover:bg-gray-200"
          >
            {{ t('home.login') }}
          </router-link>

          <LocaleSwitcher />

          <button
            @click="toggleTheme"
            class="hidden h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-black/5 hover:text-gray-950 dark:text-dark-300 dark:hover:bg-white/10 dark:hover:text-white sm:inline-flex"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
        </div>
      </header>

      <!-- Brand Center -->
      <main class="mx-auto flex w-full max-w-6xl items-center justify-center py-14 text-center">
        <section class="flex flex-col items-center">
          <h1
            class="brand-serif text-[44px] font-semibold leading-none tracking-normal text-black sm:text-6xl lg:text-7xl dark:text-white"
          >
            {{ siteName }}
          </h1>
          <p class="mt-4 max-w-xl text-sm leading-6 text-gray-500 sm:text-base dark:text-dark-300">
            {{ siteSubtitle }}
          </p>
        </section>
      </main>

      <!-- Entry Links -->
      <nav class="mx-auto grid w-full max-w-4xl grid-cols-1 gap-4 pb-8 sm:grid-cols-2 lg:grid-cols-4">
        <router-link
          v-for="item in homeLinks"
          :key="item.title"
          :to="item.to"
          class="group min-h-[112px] rounded-lg px-1 py-2 transition-colors hover:bg-black/[0.03] dark:hover:bg-white/[0.06] sm:px-3"
        >
          <div class="flex items-center gap-2 text-[19px] font-semibold leading-6 text-black dark:text-white">
            <span>{{ item.title }}</span>
            <Icon
              name="arrowRight"
              size="sm"
              :stroke-width="2"
              class="transition-transform group-hover:translate-x-0.5"
            />
          </div>
          <p class="mt-3 text-sm leading-5 text-gray-500 dark:text-dark-400">
            {{ item.desc }}
          </p>
        </router-link>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import BrandWordmark from '@/components/common/BrandWordmark.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { resolveSiteName } from '@/constants/site'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t, locale } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => resolveSiteName(appStore.cachedPublicSettings?.site_name || appStore.siteName))
const siteLogo = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '',
  { allowRelative: true, allowDataUrl: true },
))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

function localText(zh: string, en: string): string {
  return locale.value.startsWith('zh') ? zh : en
}

const homeLinks = computed(() => {
  const loginPath = '/login'

  return [
    {
      title: localText('控制台', 'Dashboard'),
      desc: localText('查看账户状态、用量和关键配置。', 'View account status, usage, and key settings.'),
      to: isAuthenticated.value ? dashboardPath.value : loginPath,
    },
    {
      title: localText('API 密钥', 'API Keys'),
      desc: localText('创建、管理并复制你的服务密钥。', 'Create, manage, and copy your service keys.'),
      to: isAuthenticated.value ? '/keys' : loginPath,
    },
    {
      title: localText('充值订阅', 'Subscribe'),
      desc: localText('选择套餐或为账户余额充值。', 'Choose a plan or recharge your account balance.'),
      to: isAuthenticated.value ? '/purchase' : loginPath,
    },
    {
      title: localText('使用统计', 'Usage'),
      desc: localText('追踪请求、Token 消耗和调用成本。', 'Track requests, token usage, and call costs.'),
      to: isAuthenticated.value ? '/usage' : loginPath,
    },
  ]
})

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
