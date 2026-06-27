<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  labels: string[]
  emptyText?: string
}>(), {
  emptyText: '—',
})

const visibleLabels = computed(() => props.labels.filter((label) => Boolean(label && label.trim())))
</script>

<template>
  <div class="flex flex-wrap gap-1.5">
    <span v-for="label in visibleLabels" :key="label" class="badge badge-success">
      {{ label }}
    </span>
    <span v-if="visibleLabels.length === 0" class="text-xs text-gray-400">
      {{ emptyText }}
    </span>
  </div>
</template>
