<template>
  <div
    class="inline-flex items-center rounded-full border border-gray-200 bg-white p-0.5 text-xs font-medium shadow-sm dark:border-dark-700 dark:bg-dark-800"
  >
    <button
      v-for="opt in options"
      :key="opt"
      type="button"
      :class="[
        'rounded-full px-3 py-1 transition-colors',
        modelValue === opt
          ? 'bg-primary-500 text-white shadow'
          : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700',
      ]"
      :aria-pressed="modelValue === opt"
      @click="emit('update:modelValue', opt)"
    >
      {{ opt }}
    </button>
  </div>
</template>

<script setup lang="ts">
import type { PlazaCurrency } from '@/composables/useCurrencyToggle'

defineProps<{
  modelValue: PlazaCurrency
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: PlazaCurrency): void
}>()

const options: PlazaCurrency[] = ['CNY', 'USD']
</script>
