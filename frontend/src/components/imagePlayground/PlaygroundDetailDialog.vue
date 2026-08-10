<template>
  <BaseDialog
    :show="show"
    :title="t('imagePlayground.detail.title')"
    width="extra-wide"
    :close-on-click-outside="true"
    @close="$emit('close')"
  >
    <div v-if="task" class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div class="min-w-0">
        <div class="relative h-[min(52vh,560px)] min-h-[280px] overflow-hidden rounded-lg bg-gray-100 dark:bg-dark-950 sm:h-[min(55vh,620px)] lg:h-[min(62vh,680px)]">
          <img
            v-if="selectedImage"
            :src="selectedImage.url"
            :alt="task.prompt_preview || t('imagePlayground.generatedImage')"
            class="h-full w-full object-contain"
          />
          <ExpiryCountdown
            v-if="selectedImage && task.expires_at"
            class="absolute right-3 top-3"
            :expires-at="task.expires_at"
            :now="now"
          />
        </div>

        <div v-if="task.images.length > 1" class="mt-3 grid grid-cols-4 gap-2 sm:grid-cols-6">
          <button
            v-for="image in task.images"
            :key="image.index"
            type="button"
            class="aspect-square overflow-hidden rounded-md border-2 bg-gray-100 transition dark:bg-dark-900"
            :class="selectedIndex === image.index ? 'border-gray-900 dark:border-white' : 'border-transparent hover:border-gray-300 dark:hover:border-dark-500'"
            @click="selectedIndex = image.index"
          >
            <img
              v-if="image.url"
              :src="image.url"
              :alt="`${t('imagePlayground.generatedImage')} ${image.index + 1}`"
              class="h-full w-full object-cover"
            />
            <span v-else class="block h-full w-full animate-pulse bg-gray-200 dark:bg-dark-700" />
          </button>
        </div>
      </div>

      <aside class="flex min-w-0 flex-col border-t border-gray-200 pt-5 dark:border-dark-700 lg:border-l lg:border-t-0 lg:pl-5 lg:pt-0">
        <div>
          <p class="text-xs font-semibold uppercase text-gray-400 dark:text-gray-500">
            {{ t('imagePlayground.detail.prompt') }}
          </p>
          <p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-gray-800 dark:text-gray-200">
            {{ fullPrompt || task.prompt_preview || t('imagePlayground.untitledPrompt') }}
          </p>
        </div>

        <dl class="mt-6 grid grid-cols-2 gap-x-4 gap-y-4 border-t border-gray-200 pt-5 text-sm dark:border-dark-700">
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('imagePlayground.fields.model') }}</dt>
            <dd class="mt-1 truncate font-medium text-gray-900 dark:text-white" :title="task.model">{{ task.model }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('imagePlayground.fields.platform') }}</dt>
            <dd class="mt-1 capitalize font-medium text-gray-900 dark:text-white">{{ task.platform }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('imagePlayground.detail.createdAt') }}</dt>
            <dd class="mt-1 font-medium text-gray-900 dark:text-white">{{ formattedTime }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('imagePlayground.detail.expiresAt') }}</dt>
            <dd class="mt-1 font-medium text-gray-900 dark:text-white">
              <span class="block">{{ formattedExpiry }}</span>
              <span class="mt-1 block text-xs font-normal tabular-nums text-gray-500 dark:text-gray-400">{{ expiryCountdownText }}</span>
            </dd>
          </div>
        </dl>

        <div class="mt-6 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-1">
          <button type="button" class="btn btn-primary w-full" @click="$emit('download', task, selectedIndex)">
            <Icon name="download" size="sm" class="mr-2" />
            {{ t('imagePlayground.actions.download') }}
          </button>
          <button type="button" class="btn btn-secondary w-full" @click="$emit('reuse', task)">
            <Icon name="copy" size="sm" class="mr-2" />
            {{ t('imagePlayground.actions.reuse') }}
          </button>
          <button type="button" class="btn btn-danger w-full sm:col-span-2 lg:col-span-1" @click="$emit('deleteImage', task, selectedIndex)">
            <Icon name="trash" size="sm" class="mr-2" />
            {{ t('imagePlayground.actions.deleteImage') }}
          </button>
        </div>
      </aside>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ExpiryCountdown from '@/components/imagePlayground/ExpiryCountdown.vue'
import type { ImagePlaygroundTask } from '@/api/imagePlayground'

const props = defineProps<{
  show: boolean
  task: ImagePlaygroundTask | null
  fullPrompt?: string
  now: number
}>()

defineEmits<{
  (event: 'close'): void
  (event: 'download', task: ImagePlaygroundTask, imageIndex: number): void
  (event: 'reuse', task: ImagePlaygroundTask): void
  (event: 'deleteImage', task: ImagePlaygroundTask, imageIndex: number): void
}>()

const { t, locale } = useI18n()
const selectedIndex = ref(0)

watch(
  () => [props.task?.id, props.task?.images.map((image) => image.index).join(',')],
  () => {
    if (!props.task?.images.some((image) => image.index === selectedIndex.value)) {
      selectedIndex.value = props.task?.images[0]?.index ?? 0
    }
  },
  { immediate: true },
)

const selectedImage = computed(() => props.task?.images.find((image) => image.index === selectedIndex.value))

function toDate(timestamp: number): Date {
  return new Date(timestamp < 10_000_000_000 ? timestamp * 1000 : timestamp)
}

const dateFormatter = computed(() => new Intl.DateTimeFormat(locale.value, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
}))

const formattedTime = computed(() => props.task ? dateFormatter.value.format(toDate(props.task.created_at)) : '')
const formattedExpiry = computed(() => props.task?.expires_at ? dateFormatter.value.format(toDate(props.task.expires_at)) : '')
const expiryCountdownText = computed(() => {
  if (!props.task?.expires_at) return ''
  const expiry = toDate(props.task.expires_at).getTime()
  if (expiry <= props.now) return t('imagePlayground.retention.expired')
  const seconds = Math.max(0, Math.ceil((expiry - props.now) / 1000))
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const remaining = days > 0
    ? `${days}${t('imagePlayground.retention.dayUnit')} ${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
    : `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
  return t('imagePlayground.retention.countdown', { time: remaining })
})
</script>
