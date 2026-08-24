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
    class="relative flex min-h-screen flex-col overflow-hidden bg-[#fafbfc] dark:bg-dark-950"
  >
    <!-- Background -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -left-32 top-0 h-[480px] w-[480px] rounded-full bg-primary-400/10 blur-3xl dark:bg-primary-500/5"
      ></div>
      <div
        class="absolute -right-32 bottom-0 h-[400px] w-[400px] rounded-full bg-blue-400/10 blur-3xl dark:bg-blue-500/5"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.025)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.025)_1px,transparent_1px)] bg-[size:48px_48px] dark:opacity-30"
      ></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 border-b border-gray-200/60 bg-white/70 px-6 py-3 backdrop-blur-md dark:border-dark-800/60 dark:bg-dark-950/70">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="h-9 w-9 overflow-hidden rounded-lg shadow-sm ring-1 ring-gray-200/80 dark:ring-dark-700">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ siteName }}</span>
        </div>

        <div class="flex items-center gap-2">
          <LocaleSwitcher />

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

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
            <Icon name="arrowRight" size="xs" class="text-gray-400" />
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-900 px-3.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10 flex-1">
      <!-- Hero -->
      <section class="px-6 pb-16 pt-16 md:pb-20 md:pt-24">
        <div class="mx-auto max-w-4xl text-center">
          <div
            class="mb-6 inline-flex items-center gap-2 rounded-full border border-primary-200/60 bg-primary-50/80 px-4 py-1.5 text-sm font-medium text-primary-700 dark:border-primary-800/60 dark:bg-primary-950/40 dark:text-primary-300"
          >
            <Icon name="sparkles" size="sm" />
            {{ t('home.heroSubtitle') }}
          </div>

          <h1 class="mb-5 text-4xl font-bold tracking-tight text-gray-900 dark:text-white md:text-5xl lg:text-6xl">
            {{ siteName }}
          </h1>

          <p class="mx-auto mb-4 max-w-2xl text-lg text-gray-600 dark:text-dark-300 md:text-xl">
            {{ siteSubtitle }}
          </p>
          <p class="mx-auto mb-10 max-w-2xl text-base text-gray-500 dark:text-dark-400">
            {{ t('home.heroDescription') }}
          </p>

          <div class="flex flex-col items-center justify-center gap-3 sm:flex-row">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="btn btn-primary inline-flex items-center px-8 py-3 text-base shadow-lg shadow-primary-500/25"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
            </router-link>
            <router-link
              v-if="!isAuthenticated"
              to="/register"
              class="btn btn-secondary inline-flex items-center px-8 py-3 text-base"
            >
              {{ t('home.cta.button') }}
            </router-link>
          </div>

          <!-- Feature tags -->
          <div class="mt-12 flex flex-wrap items-center justify-center gap-3">
            <span
              v-for="tag in featureTags"
              :key="tag"
              class="inline-flex items-center gap-1.5 rounded-full border border-gray-200/80 bg-white/80 px-4 py-1.5 text-sm text-gray-600 shadow-sm dark:border-dark-700/60 dark:bg-dark-900/60 dark:text-dark-300"
            >
              <Icon name="check" size="xs" class="text-primary-500" :stroke-width="2.5" />
              {{ tag }}
            </span>
          </div>
        </div>
      </section>

      <!-- Pain Points -->
      <section class="border-y border-gray-200/60 bg-white/60 px-6 py-16 dark:border-dark-800/60 dark:bg-dark-900/30">
        <div class="mx-auto max-w-6xl">
          <h2 class="mb-10 text-center text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.painPoints.title') }}
          </h2>
          <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            <div
              v-for="item in painPoints"
              :key="item.key"
              class="rounded-2xl border border-gray-200/60 bg-white p-6 shadow-sm transition-shadow hover:shadow-md dark:border-dark-700/60 dark:bg-dark-900/80"
            >
              <div
                class="mb-4 flex h-10 w-10 items-center justify-center rounded-xl bg-red-50 text-red-500 dark:bg-red-950/40 dark:text-red-400"
              >
                <Icon :name="item.icon" size="md" />
              </div>
              <h3 class="mb-2 font-semibold text-gray-900 dark:text-white">{{ item.title }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ item.desc }}</p>
            </div>
          </div>
        </div>
      </section>

      <!-- Solutions / Features -->
      <section class="px-6 py-16 md:py-20">
        <div class="mx-auto max-w-6xl">
          <div class="mb-12 text-center">
            <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
              {{ t('home.solutions.title') }}
            </h2>
            <p class="text-gray-500 dark:text-dark-400">{{ t('home.solutions.subtitle') }}</p>
          </div>

          <div class="grid gap-6 md:grid-cols-3">
            <div
              v-for="(feature, index) in features"
              :key="feature.title"
              class="group relative overflow-hidden rounded-2xl border border-gray-200/60 bg-white p-7 shadow-sm transition-all hover:border-primary-200 hover:shadow-lg hover:shadow-primary-500/5 dark:border-dark-700/60 dark:bg-dark-900/60 dark:hover:border-primary-800"
            >
              <div
                class="mb-5 flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 text-sm font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
              >
                {{ index + 1 }}
              </div>
              <div
                class="mb-4 flex h-11 w-11 items-center justify-center rounded-xl bg-gradient-to-br text-white shadow-md"
                :class="feature.color"
              >
                <Icon :name="feature.icon" size="md" />
              </div>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ feature.title }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ feature.desc }}</p>
            </div>
          </div>
        </div>
      </section>

      <!-- Comparison -->
      <section class="border-y border-gray-200/60 bg-gray-50/80 px-6 py-16 dark:border-dark-800/60 dark:bg-dark-900/20">
        <div class="mx-auto max-w-4xl">
          <h2 class="mb-10 text-center text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.comparison.title') }}
          </h2>
          <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-sm dark:border-dark-700/60 dark:bg-dark-900/80">
            <div class="overflow-x-auto">
              <table class="w-full min-w-[480px] text-sm">
                <thead>
                  <tr class="border-b border-gray-100 bg-gray-50/80 dark:border-dark-700 dark:bg-dark-800/50">
                    <th class="px-5 py-3.5 text-left font-medium text-gray-500 dark:text-dark-400">
                      {{ t('home.comparison.headers.feature') }}
                    </th>
                    <th class="px-5 py-3.5 text-left font-medium text-gray-500 dark:text-dark-400">
                      {{ t('home.comparison.headers.official') }}
                    </th>
                    <th class="px-5 py-3.5 text-left font-medium text-primary-600 dark:text-primary-400">
                      {{ t('home.comparison.headers.us') }}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(row, idx) in comparisonRows"
                    :key="row.feature"
                    class="border-b border-gray-50 last:border-0 dark:border-dark-800"
                    :class="idx % 2 === 0 ? 'bg-white dark:bg-dark-900/40' : 'bg-gray-50/50 dark:bg-dark-900/20'"
                  >
                    <td class="px-5 py-3.5 font-medium text-gray-700 dark:text-dark-200">{{ row.feature }}</td>
                    <td class="px-5 py-3.5 text-gray-500 dark:text-dark-400">{{ row.official }}</td>
                    <td class="px-5 py-3.5 font-medium text-primary-700 dark:text-primary-300">
                      <span class="inline-flex items-center gap-1.5">
                        <Icon name="checkCircle" size="sm" class="shrink-0 text-primary-500" />
                        {{ row.us }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </section>

      <!-- Supported Providers -->
      <section class="px-6 py-16 md:py-20">
        <div class="mx-auto max-w-6xl text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.providers.title') }}
          </h2>
          <p class="mb-10 text-gray-500 dark:text-dark-400">{{ t('home.providers.description') }}</p>

          <div class="flex flex-wrap items-center justify-center gap-3">
            <div
              v-for="provider in providers"
              :key="provider.name"
              class="flex items-center gap-2.5 rounded-xl border px-5 py-3 transition-colors"
              :class="
                provider.supported
                  ? 'border-primary-200/80 bg-white shadow-sm ring-1 ring-primary-500/10 dark:border-primary-800/60 dark:bg-dark-900/60'
                  : 'border-gray-200/60 bg-gray-50/80 opacity-60 dark:border-dark-700/60 dark:bg-dark-900/30'
              "
            >
              <div
                class="flex h-8 w-8 items-center justify-center rounded-lg text-xs font-bold text-white"
                :class="provider.color"
              >
                {{ provider.letter }}
              </div>
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ provider.name }}</span>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-medium"
                :class="
                  provider.supported
                    ? 'bg-primary-100 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-400'
                "
              >
                {{ provider.supported ? t('home.providers.supported') : t('home.providers.soon') }}
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- CTA Banner -->
      <section class="px-6 pb-16">
        <div class="mx-auto max-w-4xl">
          <div
            class="relative overflow-hidden rounded-3xl bg-gradient-to-br from-primary-600 to-primary-700 px-8 py-12 text-center shadow-xl shadow-primary-500/20 md:px-16"
          >
            <div
              class="pointer-events-none absolute -right-16 -top-16 h-48 w-48 rounded-full bg-white/10 blur-2xl"
            ></div>
            <div
              class="pointer-events-none absolute -bottom-16 -left-16 h-48 w-48 rounded-full bg-white/10 blur-2xl"
            ></div>
            <h2 class="relative mb-3 text-2xl font-bold text-white md:text-3xl">
              {{ t('home.cta.title') }}
            </h2>
            <p class="relative mb-8 text-primary-100">{{ t('home.cta.description') }}</p>
            <router-link
              :to="isAuthenticated ? dashboardPath : '/register'"
              class="relative inline-flex items-center rounded-full bg-white px-8 py-3 text-sm font-semibold text-primary-700 shadow-lg transition-transform hover:scale-105 hover:bg-primary-50"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.cta.button') }}
              <Icon name="arrowRight" size="sm" class="ml-2" />
            </router-link>
          </div>
        </div>
      </section>
    </main>

    <SiteFooter />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import SiteFooter from '@/components/layout/SiteFooter.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || t('home.heroSubtitle')
)
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const currentYear = computed(() => new Date().getFullYear())
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user?.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const featureTags = computed(() => [
  t('home.tags.subscriptionToApi'),
  t('home.tags.stickySession'),
  t('home.tags.realtimeBilling')
])

const painPoints = computed(() => [
  {
    key: 'expensive',
    icon: 'chart' as const,
    title: t('home.painPoints.items.expensive.title'),
    desc: t('home.painPoints.items.expensive.desc')
  },
  {
    key: 'complex',
    icon: 'users' as const,
    title: t('home.painPoints.items.complex.title'),
    desc: t('home.painPoints.items.complex.desc')
  },
  {
    key: 'unstable',
    icon: 'exclamationTriangle' as const,
    title: t('home.painPoints.items.unstable.title'),
    desc: t('home.painPoints.items.unstable.desc')
  },
  {
    key: 'noControl',
    icon: 'lock' as const,
    title: t('home.painPoints.items.noControl.title'),
    desc: t('home.painPoints.items.noControl.desc')
  }
])

const features = computed(() => [
  {
    icon: 'server' as const,
    color: 'from-blue-500 to-blue-600 shadow-blue-500/30',
    title: t('home.features.unifiedGateway'),
    desc: t('home.features.unifiedGatewayDesc')
  },
  {
    icon: 'shield' as const,
    color: 'from-primary-500 to-primary-600 shadow-primary-500/30',
    title: t('home.features.multiAccount'),
    desc: t('home.features.multiAccountDesc')
  },
  {
    icon: 'chart' as const,
    color: 'from-purple-500 to-purple-600 shadow-purple-500/30',
    title: t('home.features.balanceQuota'),
    desc: t('home.features.balanceQuotaDesc')
  }
])

const comparisonRows = computed(() => [
  {
    feature: t('home.comparison.items.pricing.feature'),
    official: t('home.comparison.items.pricing.official'),
    us: t('home.comparison.items.pricing.us')
  },
  {
    feature: t('home.comparison.items.models.feature'),
    official: t('home.comparison.items.models.official'),
    us: t('home.comparison.items.models.us')
  },
  {
    feature: t('home.comparison.items.management.feature'),
    official: t('home.comparison.items.management.official'),
    us: t('home.comparison.items.management.us')
  },
  {
    feature: t('home.comparison.items.stability.feature'),
    official: t('home.comparison.items.stability.official'),
    us: t('home.comparison.items.stability.us')
  },
  {
    feature: t('home.comparison.items.control.feature'),
    official: t('home.comparison.items.control.official'),
    us: t('home.comparison.items.control.us')
  }
])

const providers = computed(() => [
  { name: t('home.providers.claude'), letter: 'C', color: 'bg-gradient-to-br from-orange-400 to-orange-500', supported: true },
  { name: 'GPT', letter: 'G', color: 'bg-gradient-to-br from-green-500 to-green-600', supported: true },
  { name: t('home.providers.gemini'), letter: 'G', color: 'bg-gradient-to-br from-blue-500 to-blue-600', supported: true },
  { name: t('home.providers.antigravity'), letter: 'A', color: 'bg-gradient-to-br from-rose-500 to-pink-600', supported: true },
  { name: t('home.providers.more'), letter: '+', color: 'bg-gradient-to-br from-gray-500 to-gray-600', supported: false }
])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

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
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
