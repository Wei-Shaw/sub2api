<template>
  <div class="flex items-center gap-1.5">
    <span
      :class="[
        'inline-block h-2 w-2 rounded-full',
        variantClass,
      ]"
    ></span>
    <span class="text-sm text-gray-700 dark:text-gray-300">
      {{ label }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type Variant = 'success' | 'warning' | 'danger' | 'info' | 'gray'

const props = defineProps<{
  status: string
  label: string
  // 显式 variant 优先于 status 推断, 给 plugin 留出语义覆盖空间
  variant?: Variant
}>()

const VARIANT_CLASS: Record<Variant, string> = {
  success: 'bg-green-500',
  warning: 'bg-yellow-500',
  danger: 'bg-red-500',
  info: 'bg-blue-500',
  gray: 'bg-gray-400',
}

function variantFromStatus(status: string): Variant {
  switch (status) {
    case 'active':
    case 'success':
      return 'success'
    case 'disabled':
    case 'inactive':
    case 'warning':
      return 'warning'
    case 'error':
    case 'danger':
      return 'danger'
    case 'info':
      return 'info'
    default:
      return 'gray'
  }
}

const variantClass = computed(() => {
  const variant = props.variant ?? variantFromStatus(props.status)
  return VARIANT_CLASS[variant]
})
</script>
