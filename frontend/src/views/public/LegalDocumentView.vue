<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/95 dark:border-dark-800 dark:bg-dark-900/95">
      <div class="mx-auto flex max-w-5xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
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
          登录
        </RouterLink>
      </div>
    </header>

    <main class="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:py-10">
      <div v-if="loading" class="flex min-h-[320px] items-center justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <section
        v-else-if="loadError"
        class="rounded-lg border border-red-200 bg-red-50 p-6 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
      >
        <h1 class="text-lg font-semibold">文档加载失败</h1>
        <p class="mt-2 text-sm">请稍后刷新页面重试。</p>
      </section>

      <section
        v-else-if="!currentDocument"
        class="rounded-lg border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900"
      >
        <div class="flex items-start gap-3">
          <span class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300">
            <Icon name="document" size="sm" />
          </span>
          <div>
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">文档不存在</h1>
            <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">
              当前条款文档不存在或已被管理员移除。
            </p>
          </div>
        </div>
      </section>

      <article v-else>
        <div class="mb-8 border-b border-gray-200 pb-6 dark:border-dark-700">
          <div class="flex items-start gap-4">
            <span class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-md bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300">
              <Icon :name="documentIcon" size="md" />
            </span>
            <div class="min-w-0">
              <p class="text-sm font-medium text-primary-700 dark:text-primary-300">登录条款</p>
              <h1 class="mt-2 break-words text-2xl font-bold tracking-normal text-gray-950 dark:text-white sm:text-3xl">
                {{ currentDocument.title }}
              </h1>
              <p v-if="updatedAt" class="mt-3 text-sm text-gray-500 dark:text-dark-400">
                更新日期：{{ updatedAt }}
              </p>
            </div>
          </div>
        </div>

        <div
          v-if="hasContent"
          class="legal-document-content"
          v-html="renderedHtml"
        ></div>
        <div
          v-else
          class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-14 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400"
        >
          暂无正文内容
        </div>
      </article>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { getPublicSettings } from '@/api/auth'
import { sanitizeUrl } from '@/utils/url'
import type { LoginAgreementDocument, PublicSettings } from '@/types'

type LegalDocumentIcon = 'document' | 'shield' | 'globe' | 'cog'

const route = useRoute()
const settings = ref<PublicSettings | null>(null)
const loading = ref(true)
const loadError = ref(false)

marked.setOptions({
  breaks: true,
  gfm: true,
})

const documentId = computed(() => String(route.params.documentId || ''))
const documents = computed(() => settings.value?.login_agreement_documents ?? [])
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(settings.value?.site_logo || '', {
  allowRelative: true,
  allowDataUrl: true,
}))
const updatedAt = computed(() => settings.value?.login_agreement_updated_at || '')

const currentDocument = computed<LoginAgreementDocument | null>(() => {
  const id = documentId.value
  if (!id) {
    return null
  }
  return documents.value.find((doc) => doc.id === id) ?? null
})

const hasContent = computed(() => Boolean(currentDocument.value?.content_md?.trim()))

const renderedHtml = computed(() => {
  const content = currentDocument.value?.content_md?.trim() || ''
  if (!content) {
    return ''
  }
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
})

const documentIcon = computed<LegalDocumentIcon>(() => {
  const title = currentDocument.value?.title || ''
  if (title.includes('政策') || title.includes('隐私')) {
    return 'shield'
  }
  if (title.includes('国家') || title.includes('地区')) {
    return 'globe'
  }
  if (title.includes('特定')) {
    return 'cog'
  }
  return 'document'
})

onMounted(async () => {
  loading.value = true
  loadError.value = false
  try {
    settings.value = await getPublicSettings()
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.legal-document-content {
  line-height: 1.75;
  overflow-wrap: anywhere;
  color: inherit;
}

.legal-document-content :deep(h1) {
  @apply mb-4 mt-8 border-b border-gray-200 pb-3 text-3xl font-bold dark:border-dark-700;
}

.legal-document-content :deep(h2) {
  @apply mb-3 mt-7 text-2xl font-bold;
}

.legal-document-content :deep(h3) {
  @apply mb-2 mt-6 text-xl font-semibold;
}

.legal-document-content :deep(h4) {
  @apply mb-2 mt-5 text-lg font-semibold;
}

.legal-document-content :deep(p) {
  @apply mb-4 text-gray-700 dark:text-dark-200;
}

.legal-document-content :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200;
}

.legal-document-content :deep(ul) {
  @apply mb-4 list-disc pl-6;
}

.legal-document-content :deep(ol) {
  @apply mb-4 list-decimal pl-6;
}

.legal-document-content :deep(li) {
  @apply mb-1 text-gray-700 dark:text-dark-200;
}

.legal-document-content :deep(blockquote) {
  @apply my-5 border-l-4 border-gray-300 pl-4 text-gray-600 dark:border-dark-600 dark:text-dark-300;
}

.legal-document-content :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-800;
}

.legal-document-content :deep(pre) {
  @apply my-5 overflow-x-auto rounded-lg bg-gray-950 p-4 text-gray-100;
}

.legal-document-content :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}

.legal-document-content :deep(table) {
  @apply my-5 block w-full overflow-x-auto border-collapse;
}

.legal-document-content :deep(th) {
  @apply border border-gray-300 bg-gray-50 px-3 py-2 text-left font-semibold dark:border-dark-600 dark:bg-dark-800;
}

.legal-document-content :deep(td) {
  @apply border border-gray-300 px-3 py-2 dark:border-dark-600;
}

.legal-document-content :deep(img) {
  @apply my-5 h-auto max-w-full rounded-lg;
}

.legal-document-content :deep(hr) {
  @apply my-7 border-gray-200 dark:border-dark-700;
}
</style>
