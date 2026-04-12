<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  enabled: boolean | null
  threshold: number | null
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean | null]
  'update:threshold': [value: number | null]
}>()
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
    <div v-if="enabled" class="relative flex-1">
      <span class="absolute left-2 top-1/2 -translate-y-1/2 text-gray-400 text-sm">$</span>
      <input
        :value="threshold"
        @input="emit('update:threshold', parseFloat(($event.target as HTMLInputElement).value) || null)"
        type="number"
        min="0"
        step="0.01"
        class="input pl-6 py-1 text-sm"
      />
    </div>
  </div>
</template>
