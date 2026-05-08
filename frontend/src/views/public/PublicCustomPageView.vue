<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/95 dark:border-dark-800 dark:bg-dark-900/95">
      <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3">
          <span class="flex h-10 w-10 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-semibold text-gray-950 dark:text-white">
            {{ siteName }}
          </span>
        </RouterLink>
        <RouterLink
          to="/login"
          class="inline-flex flex-shrink-0 items-center justify-center rounded-lg bg-primary-600 px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-primary-600/20 transition hover:bg-primary-700"
        >
          {{ t('common.login') }}
        </RouterLink>
      </div>
    </header>

    <main class="mx-auto max-w-6xl px-4 py-8 sm:px-6 lg:py-10">
      <div v-if="loading" class="flex min-h-[320px] items-center justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <section
        v-else-if="!menuItem"
        class="rounded-lg border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900"
      >
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customPage.notFoundTitle') }}</h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">{{ t('customPage.notFoundDesc') }}</p>
      </section>

      <article v-else-if="isMarkdownMode" class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="border-b border-gray-200 px-6 py-5 dark:border-dark-700">
          <p class="text-sm font-medium text-primary-700 dark:text-primary-300">{{ siteName }}</p>
          <h1 class="mt-2 text-3xl font-bold text-gray-950 dark:text-white">{{ menuItem.label }}</h1>
        </div>
        <div
          ref="markdownContainer"
          class="public-markdown-content p-6 md:p-10"
          v-html="renderedHtml"
        ></div>
      </article>

      <section
        v-else-if="!isValidUrl"
        class="rounded-lg border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900"
      >
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customPage.notConfiguredTitle') }}</h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">{{ t('customPage.notConfiguredDesc') }}</p>
      </section>

      <section v-else class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between gap-4 border-b border-gray-200 px-6 py-4 dark:border-dark-700">
          <div>
            <h1 class="text-2xl font-bold text-gray-950 dark:text-white">{{ menuItem.label }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ siteSubtitle }}</p>
          </div>
          <a
            :href="embeddedUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm"
          >
            {{ t('customPage.openInNewTab') }}
          </a>
        </div>
        <iframe :src="embeddedUrl" class="h-[75vh] w-full border-0" allowfullscreen></iframe>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { getPublicSettings } from '@/api/auth'
import { buildEmbeddedUrl } from '@/utils/embedded-url'
import { updateRouteSEO } from '@/utils/seo'
import type { CustomMenuItem, PublicSettings } from '@/types'

const route = useRoute()
const { t, locale } = useI18n()

const loading = ref(true)
const settings = ref<PublicSettings | null>(null)
const renderedHtml = ref('')
const markdownContainer = ref<HTMLElement | null>(null)

const menuItemId = computed(() => String(route.params.id || ''))
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() => settings.value?.site_logo || '')
const siteSubtitle = computed(() => settings.value?.site_subtitle || '')
const menuItem = computed<CustomMenuItem | null>(() => {
  const items = settings.value?.custom_menu_items ?? []
  return items.find((item) => item.id === menuItemId.value && item.visibility !== 'admin') ?? null
})

const markdownSlug = computed(() => {
  const item = menuItem.value
  if (!item) return ''
  if (item.page_slug) return item.page_slug
  if (item.url?.startsWith('md:')) return item.url.slice(3)
  return ''
})

const isMarkdownMode = computed(() => !!markdownSlug.value)
const embeddedUrl = computed(() => {
  if (!menuItem.value || isMarkdownMode.value) return ''
  return buildEmbeddedUrl(menuItem.value.url, undefined, undefined, 'light', locale.value)
})
const isValidUrl = computed(() => embeddedUrl.value.startsWith('http://') || embeddedUrl.value.startsWith('https://'))

async function fetchAndRenderMarkdown(slug: string) {
  const resp = await fetch(`/api/v1/public/pages/${encodeURIComponent(slug)}`)
  if (!resp.ok) {
    renderedHtml.value = '<p class="text-red-500">Page not found</p>'
    return
  }
  const raw = await resp.text()
  const html = marked.parse(raw) as string
  renderedHtml.value = DOMPurify.sanitize(html)
  await nextTick()
}

watch(markdownSlug, async (slug) => {
  if (!slug) {
    renderedHtml.value = ''
    return
  }
  await fetchAndRenderMarkdown(slug)
}, { immediate: true })

onMounted(async () => {
  loading.value = true
  try {
    settings.value = await getPublicSettings()
    updateRouteSEO(route, settings.value)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.public-markdown-content {
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.public-markdown-content :deep(h1) {
  @apply mb-4 mt-8 border-b border-gray-200 pb-3 text-3xl font-bold dark:border-dark-700;
}

.public-markdown-content :deep(h2) {
  @apply mb-3 mt-7 text-2xl font-bold;
}

.public-markdown-content :deep(h3) {
  @apply mb-2 mt-6 text-xl font-semibold;
}

.public-markdown-content :deep(p) {
  @apply mb-4 text-gray-700 dark:text-dark-200;
}

.public-markdown-content :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200;
}

.public-markdown-content :deep(ul) {
  @apply mb-4 list-disc pl-6;
}

.public-markdown-content :deep(ol) {
  @apply mb-4 list-decimal pl-6;
}

.public-markdown-content :deep(img) {
  @apply my-5 h-auto max-w-full rounded-lg;
}
</style>
