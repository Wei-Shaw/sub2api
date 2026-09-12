<template>
  <BaseDialog
    :show="show"
    :title="t('keys.availableModelsTitle')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('keys.availableModelsDescription') }}
      </p>

      <div
        v-if="loading"
        class="flex min-h-32 flex-col items-center justify-center gap-3 rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50"
      >
        <Icon name="refresh" size="md" class="animate-spin text-primary-500" />
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('keys.availableModelsLoading') }}
        </p>
      </div>

      <div
        v-else-if="error"
        class="flex min-h-32 flex-col items-center justify-center gap-3 rounded-xl border border-red-200 bg-red-50 px-4 text-center dark:border-red-900/60 dark:bg-red-950/30"
      >
        <Icon name="exclamationCircle" size="md" class="text-red-500" />
        <p class="text-sm text-red-700 dark:text-red-300">{{ error }}</p>
        <button type="button" class="btn btn-secondary min-h-9 px-3 text-xs" @click="emit('retry')">
          {{ t('keys.availableModelsRetry') }}
        </button>
      </div>

      <div
        v-else-if="models.length === 0"
        class="flex min-h-32 items-center justify-center rounded-xl border border-gray-200 bg-gray-50 px-4 text-center dark:border-dark-700 dark:bg-dark-800/50"
      >
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('keys.availableModelsEmpty') }}
        </p>
      </div>

      <div v-else class="space-y-3">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('keys.availableModelsCount', { count: models.length }) }}
        </p>
        <div class="grid max-h-96 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
          <div
            v-for="model in models"
            :key="model"
            class="flex items-center justify-between gap-2 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-200"
          >
            <span class="min-w-0 break-all">{{ model }}</span>
            <button
              type="button"
              data-test="copy-model"
              class="flex-shrink-0 rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-white hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="copiedModel === model ? t('keys.availableModelsCopied') : t('keys.availableModelsCopy')"
              :aria-label="copiedModel === model ? t('keys.availableModelsCopied') : t('keys.availableModelsCopy')"
              @click="copyModel(model)"
            >
              <Icon v-if="copiedModel === model" name="check" size="sm" class="text-emerald-500" />
              <Icon v-else name="clipboard" size="sm" />
            </button>
          </div>
        </div>
      </div>
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
import { onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

defineProps<{
  show: boolean
  models: string[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'retry'): void
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copiedModel = ref<string | null>(null)
let copiedResetTimer: number | null = null

const copyModel = async (model: string) => {
  const success = await copyToClipboard(model, t('keys.availableModelsCopied'))
  if (!success) return

  copiedModel.value = model
  if (copiedResetTimer !== null) {
    window.clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = window.setTimeout(() => {
    if (copiedModel.value === model) {
      copiedModel.value = null
    }
    copiedResetTimer = null
  }, 2000)
}

onUnmounted(() => {
  if (copiedResetTimer !== null) {
    window.clearTimeout(copiedResetTimer)
  }
})
</script>
