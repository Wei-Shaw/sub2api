<template>
  <div data-testid="minimal-home" class="minimal home-style-page flex min-h-screen min-w-0 flex-col overflow-x-hidden bg-[#f4f0e8] text-[#24211d] dark:bg-[#191816] dark:text-[#eee8dc]">
    <HomeChrome
      :context="context"
      header-class="px-4 py-5 sm:px-8"
      nav-class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3"
      action-class="flex h-10 w-10 items-center justify-center rounded-full text-stone-500 hover:bg-black/5 dark:hover:bg-white/10"
      cta-class="inline-flex min-h-10 items-center rounded-full border border-current px-4 text-xs font-semibold"
    />

    <main class="mx-auto grid w-full max-w-5xl flex-1 px-4 py-10 sm:px-8 sm:py-20 lg:grid-cols-[1fr_2fr] lg:gap-16">
      <aside class="border-t border-stone-400/50 pt-4 text-xs uppercase tracking-[.2em] text-stone-500">
        <p>{{ t('home.styles.minimal.eyebrow') }}</p>
        <p class="mt-2">{{ t('home.styles.minimal.established', { year: context.currentYear }) }}</p>
      </aside>

      <article class="mt-12 min-w-0 lg:mt-0">
        <p class="font-serif text-xl italic text-stone-500">{{ t('home.styles.minimal.kicker') }}</p>
        <h1 class="mt-5 [overflow-wrap:anywhere] font-serif text-5xl leading-[1.02] sm:text-7xl">{{ context.siteName }}</h1>
        <p class="mt-8 max-w-2xl whitespace-pre-wrap font-serif text-xl leading-9 text-stone-600 dark:text-stone-300 sm:text-2xl">{{ context.siteSubtitle }}</p>
        <div class="mt-12 flex flex-wrap items-center gap-6">
          <router-link :to="destination" class="border-b border-current pb-1 text-sm font-bold">
            {{ context.isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
            <span aria-hidden="true">&rarr;</span>
          </router-link>
          <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer" class="text-sm text-stone-500">{{ t('home.docs') }}</a>
        </div>

        <ol class="mt-20 grid gap-8 border-t border-stone-400/50 pt-8 sm:grid-cols-3">
          <li v-for="(item, index) in notes" :key="item.title">
            <span class="text-xs text-stone-400" aria-hidden="true">0{{ index + 1 }}</span>
            <h2 class="mt-3 font-serif text-lg">{{ t(item.title) }}</h2>
            <p class="mt-2 text-sm leading-6 text-stone-500 dark:text-stone-400">{{ t(item.body) }}</p>
          </li>
        </ol>
      </article>
    </main>

    <footer class="mx-auto w-full max-w-5xl px-4 py-7 text-xs text-stone-500 sm:px-8">&copy; {{ context.currentYear }} {{ context.siteName }}</footer>
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

const notes = [
  { title: 'home.styles.minimal.notes.endpoint.title', body: 'home.styles.minimal.notes.endpoint.body' },
  { title: 'home.styles.minimal.notes.choice.title', body: 'home.styles.minimal.notes.choice.body' },
  { title: 'home.styles.minimal.notes.usage.title', body: 'home.styles.minimal.notes.usage.body' },
] as const
</script>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  .minimal *,
  .minimal *::before,
  .minimal *::after {
    animation: none !important;
    transition: none !important;
  }
}
</style>
