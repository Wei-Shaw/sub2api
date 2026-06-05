<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  label: string
  value: unknown
  format: string
}>()

const formattedValue = computed(() => formatDisplayValue(props.value, props.format))

const badgeClass = computed(() => {
  if (props.value == null) return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
})

function formatDisplayValue(raw: unknown, format: string): string {
  if (raw == null) return '-'
  const num = typeof raw === 'number' ? raw : Number(raw)
  if (Number.isNaN(num)) return String(raw)
  switch (format) {
    case 'currency':
      return '$' + num.toFixed(2)
    case 'percentage':
      return num.toFixed(1) + '%'
    case 'count':
      return String(Math.round(num))
    default:
      return String(raw)
  }
}
</script>

<template>
  <span
    :class="[
      'inline-flex items-center gap-1 rounded-md px-1.5 py-px text-[10px] font-medium leading-tight',
      badgeClass
    ]"
  >
    <span class="font-semibold opacity-70">{{ label }}</span>
    <span class="font-mono">{{ formattedValue }}</span>
  </span>
</template>
