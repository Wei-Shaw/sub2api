<template>
  <input
    type="checkbox"
    class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
    :checked="Boolean(modelValue)"
    @change="(e) => emit('update:modelValue', (e.target as HTMLInputElement).checked)"
  />
</template>

<script setup lang="ts">
// SETTINGS-V2 boolean widget — emits the raw `checked` flag, no
// coercion. Schema-level `default` is applied server-side, so a never
// -touched checkbox simply does not appear dirty.
import type { WidgetProps } from '../types'

defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
</script>
