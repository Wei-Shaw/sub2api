<template>
  <div class="flex items-stretch gap-2">
    <span
      class="w-1 shrink-0 rounded-full"
      :class="firstTokenMs != null
        ? ['bg-gradient-to-b from-40% to-60%', LATENCY_BAR_FROM_CLASSES[firstTokenSeverity(firstTokenMs)], LATENCY_BAR_TO_CLASSES[durationSeverity(durationMs ?? 0)]]
        : LATENCY_BAR_CLASSES[durationSeverity(durationMs ?? 0)]"
      aria-hidden="true"
    ></span>
    <div class="grid grid-cols-[max-content_max-content] items-baseline gap-x-2 gap-y-0.5 text-xs">
      <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyFirstToken') }}</span>
      <span v-if="firstTokenMs != null" class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[firstTokenSeverity(firstTokenMs)]">{{ formatDuration(firstTokenMs) }}</span>
      <span v-else class="text-gray-400 dark:text-gray-500">-</span>
      <span class="text-gray-400 dark:text-gray-500">{{ t('usage.latencyDuration') }}</span>
      <span class="font-medium tabular-nums" :class="LATENCY_TEXT_CLASSES[durationSeverity(durationMs ?? 0)]">{{ formatDuration(durationMs) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  LATENCY_BAR_CLASSES,
  LATENCY_BAR_FROM_CLASSES,
  LATENCY_BAR_TO_CLASSES,
  LATENCY_TEXT_CLASSES,
  durationSeverity,
  firstTokenSeverity,
} from '@/utils/latencyHealth'

defineProps<{ firstTokenMs?: number | null; durationMs?: number | null }>()
const { t } = useI18n()

function formatDuration(value: number | null | undefined): string {
  if (value == null) return '-'
  return value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${Math.round(value)}ms`
}
</script>
