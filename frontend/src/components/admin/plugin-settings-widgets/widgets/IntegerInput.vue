<template>
  <input
    type="number"
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    step="1"
    :value="numberValue"
    @input="(e) => onInput((e.target as HTMLInputElement).value)"
  />
</template>

<script setup lang="ts">
// SETTINGS-V2 integer widget — identical UX to NumberInput, but parses
// with `parseInt(_, 10)` so e.g. `1.5` becomes `1` (and the schema
// validator rejects fractional input on the way to the server).
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
  const n = Number.parseInt(raw, 10)
  emit('update:modelValue', Number.isNaN(n) ? raw : n)
}
</script>
