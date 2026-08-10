<template>
  <div data-testid="catalog-home" class="catalog home-style-page min-h-screen min-w-0 overflow-x-hidden bg-slate-50 text-slate-950 dark:bg-slate-950 dark:text-white">
    <HomeChrome
      :context="context"
      header-class="border-b border-slate-200 bg-white px-4 py-4 dark:border-slate-800 dark:bg-slate-950 sm:px-6"
      nav-class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-3"
    />

    <main class="mx-auto max-w-7xl px-4 py-10 sm:px-6 sm:py-14">
      <div class="grid gap-8 lg:grid-cols-[280px_1fr]">
        <aside>
          <p class="text-xs font-bold uppercase tracking-[.2em] text-primary-600">{{ t('home.styles.catalog.eyebrow') }}</p>
          <h1 class="mt-3 [overflow-wrap:anywhere] text-3xl font-bold">{{ t('home.styles.catalog.title') }}</h1>
          <p class="mt-4 whitespace-pre-wrap text-sm leading-6 text-slate-600 dark:text-slate-400">{{ context.siteSubtitle }}</p>
          <router-link :to="destination" class="mt-7 inline-flex min-h-11 w-full items-center justify-center rounded-lg bg-primary-600 px-4 text-sm font-semibold text-white">
            {{ context.isAuthenticated ? t('home.dashboard') : t('home.getStarted') }}
          </router-link>
          <a
            v-if="context.docUrl"
            :href="context.docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-3 inline-flex min-h-11 w-full items-center justify-center rounded-lg border border-slate-300 px-4 text-sm font-semibold dark:border-slate-700"
          >
            {{ t('home.docs') }}
          </a>
          <p class="mt-5 text-xs leading-5 text-slate-500">{{ t('home.styles.catalog.staticNote') }}</p>
        </aside>

        <section class="min-w-0" :aria-labelledby="'catalog-families'">
          <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
            <h2 id="catalog-families" class="text-lg font-semibold">{{ t('home.styles.catalog.familiesTitle') }}</h2>
            <span class="rounded-full bg-slate-200 px-3 py-1 text-xs font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-300">
              {{ t('home.styles.catalog.providerCount') }}
            </span>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <article v-for="model in models" :key="model.name" class="min-w-0 rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
              <div class="flex items-start justify-between gap-3">
                <span :class="model.color" class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-sm font-black text-white" aria-hidden="true">{{ t(model.name).charAt(0) }}</span>
                <span class="rounded-full bg-slate-100 px-2 py-1 text-[10px] font-bold uppercase tracking-wider text-slate-500 dark:bg-slate-800">
                  {{ t('home.styles.catalog.listed') }}
                </span>
              </div>
              <h3 class="mt-5 text-xl font-bold">{{ t(model.name) }}</h3>
              <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-400">{{ t(model.description) }}</p>
              <ul class="mt-5 flex flex-wrap gap-2" :aria-label="t('home.styles.catalog.capabilityTags')">
                <li v-for="tag in model.tags" :key="tag" class="rounded-md bg-slate-100 px-2 py-1 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-300">{{ t(tag) }}</li>
              </ul>
            </article>
          </div>
        </section>
      </div>
    </main>

    <footer class="border-t border-slate-200 px-4 py-6 text-center text-xs text-slate-500 dark:border-slate-800 sm:px-6">&copy; {{ context.currentYear }} {{ context.siteName }}</footer>
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

const models = [
  {
    name: 'home.styles.platforms.claude.name',
    color: 'bg-orange-500',
    description: 'home.styles.catalog.models.claude.description',
    tags: ['home.styles.catalog.tags.reasoning', 'home.styles.catalog.tags.code', 'home.styles.catalog.tags.longContext'],
  },
  {
    name: 'home.styles.platforms.gpt.name',
    color: 'bg-emerald-600',
    description: 'home.styles.catalog.models.gpt.description',
    tags: ['home.styles.catalog.tags.general', 'home.styles.catalog.tags.tools', 'home.styles.catalog.tags.vision'],
  },
  {
    name: 'home.styles.platforms.gemini.name',
    color: 'bg-blue-600',
    description: 'home.styles.catalog.models.gemini.description',
    tags: ['home.styles.catalog.tags.multimodal', 'home.styles.catalog.tags.fast', 'home.styles.catalog.tags.context'],
  },
  {
    name: 'home.styles.platforms.antigravity.name',
    color: 'bg-fuchsia-600',
    description: 'home.styles.catalog.models.antigravity.description',
    tags: ['home.styles.catalog.tags.routing', 'home.styles.catalog.tags.flexible', 'home.styles.catalog.tags.api'],
  },
] as const
</script>

<style scoped>
.catalog article {
  transition: border-color 150ms ease, transform 150ms ease, box-shadow 150ms ease;
}

.catalog article:hover {
  transform: translateY(-2px);
}

@media (prefers-reduced-motion: reduce) {
  .catalog article {
    transform: none !important;
    transition: none !important;
  }
}
</style>
