<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  enabled: boolean | null
  threshold: number | null
  thresholdType: string | null // "fixed" (default) or "percentage"
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean | null]
  'update:threshold': [value: number | null]
  'update:thresholdType': [value: string | null]
}>()

function toggleType(current: string | null) {
  emit('update:thresholdType', current === 'percentage' ? 'fixed' : 'percentage')
}
</script>

<template>
  <div class="ml-4 mt-2 flex items-center gap-3">
    <label class="text-sm text-gray-500 whitespace-nowrap">{{ t('admin.accounts.quotaNotify.alert') }}</label>
    <button
      type="button"
      @click="emit('update:enabled', !enabled)"
      :class="[
        'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
        enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
      ]"
    >
      <span
        :class="[
          'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
          enabled ? 'translate-x-4' : 'translate-x-0'
        ]"
      />
    </button>
    <div v-if="enabled" class="flex items-center gap-1 flex-1">
      <button
        type="button"
        class="px-1.5 py-0.5 text-xs font-medium rounded border transition-colors"
        :class="(!thresholdType || thresholdType === 'fixed') ? 'bg-primary-100 text-primary-700 border-primary-300 dark:bg-primary-900/30 dark:text-primary-400 dark:border-primary-700' : 'bg-gray-100 text-gray-500 border-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:border-dark-500'"
        @click="toggleType(thresholdType)"
      >
        $
      </button>
      <button
        type="button"
        class="px-1.5 py-0.5 text-xs font-medium rounded border transition-colors"
        :class="thresholdType === 'percentage' ? 'bg-primary-100 text-primary-700 border-primary-300 dark:bg-primary-900/30 dark:text-primary-400 dark:border-primary-700' : 'bg-gray-100 text-gray-500 border-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:border-dark-500'"
        @click="toggleType(thresholdType)"
      >
        %
      </button>
      <input
        :value="threshold"
        @input="emit('update:threshold', parseFloat(($event.target as HTMLInputElement).value) || null)"
        type="number"
        min="0"
        :max="thresholdType === 'percentage' ? 100 : undefined"
        :step="thresholdType === 'percentage' ? 1 : 0.01"
        class="input py-1 text-sm flex-1"
        :placeholder="thresholdType === 'percentage' ? t('admin.accounts.quotaNotify.thresholdPlaceholder') : t('admin.accounts.quotaNotify.threshold')"
      />
    </div>
  </div>
</template>
