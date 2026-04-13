<script setup lang="ts">
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
</script>

<template>
  <div class="flex items-center gap-1.5">
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
    <template v-if="enabled">
      <input
        :value="threshold"
        @input="emit('update:threshold', parseFloat(($event.target as HTMLInputElement).value) || null)"
        type="number"
        min="0"
        :max="thresholdType === 'percentage' ? 100 : undefined"
        :step="thresholdType === 'percentage' ? 1 : 0.01"
        class="input py-1 text-sm flex-1 min-w-0"
      />
      <select
        :value="thresholdType || 'fixed'"
        @change="emit('update:thresholdType', ($event.target as HTMLSelectElement).value)"
        class="input py-1 text-xs w-16 flex-shrink-0 text-center"
      >
        <option value="fixed">$</option>
        <option value="percentage">%</option>
      </select>
    </template>
  </div>
</template>
