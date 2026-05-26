<template>
  <select
    class="block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100 text-sm"
    :value="String(modelValue ?? '')"
    @change="(e) => onChange((e.target as HTMLSelectElement).value)"
  >
    <option
      v-for="opt in props.prop.enumValues"
      :key="String(opt.value)"
      :value="String(opt.value)"
    >
      {{ opt.label }}
    </option>
  </select>
</template>

<script setup lang="ts">
// SETTINGS-V2 enum widget — `<select>` works on string values, so we
// coerce on the way in and resolve back to the original schema value
// (which may be a number or boolean) on the way out.
import type { WidgetProps } from '../types'

const props = defineProps<WidgetProps>()
const emit = defineEmits<{ (e: 'update:modelValue', v: unknown): void }>()

function onChange(raw: string) {
  const match = props.prop.enumValues?.find((o) => String(o.value) === raw)
  emit('update:modelValue', match ? match.value : raw)
}
</script>
