<template>
  <input
    type="text"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :value="String(modelValue ?? '')"
    @input="(e) => emit('update:modelValue', (e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
// SETTINGS-V2 plain-string widget. Non-secret strings only — anything
// flagged `visibility: secret` resolves to SecretInput in resolveWidget.
import type { WidgetProps } from '../types'

defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()
</script>
