<template>
  <div
    data-testid="editorial-home"
    class="editorial home-style-page min-h-screen min-w-0 overflow-x-hidden bg-[#f7f8fa] text-[#171922] dark:bg-[#20232c] dark:text-[#f2f4f7]"
  >
    <HomeChrome
      :context="context"
      header-class="border-b border-black/10 px-4 py-4 dark:border-white/10 sm:px-7"
      nav-class="mx-auto flex max-w-[1160px] flex-wrap items-center justify-between gap-3"
      cta-class="inline-flex min-h-10 items-center rounded-md bg-[#5b48f2] px-4 text-sm font-bold text-white"
    />

    <main>
      <section class="mx-auto grid max-w-[1160px] gap-12 px-4 py-14 sm:px-7 lg:grid-cols-[.88fr_1.12fr] lg:items-center lg:py-20">
        <div class="min-w-0">
          <p class="mb-5 flex items-center gap-2 font-mono text-[11px] font-bold tracking-[.16em] text-[#5b48f2]">
            <span class="h-px w-5 bg-current" aria-hidden="true"></span>
            {{ t('home.styles.editorial.eyebrow') }}
          </p>
          <h1 class="[overflow-wrap:anywhere] font-serif text-5xl font-medium leading-[.98] sm:text-6xl lg:text-7xl">
            {{ t('home.styles.editorial.titleLineOne') }}<br />
            <em class="not-italic text-[#5b48f2]">{{ t('home.styles.editorial.titleLineTwo') }}</em>
          </h1>
          <p class="mt-6 max-w-lg whitespace-pre-wrap leading-7 text-gray-600 dark:text-gray-300">
            {{ context.siteSubtitle }}
          </p>
          <div class="mt-7 flex flex-wrap items-center gap-5">
            <router-link
              :to="destination"
              class="inline-flex min-h-11 items-center rounded-md bg-[#5b48f2] px-5 text-sm font-bold text-white"
            >
              {{ context.isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <span class="ml-2" aria-hidden="true">&rarr;</span>
            </router-link>
            <a
              v-if="context.docUrl"
              :href="context.docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="border-b border-current py-2 text-xs font-bold"
            >
              {{ t('home.styles.editorial.viewExample') }}
            </a>
          </div>
          <ul class="mt-7 flex flex-wrap gap-x-5 gap-y-2 text-[11px] text-gray-500 dark:text-gray-400">
            <li v-for="trust in editorialTrust" :key="trust">{{ t(trust) }}</li>
          </ul>
        </div>

        <div
          class="min-w-0 overflow-hidden rounded-lg border border-[#313642] bg-[#151821] font-mono text-xs text-gray-200 shadow-[18px_20px_0_rgba(91,72,242,.1)]"
          role="group"
          :aria-label="t('home.styles.editorial.terminalLabel')"
        >
          <div class="flex min-h-11 items-center justify-between border-b border-[#313642] px-4 text-[10px] text-gray-500">
            <span class="flex gap-1.5" aria-hidden="true">
              <i class="h-2 w-2 rounded-full bg-red-400"></i>
              <i class="h-2 w-2 rounded-full bg-amber-300"></i>
              <i class="h-2 w-2 rounded-full bg-green-400"></i>
            </span>
            <span>{{ context.siteName }} / {{ t('home.styles.editorial.request') }}</span>
          </div>
          <div class="space-y-5 p-5 sm:p-7">
            <p class="break-words">
              <b class="text-lime-300">$</b> {{ t('home.styles.editorial.command') }}
            </p>
            <p class="text-gray-500"># {{ t('home.styles.editorial.routeComment') }}</p>
            <div class="grid grid-cols-[auto_1fr_auto_1fr_auto] items-center gap-2 text-[9px] sm:text-[10px]">
              <span>{{ t('home.styles.editorial.yourApp') }}</span>
              <i class="h-px bg-gray-700" aria-hidden="true"></i>
              <span class="border border-lime-600 px-2 py-1 text-lime-300">{{ t('home.styles.editorial.gateway') }}</span>
              <i class="h-px bg-gray-700" aria-hidden="true"></i>
              <span>{{ t('home.styles.editorial.model') }}</span>
            </div>
            <p class="rounded border border-gray-700 bg-black/20 p-3">
              <b class="text-green-400">{{ t('home.styles.editorial.responseCode') }}</b>
              <span class="ml-2 text-amber-300">{{ t('home.styles.editorial.responseBody') }}</span>
            </p>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
              <div v-for="detail in terminalDetails" :key="detail.label" class="min-w-0 rounded border border-gray-700 p-3">
                <small class="block text-[8px] uppercase tracking-wider text-gray-500">{{ t(detail.label) }}</small>
                <strong class="mt-1 block truncate text-[10px] text-gray-200">{{ t(detail.value) }}</strong>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="border-y border-black/10 bg-white/60 dark:border-white/10 dark:bg-white/5" :aria-label="t('home.styles.editorial.capabilitiesLabel')">
        <div class="mx-auto grid max-w-[1160px] md:grid-cols-3">
          <article
            v-for="(item, index) in capabilities"
            :key="item.title"
            class="grid grid-cols-[2.25rem_1fr] gap-3 border-b border-black/10 p-6 last:border-0 dark:border-white/10 md:border-b-0 md:border-r md:last:border-r-0"
          >
            <span :class="item.color" class="flex h-9 w-9 items-center justify-center rounded-md font-bold text-white" aria-hidden="true">
              {{ index + 1 }}
            </span>
            <div>
              <h2 class="font-bold">{{ t(item.title) }}</h2>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(item.body) }}</p>
            </div>
          </article>
        </div>
      </section>

      <section class="mx-auto max-w-[1160px] px-4 py-12 sm:px-7">
        <div class="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
          <div>
            <p class="font-mono text-[10px] font-bold tracking-[.14em] text-[#5b48f2]">{{ t('home.styles.editorial.directoryEyebrow') }}</p>
            <h2 class="mt-2 text-2xl font-bold">{{ t('home.styles.editorial.directoryTitle') }}</h2>
          </div>
          <p class="max-w-md text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t('home.styles.editorial.directoryDescription') }}</p>
        </div>
        <ul class="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <li v-for="platform in platforms" :key="platform.name" class="rounded-lg border border-black/10 bg-white/70 p-4 dark:border-white/10 dark:bg-white/5">
            <span :class="platform.color" class="flex h-9 w-9 items-center justify-center rounded-md text-xs font-black text-white" aria-hidden="true">
              {{ t(platform.name).charAt(0) }}
            </span>
            <strong class="mt-3 block">{{ t(platform.name) }}</strong>
            <small class="mt-1 block text-gray-500 dark:text-gray-400">{{ t(platform.description) }}</small>
          </li>
        </ul>
      </section>
    </main>

    <footer class="mx-auto flex w-full max-w-[1160px] flex-wrap justify-between gap-3 border-t border-black/10 px-4 py-7 text-xs text-gray-500 dark:border-white/10 sm:px-7">
      <span>&copy; {{ context.currentYear }} {{ context.siteName }}</span>
      <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HomeChrome from './HomeChrome.vue'
import type { HomeStyleContext } from './types'

const props = defineProps<{ context: HomeStyleContext }>()
const { t } = useI18n()
const destination = computed(() => props.context.isAuthenticated ? props.context.dashboardPath : '/login')

const editorialTrust = [
  'home.styles.editorial.trust.unifiedAuth',
  'home.styles.editorial.trust.sdkCompatible',
  'home.styles.editorial.trust.usageBilling',
] as const

const terminalDetails = [
  { label: 'home.styles.editorial.details.model', value: 'home.styles.editorial.details.modelValue' },
  { label: 'home.styles.editorial.details.route', value: 'home.styles.editorial.details.routeValue' },
  { label: 'home.styles.editorial.details.interface', value: 'home.styles.editorial.details.interfaceValue' },
] as const

const capabilities = [
  { title: 'home.styles.editorial.capabilities.sdk.title', body: 'home.styles.editorial.capabilities.sdk.body', color: 'bg-[#624cf1]' },
  { title: 'home.styles.editorial.capabilities.routing.title', body: 'home.styles.editorial.capabilities.routing.body', color: 'bg-[#1ca982]' },
  { title: 'home.styles.editorial.capabilities.billing.title', body: 'home.styles.editorial.capabilities.billing.body', color: 'bg-[#ef6e5d]' },
] as const

const platforms = [
  { name: 'home.styles.platforms.claude.name', description: 'home.styles.editorial.platformDescriptions.claude', color: 'bg-orange-500' },
  { name: 'home.styles.platforms.gpt.name', description: 'home.styles.editorial.platformDescriptions.gpt', color: 'bg-emerald-600' },
  { name: 'home.styles.platforms.gemini.name', description: 'home.styles.editorial.platformDescriptions.gemini', color: 'bg-blue-600' },
  { name: 'home.styles.platforms.antigravity.name', description: 'home.styles.editorial.platformDescriptions.antigravity', color: 'bg-fuchsia-600' },
] as const
</script>

<style scoped>
.editorial {
  background-image:
    linear-gradient(rgb(67 75 94 / 4.5%) 1px, transparent 1px),
    linear-gradient(90deg, rgb(67 75 94 / 4.5%) 1px, transparent 1px);
  background-size: 48px 48px;
}

@media (prefers-reduced-motion: reduce) {
  .editorial *,
  .editorial *::before,
  .editorial *::after {
    animation: none !important;
    scroll-behavior: auto !important;
    transition: none !important;
  }
}
</style>
