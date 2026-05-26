<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.poolMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.poolModeHint') }}
        </p>
      </div>
      <button
        type="button"
        @click="emit('update:enabled', !enabled)"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            enabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
    <div v-if="enabled" class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
      <p class="text-xs text-blue-700 dark:text-blue-400">
        <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
        {{ t('admin.accounts.poolModeInfo') }}
      </p>
    </div>
    <div v-if="enabled" class="mt-3">
      <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') }}</label>
      <input
        :value="retryCount"
        @input="onRetryInput"
        type="number"
        min="0"
        :max="MAX_RETRY"
        step="1"
        class="input"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.poolModeRetryCountHint', { default: DEFAULT_RETRY, max: MAX_RETRY }) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'

const DEFAULT_RETRY = 3
const MAX_RETRY = 10

defineProps<{
  enabled: boolean
  retryCount: number
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:retryCount': [value: number]
}>()

const { t } = useI18n()

const onRetryInput = (e: Event) => {
  const val = Number((e.target as HTMLInputElement).value)
  emit('update:retryCount', val)
}
</script>
