<template>
  <BaseDialog
    :show="show"
    :title="t('keys.modelsModal.title', { name: keyName })"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('keys.modelsModal.description') }}
      </p>

      <div v-if="loading" class="flex min-h-48 items-center justify-center gap-2 text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="sm" class="animate-spin" />
        <span>{{ t('keys.modelsModal.loading') }}</span>
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-48 flex-col items-center justify-center gap-3 rounded-xl border border-red-200 bg-red-50 px-6 text-center dark:border-red-900/60 dark:bg-red-950/20"
      >
        <Icon name="exclamationCircle" size="lg" class="text-red-500" />
        <p class="max-w-lg text-sm text-red-700 dark:text-red-300">
          {{ t('keys.modelsModal.failedToLoad') }}
        </p>
        <button type="button" class="btn btn-secondary text-sm" @click="loadModels">
          {{ t('keys.modelsModal.retry') }}
        </button>
      </div>

      <template v-else>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="relative min-w-0 flex-1">
            <Icon
              name="search"
              size="sm"
              class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <input
              v-model="searchQuery"
              data-testid="model-search"
              type="search"
              class="input w-full pl-9"
              :placeholder="t('keys.modelsModal.searchPlaceholder')"
              :aria-label="t('keys.modelsModal.searchPlaceholder')"
            />
          </div>
          <span class="shrink-0 text-sm text-gray-500 dark:text-gray-400">
            {{ t('keys.modelsModal.count', { count: models.length }) }}
          </span>
        </div>

        <div
          v-if="filteredModels.length > 0"
          class="max-h-96 divide-y divide-gray-100 overflow-y-auto rounded-xl border border-gray-200 dark:divide-dark-700 dark:border-dark-600"
        >
          <div
            v-for="model in filteredModels"
            :key="model.id"
            data-testid="model-row"
            class="group flex items-center gap-3 px-3 py-2.5 hover:bg-gray-50 dark:hover:bg-dark-700"
          >
            <ModelIcon :model="model.id" size="18px" class="shrink-0" />
            <div class="min-w-0 flex-1">
              <p class="truncate font-mono text-sm text-gray-900 dark:text-white">
                {{ model.id }}
              </p>
              <p
                v-if="model.display_name && model.display_name !== model.id"
                class="mt-0.5 truncate text-xs text-gray-400 dark:text-gray-500"
              >
                {{ model.display_name }}
              </p>
            </div>
            <button
              type="button"
              data-testid="copy-model-id"
              class="shrink-0 rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-200 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-600 dark:hover:text-primary-400"
              :title="t('keys.modelsModal.copy')"
              :aria-label="`${t('keys.modelsModal.copy')}: ${model.id}`"
              @click="copyModelId(model.id)"
            >
              <Icon name="copy" size="sm" />
            </button>
          </div>
        </div>

        <div
          v-else
          class="flex min-h-48 items-center justify-center rounded-xl border border-dashed border-gray-200 px-6 text-center text-sm text-gray-400 dark:border-dark-600 dark:text-gray-500"
        >
          {{ models.length === 0 ? t('keys.modelsModal.empty') : t('keys.modelsModal.noMatches') }}
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import type { ApiKeyModel } from '@/api/keys'
import { useClipboard } from '@/composables/useClipboard'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  apiKey: string
  keyName: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const models = ref<ApiKeyModel[]>([])
const searchQuery = ref('')
const loading = ref(false)
const loadError = ref(false)
let abortController: AbortController | null = null

const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return models.value

  return models.value.filter((model) =>
    model.id.toLowerCase().includes(query) ||
    model.display_name?.toLowerCase().includes(query)
  )
})

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  return (error as { name?: string }).name === 'AbortError'
}

const loadModels = async () => {
  abortController?.abort()
  loadError.value = false
  models.value = []
  searchQuery.value = ''

  if (!props.apiKey) {
    loadError.value = true
    return
  }

  const controller = new AbortController()
  abortController = controller
  loading.value = true

  try {
    models.value = await keysAPI.listModels(props.apiKey, { signal: controller.signal })
  } catch (error) {
    if (!isAbortError(error)) {
      loadError.value = true
    }
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const copyModelId = async (modelId: string) => {
  await copyToClipboard(modelId)
}

watch(
  () => [props.show, props.apiKey] as const,
  ([show]) => {
    if (show) {
      void loadModels()
    } else {
      abortController?.abort()
      abortController = null
      loading.value = false
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  abortController?.abort()
})
</script>
