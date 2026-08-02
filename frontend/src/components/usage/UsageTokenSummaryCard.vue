<template>
  <div class="card flex items-center gap-3 p-4" data-testid="usage-token-summary">
    <div class="rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30">
      <Icon name="cube" size="md" />
    </div>
    <div>
      <p class="text-xs font-medium text-gray-500">{{ t('usage.totalTokens') }}</p>
      <p class="text-xl font-bold">{{ formatTokens(totalTokens) }}</p>
      <p class="flex flex-wrap items-center gap-x-1 text-xs text-gray-500">
        <span>{{ t('usage.in') }}: {{ formatTokens(inputTokens) }}</span>
        <span>/</span>
        <span>{{ t('usage.out') }}: {{ formatTokens(outputTokens) }}</span>
        <span>/</span>
        <span class="group relative inline-flex cursor-help items-center gap-0.5" tabindex="0" data-testid="usage-cache-detail-trigger">
          <span>{{ t('usage.cacheTotal') }}: {{ formatTokens(cacheTokens) }}</span>
          <Icon name="infoCircle" size="xs" class="text-gray-400" />
          <span
            class="pointer-events-none absolute left-1/2 top-full z-30 mt-2 w-56 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs text-gray-700 opacity-0 shadow-lg transition-opacity duration-150 group-hover:opacity-100 group-focus:opacity-100 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
            data-testid="usage-cache-detail"
          >
            <span class="mb-2 block font-medium text-gray-900 dark:text-white">{{ t('usage.cacheBreakdown') }}</span>
            <span class="flex items-center justify-between gap-3">
              <span>{{ t('usage.cacheCreationTokensLabel') }}</span>
              <span class="tabular-nums">{{ formatTokens(cacheCreationTokens) }}</span>
            </span>
            <span class="mt-1 flex items-center justify-between gap-3">
              <span>{{ t('usage.cacheReadTokensLabel') }}</span>
              <span class="tabular-nums">{{ formatTokens(cacheReadTokens) }}</span>
            </span>
          </span>
        </span>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  inputTokens: number
  outputTokens: number
  cacheCreationTokens: number
  cacheReadTokens: number
  totalTokens: number
}>()

const { t } = useI18n()
const cacheTokens = computed(() => props.cacheCreationTokens + props.cacheReadTokens)

function formatTokens(value: number) {
  if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`
  if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`
  if (value >= 1e3) return `${(value / 1e3).toFixed(2)}K`
  return value.toLocaleString()
}
</script>
