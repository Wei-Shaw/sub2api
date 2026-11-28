<template>
  <fieldset class="border-t border-gray-100 pt-4 dark:border-dark-700">
    <legend class="text-sm font-medium text-gray-900 dark:text-white">
      {{ t('admin.settings.latencyThresholds.title') }}
    </legend>
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.latencyThresholds.description') }}
    </p>
    <div class="mt-4 grid grid-cols-1 gap-6 md:grid-cols-3">
      <div v-for="field in fields" :key="field.key">
        <label
          :for="`first-token-${field.key}`"
          class="mb-2 block text-sm font-medium"
          :class="field.color"
        >
          {{ t(field.label) }}
        </label>
        <input
          :id="`first-token-${field.key}`"
          :data-testid="`first-token-${field.key}`"
          :value="modelValue[field.key] / 1000"
          type="number"
          min="0.001"
          step="0.001"
          required
          class="input w-full"
          @input="updateSeconds(field.key, $event)"
        />
      </div>
    </div>
    <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.latencyThresholds.hint') }}
    </p>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { FirstTokenThresholds } from '@/types'

const props = defineProps<{ modelValue: FirstTokenThresholds }>()
const emit = defineEmits<{ 'update:modelValue': [value: FirstTokenThresholds] }>()
const { t } = useI18n()

const fields = [
  { key: 'warn', label: 'admin.settings.latencyThresholds.warn', color: 'text-amber-600 dark:text-amber-400' },
  { key: 'slow', label: 'admin.settings.latencyThresholds.slow', color: 'text-orange-600 dark:text-orange-400' },
  { key: 'critical', label: 'admin.settings.latencyThresholds.critical', color: 'text-red-600 dark:text-red-400' },
] as const

function updateSeconds(key: keyof FirstTokenThresholds, event: Event) {
  const seconds = (event.target as HTMLInputElement).valueAsNumber
  emit('update:modelValue', { ...props.modelValue, [key]: Math.round(seconds * 1000) })
}
</script>
