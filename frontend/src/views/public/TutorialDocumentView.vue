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

      <article
        v-else
        class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900"
      >
        <div class="border-b border-gray-200 px-6 py-5 dark:border-dark-700">
          <p class="text-sm font-medium text-primary-700 dark:text-primary-300">{{ siteName }}</p>
          <h1 class="mt-2 text-3xl font-bold text-gray-950 dark:text-white">教程文档</h1>
        </div>
        <div class="public-markdown-content p-6 md:p-10" v-html="renderedHtml"></div>
      </article>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { getPublicSettings } from '@/api/auth'
import type { PublicSettings } from '@/types'
import { updateRouteSEO } from '@/utils/seo'

interface TutorialDocumentPayload {
  content_html: string
}

const route = useRoute()
const { t } = useI18n()

const loading = ref(true)
const settings = ref<PublicSettings | null>(null)
const renderedHtml = ref('')

const siteName = ref('Sub2API')
const siteLogo = ref('')

function normalizeHTML(raw: string) {
  return DOMPurify.sanitize(raw, {
    ADD_ATTR: ['style', 'target', 'rel', 'class'],
  })
}

onMounted(async () => {
  loading.value = true
  try {
    const [publicSettingsResp, tutorialResp] = await Promise.all([
      getPublicSettings(),
      fetch('/api/v1/tutorial-document', {
        headers: { Accept: 'application/json' },
      }),
    ])

    settings.value = publicSettingsResp
    siteName.value = publicSettingsResp.site_name || 'Sub2API'
    siteLogo.value = publicSettingsResp.site_logo || ''

    if (tutorialResp.ok) {
      const payload = await tutorialResp.json() as { code?: number; data?: TutorialDocumentPayload }
      renderedHtml.value = normalizeHTML(payload?.data?.content_html || '')
    } else {
      renderedHtml.value = '<p class="text-red-500">页面未找到。</p>'
    }

    updateRouteSEO(route, publicSettingsResp)
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

.public-markdown-content :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-700;
}

.public-markdown-content :deep(pre) {
  @apply my-4 overflow-x-auto rounded-xl bg-gray-900 p-4 text-gray-100;
}

.public-markdown-content :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}
</style>
