<template>
  <span
    v-if="fp"
    :class="['inline-flex items-center rounded px-1.5 py-0.5 font-mono text-[11px] ring-1', fingerprintClass(fp), strike ? 'line-through opacity-70' : '']"
    :title="title || fp"
    :data-fp="fp"
    data-testid="fp-chip"
  >{{ fp }}</span>
  <span v-else class="text-gray-400 dark:text-gray-500">—</span>
</template>

<script setup lang="ts">
import { fingerprintClass } from './accountTraceUtils'

defineProps<{ fp?: string | null; title?: string; strike?: boolean }>()
</script>
