<template>
  <div class="flex items-center gap-2">
    <label class="flex cursor-pointer items-center gap-2">
      <input type="checkbox" :checked="modelValue" @change="emit('update:modelValue', !modelValue)"
        class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500" />
      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ label }}</span>
    </label>
    <div v-if="tooltip" class="group relative">
      <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500">?</span>
      <div class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700">
        {{ tooltip }}
        <div class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ modelValue: boolean; label: string; tooltip?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>
