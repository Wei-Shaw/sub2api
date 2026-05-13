<template>
  <span v-if="version" class="text-xs text-gray-500 dark:text-dark-400">
    v{{ version }}
  </span>
</template>

<script setup lang="ts">
defineProps<{
  version?: string
}>()
</script>
