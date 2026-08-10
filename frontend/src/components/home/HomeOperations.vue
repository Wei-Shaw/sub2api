<template>
  <div data-testid="operations-home" class="operations home-style-page min-h-screen min-w-0 overflow-x-hidden bg-[#070b10] font-mono text-slate-200">
    <HomeChrome
      :context="context"
      header-class="border-b border-cyan-400/20 bg-[#0a1017] px-4 py-3 sm:px-6"
      nav-class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-3"
      action-class="flex h-10 w-10 items-center justify-center border border-slate-700 bg-slate-900 text-slate-400 hover:border-cyan-400/60 hover:text-cyan-300"
      cta-class="inline-flex min-h-10 items-center border border-cyan-400/50 bg-cyan-400/10 px-4 text-xs font-bold uppercase tracking-wider text-cyan-300"
    />

    <main class="mx-auto max-w-7xl px-4 py-8 sm:px-6">
      <div class="mb-6 flex flex-wrap items-end justify-between gap-5">
        <div>
          <p class="text-xs uppercase tracking-[.24em] text-cyan-400">{{ t('home.styles.operations.eyebrow') }}</p>
          <h1 class="mt-2 [overflow-wrap:anywhere] text-3xl font-semibold text-white sm:text-5xl">{{ context.siteName }}</h1>
          <p class="mt-3 max-w-2xl whitespace-pre-wrap text-sm leading-6 text-slate-400">{{ context.siteSubtitle }}</p>
        </div>
        <p class="flex max-w-sm items-center gap-2 border border-amber-300/30 bg-amber-300/5 px-3 py-2 text-[10px] uppercase tracking-wider text-amber-200">
          <span class="h-2 w-2 shrink-0 border border-amber-200" aria-hidden="true"></span>
          {{ t('home.styles.operations.demoNotice') }}
        </p>
      </div>

      <section :aria-labelledby="'operations-capabilities'">
        <h2 id="operations-capabilities" class="sr-only">{{ t('home.styles.operations.capabilitiesLabel') }}</h2>
        <dl class="grid gap-px overflow-hidden border border-slate-800 bg-slate-800 sm:grid-cols-2 lg:grid-cols-4">
          <div v-for="metric in metrics" :key="metric.label" class="bg-[#0d141d] p-5">
            <dt class="text-[10px] uppercase tracking-widest text-slate-500">{{ t(metric.label) }}</dt>
            <dd class="mt-3 text-2xl text-white">{{ t(metric.value) }}</dd>
            <dd class="mt-2 text-xs text-cyan-300">{{ t(metric.note) }}</dd>
          </div>
        </dl>
      </section>

      <section class="mt-6 grid gap-6 lg:grid-cols-[1.5fr_1fr]" :aria-label="t('home.styles.operations.workspaceLabel')">
        <div class="min-w-0 border border-slate-800 bg-[#0d141d]">
          <header class="flex flex-wrap justify-between gap-2 border-b border-slate-800 px-5 py-3 text-[10px] uppercase tracking-widest text-slate-500">
            <h2>{{ t('home.styles.operations.matrixTitle') }}</h2>
            <span>{{ t('home.styles.operations.matrixCaption') }}</span>
          </header>
          <div class="divide-y divide-slate-800">
            <div v-for="route in routes" :key="route.name" class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-5 py-4 text-xs sm:grid-cols-[minmax(0,1fr)_auto_auto]">
              <span class="truncate text-slate-200">{{ t(route.name) }}</span>
              <span class="hidden text-slate-500 sm:inline">{{ t(route.mode) }}</span>
              <b class="text-cyan-300">{{ t('home.styles.operations.routable') }}</b>
            </div>
          </div>
        </div>

        <div class="border border-slate-800 bg-[#0d141d] p-5">
          <h2 class="text-[10px] uppercase tracking-widest text-slate-500">{{ t('home.styles.operations.quickAccess') }}</h2>
          <router-link :to="destination" class="mt-5 flex min-h-12 items-center justify-between border border-cyan-400/40 bg-cyan-400/10 px-4 text-sm text-cyan-300">
            <span>{{ context.isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
            <span aria-hidden="true">&rarr;</span>
          </router-link>
          <a
            v-if="context.docUrl"
            :href="context.docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-3 flex min-h-12 items-center justify-between border border-slate-700 px-4 text-sm text-slate-300"
          >
            <span>{{ t('home.docs') }}</span>
            <span aria-hidden="true">&nearr;</span>
          </a>
          <p class="mt-5 border-l-2 border-amber-300/50 pl-3 text-[10px] leading-5 text-slate-500">{{ t('home.styles.operations.disclaimer') }}</p>
        </div>
      </section>
    </main>

    <footer class="mx-auto max-w-7xl border-t border-slate-800 px-4 py-5 text-[10px] uppercase tracking-widest text-slate-600 sm:px-6">
      &copy; {{ context.currentYear }} {{ context.siteName }} / {{ t('home.styles.operations.footerLabel') }}
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

const metrics = [
  { label: 'home.styles.operations.metrics.routing.label', value: 'home.styles.operations.metrics.routing.value', note: 'home.styles.operations.metrics.routing.note' },
  { label: 'home.styles.operations.metrics.providers.label', value: 'home.styles.operations.metrics.providers.value', note: 'home.styles.operations.metrics.providers.note' },
  { label: 'home.styles.operations.metrics.protocol.label', value: 'home.styles.operations.metrics.protocol.value', note: 'home.styles.operations.metrics.protocol.note' },
  { label: 'home.styles.operations.metrics.api.label', value: 'home.styles.operations.metrics.api.value', note: 'home.styles.operations.metrics.api.note' },
] as const

const routes = [
  { name: 'home.styles.operations.routes.claude.name', mode: 'home.styles.operations.routes.claude.mode' },
  { name: 'home.styles.operations.routes.gpt.name', mode: 'home.styles.operations.routes.gpt.mode' },
  { name: 'home.styles.operations.routes.gemini.name', mode: 'home.styles.operations.routes.gemini.mode' },
  { name: 'home.styles.operations.routes.antigravity.name', mode: 'home.styles.operations.routes.antigravity.mode' },
] as const
</script>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  .operations *,
  .operations *::before,
  .operations *::after {
    animation: none !important;
    transition: none !important;
  }
}
</style>
