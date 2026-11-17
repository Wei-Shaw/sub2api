<template>
  <time :datetime="value" :title="usageTime(value, true)" class="whitespace-nowrap text-sm tabular-nums text-gray-600 dark:text-gray-400">{{ usageTime(value) }}</time>
</template>
<script setup lang="ts">
import { usageTime } from './timeDisplay'
defineProps<{ value: string }>()
</script>
