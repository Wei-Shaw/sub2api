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

  <!--
    Compact Home Page — the minimal variant an operator turns on when the
    landing page is not the point. One column, left aligned, three hairlines.
  -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-canvas text-ink"
  >
    <header class="border-b border-line">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-6 py-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt=""
            class="h-7 w-7 shrink-0 rounded object-contain"
          />
          <span class="min-w-0 truncate text-sm font-semibold [overflow-wrap:anywhere]">{{
            siteName
          }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-1">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            :class="ICON_BUTTON"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <button
            type="button"
            :class="ICON_BUTTON"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
          <Button :to="entryPath" tone="accent" variant="solid" size="md" class="ml-2">
            {{ entryLabel }}
          </Button>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center px-6 py-16">
      <div class="mx-auto w-full min-w-0 max-w-5xl">
        <img :src="siteLogo || '/logo.svg'" alt="" class="h-12 w-12 rounded object-contain" />
        <h1 class="mt-8 max-w-3xl text-3xl font-semibold [overflow-wrap:anywhere]">
          {{ siteName }}
        </h1>
        <p
          class="mt-4 max-w-2xl whitespace-pre-wrap text-md text-ink-secondary [overflow-wrap:anywhere]"
        >
          {{ siteSubtitle }}
        </p>
        <div class="mt-8">
          <Button :to="entryPath" tone="accent" variant="solid" size="md">
            {{ entryCtaLabel }}
            <template #trailing>
              <Icon name="arrowRight" size="xs" :stroke-width="2" />
            </template>
          </Button>
        </div>
      </div>
    </main>

    <footer class="border-t border-line px-6 py-5">
      <p class="mx-auto min-w-0 max-w-5xl text-xs text-ink-tertiary [overflow-wrap:anywhere]">
        &copy; {{ currentYear }} {{ siteName }}
      </p>
    </footer>
  </div>

  <!--
    Default Home Page — editorial. Type and hairlines carry the hierarchy: no
    orbs, no glass, no gradient icon tiles, no card that lifts on hover. The
    copy keys for the pain-point, comparison and CTA sections already existed
    in the locale files and were never rendered; they are the content this
    layout is built around.
  -->
  <div v-else data-testid="default-home" class="flex min-h-screen flex-col bg-canvas text-ink">
    <header class="sticky top-0 z-20 border-b border-line bg-canvas">
      <nav class="mx-auto flex max-w-5xl items-center justify-between gap-4 px-6 py-3">
        <div class="flex min-w-0 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt=""
            class="h-7 w-7 shrink-0 rounded object-contain"
          />
          <span class="min-w-0 truncate text-sm font-semibold [overflow-wrap:anywhere]">{{
            siteName
          }}</span>
        </div>

        <div class="flex shrink-0 items-center gap-1">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            :class="ICON_BUTTON"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
          </a>
          <button
            type="button"
            :class="ICON_BUTTON"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
          <Button :to="entryPath" tone="accent" variant="solid" size="md" class="ml-2">
            <template v-if="userInitial" #icon>
              <span
                class="flex h-4 w-4 items-center justify-center border border-accent-on/40 font-mono text-2xs"
              >
                {{ userInitial }}
              </span>
            </template>
            {{ entryLabel }}
          </Button>
        </div>
      </nav>
    </header>

    <main class="flex-1">
      <!-- Hero -->
      <section class="border-b border-line">
        <div
          class="mx-auto grid max-w-5xl gap-12 px-6 py-16 lg:grid-cols-[minmax(0,1fr)_minmax(0,25rem)] lg:gap-16 lg:py-24"
        >
          <div class="min-w-0">
            <p class="font-mono text-2xs uppercase tracking-[0.12em] text-ink-tertiary">
              {{ t('home.heroSubtitle') }}
            </p>
            <h1 class="mt-5 text-3xl font-semibold [overflow-wrap:anywhere]">{{ siteName }}</h1>
            <p
              class="mt-4 max-w-xl whitespace-pre-wrap text-md text-ink-secondary [overflow-wrap:anywhere]"
            >
              {{ siteSubtitle }}
            </p>
            <p class="mt-3 max-w-xl text-sm text-ink-tertiary">
              {{ t('home.heroDescription') }}
            </p>

            <div class="mt-8 flex flex-wrap items-center gap-3">
              <Button :to="entryPath" tone="accent" variant="solid" size="md">
                {{ entryCtaLabel }}
                <template #trailing>
                  <Icon name="arrowRight" size="xs" :stroke-width="2" />
                </template>
              </Button>
              <Button
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                variant="outline"
                size="md"
              >
                {{ t('home.docs') }}
                <template #trailing>
                  <Icon name="externalLink" size="xs" :stroke-width="2" />
                </template>
              </Button>
            </div>
          </div>

          <!-- A request/response sample, flat. Was a rotated, glowing terminal. -->
          <div class="min-w-0 border border-line bg-surface">
            <div
              class="flex items-center justify-between border-b border-line bg-surface-sunken px-3 py-2"
            >
              <span class="font-mono text-2xs uppercase tracking-[0.08em] text-ink-tertiary">
                POST /v1/messages
              </span>
              <span class="font-mono text-2xs text-success">200</span>
            </div>
            <div class="space-y-2 overflow-x-auto p-4 font-mono text-xs leading-5">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-ink-tertiary">$</span>
                <span class="text-ink">curl -X POST</span>
                <span class="text-accent">/v1/messages</span>
              </div>
              <div class="text-ink-tertiary">&#35; routing to upstream</div>
              <div class="flex flex-wrap items-center gap-2">
                <span class="border border-success/40 bg-success-tint px-1.5 text-success">
                  200 OK
                </span>
                <span class="text-ink-secondary">{ "content": "Hello!" }</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-ink-tertiary">$</span>
                <span class="ds-caret inline-block h-3.5 w-1.5 bg-accent" aria-hidden="true"></span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Capability strip -->
      <section class="border-b border-line">
        <ul
          class="mx-auto grid max-w-5xl divide-y divide-line-subtle px-6 sm:grid-cols-3 sm:divide-x sm:divide-y-0"
        >
          <li
            v-for="(tag, index) in tags"
            :key="tag.key"
            class="flex items-center gap-2.5 py-4"
            :class="index === 0 ? 'sm:pr-5' : index === tags.length - 1 ? 'sm:pl-5' : 'sm:px-5'"
          >
            <Icon :name="tag.icon" size="sm" class="shrink-0 text-ink-tertiary" />
            <span class="text-sm font-medium">{{ t(`home.tags.${tag.key}`) }}</span>
          </li>
        </ul>
      </section>

      <!-- Pain points -->
      <section class="border-b border-line">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <h2 class="text-xl font-semibold">{{ t('home.painPoints.title') }}</h2>
          <ul class="mt-8 grid gap-px bg-line-subtle sm:grid-cols-2 lg:grid-cols-4">
            <li v-for="(item, index) in painPoints" :key="item" class="bg-canvas p-5 sm:p-6">
              <span class="font-mono text-2xs tabular-nums text-ink-tertiary">
                {{ String(index + 1).padStart(2, '0') }}
              </span>
              <h3 class="mt-3 text-sm font-medium">
                {{ t(`home.painPoints.items.${item}.title`) }}
              </h3>
              <p class="mt-2 text-xs leading-5 text-ink-tertiary">
                {{ t(`home.painPoints.items.${item}.desc`) }}
              </p>
            </li>
          </ul>
        </div>
      </section>

      <!-- Solutions -->
      <section class="border-b border-line">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <h2 class="text-xl font-semibold">{{ t('home.solutions.title') }}</h2>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('home.solutions.subtitle') }}</p>
          <ul class="mt-8 grid gap-px bg-line-subtle md:grid-cols-3">
            <li v-for="(item, index) in solutions" :key="item.key" class="bg-canvas p-6">
              <div class="flex items-center justify-between">
                <Icon :name="item.icon" size="md" class="text-ink-secondary" />
                <span class="font-mono text-2xs tabular-nums text-ink-tertiary">
                  {{ String(index + 1).padStart(2, '0') }}
                </span>
              </div>
              <h3 class="mt-5 text-base font-medium">{{ t(`home.features.${item.key}`) }}</h3>
              <p class="mt-2 text-sm leading-6 text-ink-tertiary">
                {{ t(`home.features.${item.key}Desc`) }}
              </p>
            </li>
          </ul>
        </div>
      </section>

      <!-- Comparison — a real table, hairlines and one heavy rule. -->
      <section class="border-b border-line">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <h2 class="text-xl font-semibold">{{ t('home.comparison.title') }}</h2>
          <div class="mt-8 overflow-x-auto">
            <table class="w-full min-w-[38rem] border-collapse text-left">
              <thead>
                <tr class="border-b border-line-strong">
                  <th
                    scope="col"
                    class="w-1/4 py-2 pr-4 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
                  >
                    {{ t('home.comparison.headers.feature') }}
                  </th>
                  <th
                    scope="col"
                    class="py-2 pr-4 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
                  >
                    {{ t('home.comparison.headers.official') }}
                  </th>
                  <th
                    scope="col"
                    class="py-2 text-2xs font-medium uppercase tracking-[0.04em] text-ink"
                  >
                    {{ t('home.comparison.headers.us') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in comparisonRows"
                  :key="row"
                  class="border-b border-line-subtle align-top"
                >
                  <th scope="row" class="py-3 pr-4 text-sm font-medium">
                    {{ t(`home.comparison.items.${row}.feature`) }}
                  </th>
                  <td class="py-3 pr-4 text-sm text-ink-tertiary">
                    {{ t(`home.comparison.items.${row}.official`) }}
                  </td>
                  <td class="py-3 text-sm text-ink">
                    {{ t(`home.comparison.items.${row}.us`) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <!-- Providers -->
      <section class="border-b border-line">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <h2 class="text-xl font-semibold">{{ t('home.providers.title') }}</h2>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('home.providers.description') }}</p>
          <ul class="mt-8 grid gap-px bg-line-subtle sm:grid-cols-2 lg:grid-cols-3">
            <li
              v-for="provider in providers"
              :key="provider.label"
              class="flex items-center gap-3 bg-canvas p-4"
            >
              <span
                class="flex h-7 w-7 shrink-0 items-center justify-center border border-line font-mono text-xs text-ink-secondary"
                aria-hidden="true"
              >
                {{ provider.mark }}
              </span>
              <span class="min-w-0 flex-1 truncate text-sm font-medium">{{ provider.label }}</span>
              <Badge :tone="provider.soon ? 'neutral' : 'success'" caps>
                {{ provider.soon ? t('home.providers.soon') : t('home.providers.supported') }}
              </Badge>
            </li>
          </ul>
        </div>
      </section>

      <!-- Closing CTA -->
      <section class="border-b border-line bg-surface-sunken">
        <div
          class="mx-auto flex max-w-5xl flex-col gap-6 px-6 py-14 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="min-w-0">
            <h2 class="text-xl font-semibold">{{ t('home.cta.title') }}</h2>
            <p class="mt-2 max-w-xl text-sm text-ink-secondary">{{ t('home.cta.description') }}</p>
          </div>
          <Button :to="signupPath" tone="accent" variant="solid" size="md" class="shrink-0">
            {{ signupLabel }}
            <template #trailing>
              <Icon name="arrowRight" size="xs" :stroke-width="2" />
            </template>
          </Button>
        </div>
      </section>
    </main>

    <footer class="px-6 py-8">
      <div
        class="mx-auto flex max-w-5xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <p class="min-w-0 text-xs text-ink-tertiary [overflow-wrap:anywhere]">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-5">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-ink-tertiary underline-offset-2 transition-colors duration-fast hover:text-ink hover:underline"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            :title="t('home.viewOnGithub')"
            class="text-xs text-ink-tertiary underline-offset-2 transition-colors duration-fast hover:text-ink hover:underline"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import { Badge, Button } from '@/components/common'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useTheme } from '@/composables/useTheme'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

/** Icon-only control. No border until hover — the header is already a rule. */
const ICON_BUTTON =
  'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink'

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform'
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(
  () => appStore.cachedPublicSettings?.compact_home_enabled === true
)

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme — the shared reactive owner. This view used to keep a private `isDark`
// ref and write localStorage itself, so toggling here left the sidebar stale.
const { isDark, toggleTheme } = useTheme()

// GitHub URL
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

/** Header destination, plus the two label registers the CTAs use. */
const entryPath = computed(() => (isAuthenticated.value ? dashboardPath.value : '/login'))
const entryLabel = computed(() => (isAuthenticated.value ? t('home.dashboard') : t('home.login')))
const entryCtaLabel = computed(() =>
  isAuthenticated.value ? t('home.goToDashboard') : t('home.getStarted')
)

/**
 * The closing CTA reads "sign up free", so it must only point at `/register`
 * when the instance actually accepts registrations — otherwise it walks the
 * visitor into a route that turns them away.
 */
const registrationEnabled = computed(
  () => appStore.cachedPublicSettings?.registration_enabled === true
)
const signupPath = computed(() => {
  if (isAuthenticated.value) return dashboardPath.value
  return registrationEnabled.value ? '/register' : '/login'
})
const signupLabel = computed(() => {
  if (isAuthenticated.value) return t('home.goToDashboard')
  return registrationEnabled.value ? t('home.cta.button') : t('home.login')
})

const tags = [
  { key: 'subscriptionToApi', icon: 'swap' },
  { key: 'stickySession', icon: 'shield' },
  { key: 'realtimeBilling', icon: 'chart' },
] as const

const painPoints = ['expensive', 'complex', 'unstable', 'noControl'] as const

const solutions = [
  { key: 'unifiedGateway', icon: 'server' },
  { key: 'multiAccount', icon: 'sync' },
  { key: 'balanceQuota', icon: 'chart' },
] as const

const comparisonRows = ['pricing', 'models', 'management', 'stability', 'control'] as const

const providers = computed(() => [
  { mark: 'C', label: t('home.providers.claude'), soon: false },
  { mark: 'G', label: 'GPT', soon: false },
  { mark: 'G', label: t('home.providers.gemini'), soon: false },
  { mark: 'A', label: t('home.providers.antigravity'), soon: false },
  { mark: '+', label: t('home.providers.more'), soon: true },
])

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  // Theme is applied before mount in main.ts, so there is nothing to init here.

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
/*
 * The only animation on this page. A frozen caret reads as a broken page, so
 * under reduced-motion it stops at full opacity rather than slowing down.
 */
.ds-caret {
  animation: ds-caret-blink 1s step-end infinite;
}

@keyframes ds-caret-blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ds-caret {
    animation: none;
    opacity: 1;
  }
}
</style>
