<template>
  <div class="flex items-center gap-1.5">
    <span
      :class="[
        'inline-block h-2 w-2 rounded-full',
        variantClass
      ]"
    ></span>
    <span class="text-sm text-gray-700 dark:text-gray-300">
      {{ label }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
}>()

const variantClass = computed(() => {
  switch (props.status) {
    case 'error':
    case 'danger':
      return 'bg-red-500'
    case 'active':
    case 'success':
      return 'bg-gray-900 dark:bg-gray-100'
    case 'disabled':
    case 'inactive':
    case 'warning':
      return 'bg-gray-400 dark:bg-gray-500'
    default:
      return 'bg-gray-300 dark:bg-gray-600'
  }
})
</script>
