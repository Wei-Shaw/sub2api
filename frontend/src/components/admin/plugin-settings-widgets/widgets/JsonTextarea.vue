<template>
  <textarea
    rows="4"
    class="block w-full rounded-md font-mono text-xs border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
    :value="rawJson"
    @input="(e) => onInput((e.target as HTMLTextAreaElement).value)"
  />
  <p v-if="parseError" class="mt-1 text-xs text-red-600 dark:text-red-400">
    {{ parseError }}
  </p>
</template>

<script setup lang="ts">
// SETTINGS-V2 fallback widget. Used for any property whose schema type
// is `object` / `array` / unknown, OR whenever the schema lacks a
// declared type. Emits a parsed JS value (object/array/scalar); on
// parse error, the error text is shown locally and `update:modelValue`
// is held back so the form layer never sees an invalid intermediate.
import { computed, ref } from 'vue'

import type { WidgetProps } from '../types'

const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()

const parseError = ref('')

const rawJson = computed(() => {
  if (props.modelValue === undefined) return ''
  try {
    return JSON.stringify(props.modelValue, null, 2)
  } catch {
    return String(props.modelValue)
  }
})

function onInput(raw: string) {
  if (raw.trim() === '') {
    parseError.value = ''
    emit('update:modelValue', '')
    return
  }
  try {
    const parsed = JSON.parse(raw)
    parseError.value = ''
    emit('update:modelValue', parsed)
  } catch (err) {
    parseError.value = err instanceof Error ? err.message : String(err)
  }
}
</script>
