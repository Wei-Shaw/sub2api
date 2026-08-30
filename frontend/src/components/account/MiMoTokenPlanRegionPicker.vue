<template>
  <div class="flex flex-wrap gap-2" data-testid="mimo-token-plan-region-picker">
    <button
      v-for="region in MIMO_TOKEN_PLAN_REGIONS"
      :key="region.value"
      type="button"
      :data-testid="`mimo-token-plan-region-${region.value}`"
      :class="[
        'rounded-md border px-2.5 py-1 text-xs font-medium transition-colors',
        modelValue === region.value
          ? 'border-sky-500 bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
          : 'border-gray-200 text-gray-600 hover:border-sky-300 hover:text-sky-700 dark:border-dark-600 dark:text-gray-300'
      ]"
      @click="$emit('update:modelValue', region.value)"
    >
      {{ region.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { MIMO_TOKEN_PLAN_REGIONS, type MiMoTokenPlanRegion } from './credentialsBuilder'

defineProps<{ modelValue: MiMoTokenPlanRegion | null }>()
defineEmits<{ (event: 'update:modelValue', value: MiMoTokenPlanRegion): void }>()
</script>
