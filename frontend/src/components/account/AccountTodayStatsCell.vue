<template>
  <div>
    <!-- Loading state -->
    <div v-if="props.loading && !props.stats" class="space-y-0.5">
      <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <!-- Error state -->
    <div v-else-if="props.error && !props.stats" class="text-xs text-red-500">
      {{ props.error REDACTEDREDACTED
    </div>

    <!-- Stats data -->
    <div v-else-if="props.stats" class="space-y-0.5 text-xs">
      <!-- Requests -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.requests') REDACTEDREDACTED:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatNumber(props.stats.requests)
        REDACTEDREDACTED</span>
      </div>
      <!-- Tokens -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.tokens') REDACTEDREDACTED:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatTokens(props.stats.tokens)
        REDACTEDREDACTED</span>
      </div>
      <!-- Cost (Account) -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') REDACTEDREDACTED:</span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{
          formatCurrency(props.stats.cost)
        REDACTEDREDACTED</span>
      </div>
      <!-- Cost (User/API Key) -->
      <div v-if="props.stats.user_cost != null" class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') REDACTEDREDACTED:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatCurrency(props.stats.user_cost)
        REDACTEDREDACTED</span>
      </div>
    </div>

    <!-- No data -->
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import type { WindowStats REDACTED from '@/types'
import { formatNumber, formatCurrency REDACTED from '@/utils/format'

const props = withDefaults(
  defineProps<{
    stats?: WindowStats | null
    loading?: boolean
    error?: string | null
  REDACTED>(),
  {
    stats: null,
    loading: false,
    error: null
  REDACTED
)

const { t REDACTED = useI18n()

// Format large token numbers (e.g., 1234567 -> 1.23M)
const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) {
    return `${(tokens / 1000000).toFixed(2)REDACTEDM`
  REDACTED else if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)REDACTEDK`
  REDACTED
  return tokens.toString()
REDACTED
</script>
