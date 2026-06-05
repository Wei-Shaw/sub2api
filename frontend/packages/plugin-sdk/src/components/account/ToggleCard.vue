<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
    <div class="flex items-center justify-between" :class="{ 'mb-3': enabled && hasContent }">
      <div>
        <label class="input-label mb-0">{{ label }}</label>
        <p v-if="hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ hint }}</p>
      </div>
      <button type="button" @click="emit('update:enabled', !enabled)"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]">
        <span :class="[
          'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
          enabled ? 'translate-x-5' : 'translate-x-0'
        ]" />
      </button>
    </div>
    <div v-if="enabled">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
defineProps<{ label: string; hint?: string; enabled: boolean }>()
const emit = defineEmits<{ 'update:enabled': [value: boolean] }>()
const slots = useSlots()
const hasContent = computed(() => !!slots.default)
</script>