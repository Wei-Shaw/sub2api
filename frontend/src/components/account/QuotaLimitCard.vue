<script setup lang="ts">
import { ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()

const props = defineProps<{
  modelValue: number | null
REDACTED>()

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
REDACTED>()

const enabled = ref(props.modelValue != null && props.modelValue > 0)

// Sync enabled state when modelValue changes externally (e.g. account load)
watch(
  () => props.modelValue,
  (val) => {
    enabled.value = val != null && val > 0
  REDACTED
)

// When toggle is turned off, clear the value
watch(enabled, (val) => {
  if (!val) {
    emit('update:modelValue', null)
  REDACTED
REDACTED)

const onInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:modelValue', Number.isNaN(raw) ? null : raw)
REDACTED
</script>

<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
    <div class="mb-3">
      <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaLimit') REDACTEDREDACTED</h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaLimitHint') REDACTEDREDACTED
      </p>
    </div>

    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaLimitToggle') REDACTEDREDACTED</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaLimitToggleHint') REDACTEDREDACTED
          </p>
        </div>
        <button
          type="button"
          @click="enabled = !enabled"
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

      <div v-if="enabled" class="space-y-3">
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaLimitAmount') REDACTEDREDACTED</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="modelValue"
              @input="onInput"
              type="number"
              min="0"
              step="0.01"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaLimitAmountHint') REDACTEDREDACTED</p>
        </div>
      </div>
    </div>
  </div>
</template>
