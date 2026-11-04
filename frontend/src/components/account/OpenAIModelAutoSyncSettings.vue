<template>
  <div class="mb-4 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="openai-model-auto-sync-settings">
    <label class="flex min-h-11 cursor-pointer items-center gap-3 text-sm font-medium text-gray-900 dark:text-white">
      <input
        type="checkbox"
        :checked="modelValue"
        :disabled="disabled"
        :aria-describedby="`${id}-help`"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed dark:border-dark-500 dark:bg-dark-700"
        data-testid="openai-model-auto-sync-toggle"
        @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
      />
      {{ t('admin.accounts.openai.modelAutoSync.title') }}
    </label>
    <div :id="`${id}-help`" class="space-y-1 text-xs leading-relaxed text-gray-600 dark:text-gray-400">
      <p>{{ t('admin.accounts.openai.modelAutoSync.schedule') }}</p>
      <p>{{ t('admin.accounts.openai.modelAutoSync.preservation') }}</p>
      <p>{{ t('admin.accounts.openai.modelAutoSync.retention') }}</p>
      <p v-if="modelValue && unrestricted" class="text-amber-700 dark:text-amber-400" data-testid="openai-model-auto-sync-whitelist-hint">
        {{ t('admin.accounts.openai.modelAutoSync.unrestrictedHint') }}
      </p>
    </div>
    <div class="mt-3 space-y-1 border-t border-gray-200 pt-2 text-xs text-gray-600 dark:border-dark-600 dark:text-gray-400" role="status" aria-live="polite">
      <p data-testid="openai-model-auto-sync-status">{{ statusLabel }}</p>
      <p>
        {{ t('admin.accounts.openai.modelAutoSync.lastSuccess') }}
        <time v-if="lastSuccessAt" :datetime="lastSuccessAt">{{ formatDateTimeToMinute(lastSuccessAt) }}</time>
        <span v-else>{{ t('admin.accounts.openai.modelAutoSync.neverSynced') }}</span>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIModelAutoSyncState } from '@/types'
import { formatDateTimeToMinute } from '@/utils/format'

const props = withDefaults(defineProps<{
  modelValue: boolean
  state?: OpenAIModelAutoSyncState
  unrestricted?: boolean
  disabled?: boolean
}>(), { unrestricted: false, disabled: false })

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const { t } = useI18n()
const id = useId()
const lastSuccessAt = computed(() => {
  const value = props.state?.last_success_at
  return typeof value === 'string' && Number.isFinite(new Date(value).getTime()) ? value : null
})
const statusLabel = computed(() => {
  if (!props.modelValue) return t('admin.accounts.openai.modelAutoSync.disabled')
  // Upstream error details may contain sensitive data. Only show fixed status labels.
  if (props.state?.status === 'error') {
    return t(props.state.error_code === 'empty_mapping'
      ? 'admin.accounts.openai.modelAutoSync.emptyMapping'
      : 'admin.accounts.openai.modelAutoSync.error')
  }
  if (props.state?.status === 'success') return t('admin.accounts.openai.modelAutoSync.success')
  return t('admin.accounts.openai.modelAutoSync.pending')
})
</script>
