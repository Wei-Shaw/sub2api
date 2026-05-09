<template>
  <AppLayout>
    <div class="custom-page-layout">
      <div class="card flex-1 min-h-0 overflow-hidden">
        <div v-if="loading" class="flex h-full items-center justify-center py-12">
          <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        </div>
        <div v-else class="flex h-full overflow-hidden">
          <div
            class="markdown-page-content flex-1 h-full overflow-auto p-6 md:p-10"
            v-html="renderedHtml"
          ></div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { updateRouteSEO } from '@/utils/seo'
import { useRoute } from 'vue-router'
import { sanitizePublicHTML } from '@/utils/publicContent'

interface TutorialDocumentPayload {
  content_html: string
}

const appStore = useAppStore()
const route = useRoute()
const loading = ref(true)
const renderedHtml = ref('')

onMounted(async () => {
  loading.value = true
  try {
    if (!appStore.publicSettingsLoaded) {
      await appStore.fetchPublicSettings()
    }

    const resp = await fetch('/api/v1/tutorial-document', {
      headers: { Accept: 'application/json' },
    })

    if (resp.ok) {
      const payload = await resp.json() as { code?: number; data?: TutorialDocumentPayload }
      renderedHtml.value = sanitizePublicHTML(payload?.data?.content_html || '')
    } else {
      renderedHtml.value = '<p class="text-red-500">页面未找到。</p>'
    }

    updateRouteSEO(route, appStore.cachedPublicSettings)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.custom-page-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
}

.markdown-page-content {
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.markdown-page-content :deep(h1) {
  @apply mb-4 mt-8 border-b border-gray-200 pb-3 text-3xl font-bold dark:border-dark-700;
}

.markdown-page-content :deep(h2) {
  @apply mb-3 mt-7 text-2xl font-bold;
}

.markdown-page-content :deep(h3) {
  @apply mb-2 mt-6 text-xl font-semibold;
}

.markdown-page-content :deep(p) {
  @apply mb-4 text-gray-700 dark:text-dark-200;
}

.markdown-page-content :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200;
}

.markdown-page-content :deep(ul) {
  @apply mb-4 list-disc pl-6;
}

.markdown-page-content :deep(ol) {
  @apply mb-4 list-decimal pl-6;
}

.markdown-page-content :deep(img) {
  @apply my-5 h-auto max-w-full rounded-lg;
}

.markdown-page-content :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-sm dark:bg-dark-700;
}

.markdown-page-content :deep(pre) {
  @apply my-4 overflow-x-auto rounded-xl bg-gray-900 p-4 text-gray-100;
}

.markdown-page-content :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}
</style>
