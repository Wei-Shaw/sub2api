<template>
  <input
    type="number"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    step="any"
    :value="numberValue"
    @input="(e) => onInput((e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
// SETTINGS-V2 number widget — emits a `number` whenever the input is a
// finite parse, otherwise echoes the raw string back so the form layer
// can surface the schema validation error.
import { computed } from 'vue'

import type { WidgetProps } from '../types'

const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()

const numberValue = computed(() => {
  const v = props.modelValue
  if (typeof v === 'number') return String(v)
  if (typeof v === 'string') return v
  return ''
})

function onInput(raw: string) {
  const n = Number.parseFloat(raw)
  emit('update:modelValue', Number.isNaN(n) ? raw : n)
}
</script>
