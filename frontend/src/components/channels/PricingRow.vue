<template>
  <div class="flex justify-between gap-2">
    <span class="text-gray-500 dark:text-gray-400">{{ label REDACTEDREDACTED</span>
    <span class="font-mono">{{ display REDACTEDREDACTED</span>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    value: number | null
    unit: string
    scale: number
  REDACTED>(),
  { value: null REDACTED
)

function formatScaled(value: number | null, scale: number): string {
  if (value == null) return '-'
  return `$${(value * scale).toPrecision(10).replace(/\.?0+$/, '')REDACTED`
REDACTED

const display = computed(() =>
  props.value == null ? '-' : `${formatScaled(props.value, props.scale)REDACTED ${props.unitREDACTED`
)
</script>
