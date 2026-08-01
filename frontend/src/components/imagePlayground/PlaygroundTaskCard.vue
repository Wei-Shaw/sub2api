<template>
  <article
    class="group relative overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm transition duration-200 hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-md dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-600"
  >
    <button
      type="button"
      class="relative block aspect-square w-full overflow-hidden bg-gray-100 text-left dark:bg-dark-900"
      :disabled="task.status !== 'completed' || !task.images[0]?.url"
      @click="$emit('open', task)"
    >
      <img
        v-if="task.status === 'completed' && task.images[0]?.url"
        :src="task.images[0].url"
        :alt="task.prompt_preview || t('imagePlayground.generatedImage')"
        class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.02]"
        loading="lazy"
      />

      <div
        v-else-if="task.status === 'processing' || (task.status === 'completed' && task.images.length > 0)"
        class="absolute inset-0 flex flex-col items-center justify-center px-6"
      >
        <div class="playground-shimmer absolute inset-0" />
        <div class="relative flex h-12 w-12 items-center justify-center rounded-full border border-gray-200 bg-white/90 shadow-sm dark:border-dark-700 dark:bg-dark-800/90">
          <Icon name="sparkles" size="lg" class="animate-pulse text-gray-700 dark:text-gray-200" />
        </div>
        <p class="relative mt-4 text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ task.status === 'processing' ? t('imagePlayground.status.generating') : t('imagePlayground.status.loadingPreview') }}
        </p>
        <div class="relative mt-3 h-1 w-24 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <span class="playground-progress block h-full w-1/2 bg-gray-800 dark:bg-gray-100" />
        </div>
      </div>

      <div v-else class="absolute inset-0 flex flex-col items-center justify-center px-8 text-center">
        <span class="flex h-11 w-11 items-center justify-center rounded-full bg-red-50 text-red-500 dark:bg-red-950/30 dark:text-red-400">
          <Icon name="xCircle" size="lg" />
        </span>
        <p class="mt-3 text-sm font-medium text-gray-800 dark:text-gray-100">
          {{ t('imagePlayground.status.failed') }}
        </p>
        <p class="mt-1 line-clamp-2 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ errorMessage }}
        </p>
      </div>

      <div class="absolute right-3 top-3 flex max-w-[calc(100%-24px)] flex-col items-end gap-2">
        <div
          v-if="task.status === 'completed' && task.images.length > 1"
          class="rounded-md bg-black/65 px-2 py-1 text-xs font-medium text-white backdrop-blur"
        >
          {{ task.images.length }} {{ t('imagePlayground.images') }}
        </div>
        <ExpiryCountdown v-if="task.expires_at" :expires-at="task.expires_at" :now="now" />
      </div>

      <div
        v-if="(task.input_image_count || 0) > 0"
        class="absolute left-3 top-3 flex items-center gap-1 rounded-md bg-black/65 px-2 py-1 text-xs font-medium text-white backdrop-blur"
        :title="t('imagePlayground.references.usedCount', { count: task.input_image_count })"
      >
        <Icon name="upload" size="xs" />
        {{ task.input_image_count }}
      </div>

      <div
        v-if="task.status === 'completed'"
        class="absolute inset-x-0 bottom-0 flex translate-y-full items-center justify-end gap-1 bg-gradient-to-t from-black/70 to-transparent px-3 pb-3 pt-10 opacity-0 transition duration-200 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100"
      >
        <span class="flex h-9 w-9 items-center justify-center rounded-md bg-white/95 text-gray-800 shadow-sm" :title="t('imagePlayground.actions.preview')">
          <Icon name="eye" size="sm" />
        </span>
      </div>
    </button>

    <div class="p-3.5">
      <div class="flex min-w-0 items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="line-clamp-2 min-h-10 text-sm font-medium leading-5 text-gray-900 dark:text-white" :title="task.prompt_preview">
            {{ task.prompt_preview || t('imagePlayground.untitledPrompt') }}
          </p>
          <div class="mt-2 flex min-w-0 items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <span class="truncate">{{ task.model }}</span>
            <span aria-hidden="true">/</span>
            <span class="flex-shrink-0">{{ formattedTime }}</span>
          </div>
        </div>

        <div class="flex flex-shrink-0 items-center gap-0.5">
          <button
            v-if="task.status === 'completed' && task.images[0]?.url"
            type="button"
            class="task-action"
            :title="t('imagePlayground.actions.download')"
            @click="$emit('download', task, task.images[0].index)"
          >
            <Icon name="download" size="sm" />
          </button>
          <button
            type="button"
            class="task-action"
            :title="t('imagePlayground.actions.reuse')"
            @click="$emit('reuse', task)"
          >
            <Icon name="copy" size="sm" />
          </button>
          <button
            type="button"
            class="task-action"
            :title="t('imagePlayground.actions.regenerate')"
            @click="$emit('regenerate', task)"
          >
            <Icon name="refresh" size="sm" />
          </button>
        </div>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ExpiryCountdown from '@/components/imagePlayground/ExpiryCountdown.vue'
import type { ImagePlaygroundTask } from '@/api/imagePlayground'

const props = defineProps<{
  task: ImagePlaygroundTask
  now: number
}>()

defineEmits<{
  (event: 'open', task: ImagePlaygroundTask): void
  (event: 'download', task: ImagePlaygroundTask, imageIndex: number): void
  (event: 'reuse', task: ImagePlaygroundTask): void
  (event: 'regenerate', task: ImagePlaygroundTask): void
}>()

const { t, locale } = useI18n()

const formattedTime = computed(() => {
  const timestamp = props.task.created_at < 10_000_000_000
    ? props.task.created_at * 1000
    : props.task.created_at
  return new Intl.DateTimeFormat(locale.value, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp))
})

const errorMessage = computed(() => {
  const error = props.task.error
  if (!error) return t('imagePlayground.errors.generationFailed')
  if (typeof error === 'string') return error
  if (typeof error === 'object') {
    const record = error as Record<string, unknown>
    if (typeof record.message === 'string') return record.message
    if (typeof record.error === 'object' && record.error) {
      const nested = record.error as Record<string, unknown>
      if (typeof nested.message === 'string') return nested.message
    }
  }
  return t('imagePlayground.errors.generationFailed')
})
</script>

<style scoped>
.task-action {
  @apply flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-gray-500 dark:hover:bg-dark-700 dark:hover:text-white;
}

.playground-shimmer {
  background: linear-gradient(110deg, transparent 25%, rgb(255 255 255 / 55%) 45%, transparent 65%);
  background-size: 220% 100%;
  animation: playground-shimmer 2.2s ease-in-out infinite;
}

.dark .playground-shimmer {
  background: linear-gradient(110deg, transparent 25%, rgb(255 255 255 / 5%) 45%, transparent 65%);
  background-size: 220% 100%;
}

.playground-progress {
  animation: playground-progress 1.5s ease-in-out infinite;
}

@keyframes playground-shimmer {
  from { background-position: 150% 0; }
  to { background-position: -80% 0; }
}

@keyframes playground-progress {
  0% { transform: translateX(-110%); }
  100% { transform: translateX(210%); }
}
</style>
