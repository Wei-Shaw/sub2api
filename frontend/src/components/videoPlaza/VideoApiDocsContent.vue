<template>
  <article class="space-y-8 rounded-2xl border border-gray-200 bg-white p-6 shadow-card dark:border-dark-700/70 dark:bg-dark-800/60 md:p-10">
    <div>
      <p class="text-sm font-medium text-primary-600 dark:text-primary-300">BUAGA API</p>
      <h1 class="mt-2 text-3xl font-bold tracking-tight text-gray-900 dark:text-white">{{ t('videoDocs.title') }}</h1>
      <p class="mt-3 text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('videoDocs.intro') }}</p>
    </div>

    <section><h2>{{ t('videoDocs.authTitle') }}</h2><p>{{ t('videoDocs.authText') }}</p><pre><code>Authorization: Bearer sk-xxxxxxxx</code></pre></section>
    <section><h2>{{ t('videoDocs.createTitle') }}</h2><p>{{ t('videoDocs.createText') }}</p><pre><code>curl {{ apiBase }}/v1/videos \\
  -H "Authorization: Bearer $BUAGA_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "seedance-2.5",
    "prompt": "A slow cinematic camera move over a city at sunset",
    "duration": 10,
    "resolution": "720p",
    "ratio": "16:9"
  }'</code></pre></section>
    <section><h2>{{ t('videoDocs.requestTitle') }}</h2><ul><li><code>model</code>：{{ t('videoDocs.modelField') }}</li><li><code>prompt</code>：{{ t('videoDocs.promptField') }}</li><li><code>duration</code>：{{ t('videoDocs.durationField') }}</li><li><code>resolution</code>：{{ t('videoDocs.resolutionField') }}</li><li><code>ratio</code>：{{ t('videoDocs.ratioField') }}</li><li><code>images</code>：{{ t('videoDocs.imagesField') }}</li></ul></section>
    <section><h2>{{ t('videoDocs.queryTitle') }}</h2><pre><code>GET {{ apiBase }}/v1/videos/{video_id}
GET {{ apiBase }}/v1/videos/{video_id}/content</code></pre><p>{{ t('videoDocs.queryText') }}</p></section>
    <section><h2>{{ t('videoDocs.statusTitle') }}</h2><div class="overflow-x-auto"><table><thead><tr><th>{{ t('videoDocs.status') }}</th><th>{{ t('videoDocs.meaning') }}</th></tr></thead><tbody><tr><td><code>queued</code></td><td>{{ t('videoDocs.queued') }}</td></tr><tr><td><code>in_progress</code></td><td>{{ t('videoDocs.inProgress') }}</td></tr><tr><td><code>completed</code></td><td>{{ t('videoDocs.completed') }}</td></tr><tr><td><code>failed</code></td><td>{{ t('videoDocs.failed') }}</td></tr></tbody></table></div></section>
    <section><h2>{{ t('videoDocs.compatTitle') }}</h2><p>{{ t('videoDocs.compatText') }}</p><pre><code>POST /v1/videos
POST /v1/videos/generations
POST /v1/videos/edits
POST /v1/videos/extensions</code></pre></section>
    <section><h2>{{ t('videoDocs.billingTitle') }}</h2><p>{{ t('videoDocs.billingText') }}</p></section>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const apiBase = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)
</script>

<style scoped>
h2 { @apply mb-2 text-xl font-semibold text-gray-900 dark:text-white; }
p, li { @apply text-sm leading-7 text-gray-600 dark:text-dark-300; }
ul { @apply list-disc space-y-1 pl-5; }
pre { @apply mt-3 overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs leading-6 text-gray-100; }
table { @apply min-w-full border-collapse text-left text-sm; }
th, td { @apply border-b border-gray-200 px-4 py-3 dark:border-dark-700; }
th { @apply text-xs font-medium text-gray-500 dark:text-dark-400; }
</style>
