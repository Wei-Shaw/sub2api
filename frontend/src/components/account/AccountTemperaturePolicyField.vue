<template>
  <fieldset>
    <legend class="input-label">{{ t('admin.accounts.temperature.label') }}</legend>
    <div
      class="grid gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-700"
      :class="allowUnchanged ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-3'"
    >
      <button
        v-for="option in modeOptions"
        :key="option"
        type="button"
        :data-testid="`temperature-mode-${option}`"
        :aria-pressed="mode === option"
        :class="[
          'min-h-9 rounded px-2 py-2 text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1',
          mode === option
            ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-300'
            : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
        ]"
        @click="emit('update:mode', option)"
      >
        {{ t(`admin.accounts.temperature.modes.${option}`) }}
      </button>
    </div>

    <label v-if="mode === 'override'" class="mt-3 block">
      <span class="input-label text-xs">{{ t('admin.accounts.temperature.value') }}</span>
      <input
        :value="temperature ?? ''"
        type="number"
        step="any"
        required
        class="input"
        data-testid="temperature-value"
        @input="handleTemperatureInput"
      />
    </label>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountTemperatureSelectionMode } from './credentialsBuilder'

const props = withDefaults(
  defineProps<{
    mode: AccountTemperatureSelectionMode
    temperature: number | null
    allowUnchanged?: boolean
  }>(),
  { allowUnchanged: false }
)

const emit = defineEmits<{
  'update:mode': [mode: AccountTemperatureSelectionMode]
  'update:temperature': [temperature: number | null]
}>()

const { t } = useI18n()
const modeOptions = computed<AccountTemperatureSelectionMode[]>(() =>
  props.allowUnchanged
    ? ['unchanged', 'inherit', 'override', 'omit']
    : ['inherit', 'override', 'omit']
)

const handleTemperatureInput = (event: Event) => {
  const rawValue = (event.target as HTMLInputElement).value
  if (rawValue === '') {
    emit('update:temperature', null)
    return
  }
  const value = Number(rawValue)
  emit('update:temperature', Number.isFinite(value) ? value : null)
}
</script>
