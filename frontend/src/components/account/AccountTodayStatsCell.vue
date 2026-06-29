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
      {{ props.error }}
    </div>

    <!-- Stats data -->
    <div v-else-if="props.stats" class="space-y-0.5 text-xs">
      <!-- Requests -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.requests') }}:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatNumber(props.stats.requests)
        }}</span>
      </div>
      <!-- Tokens -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400"
          >{{ t('admin.accounts.stats.tokens') }}:</span
        >
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatTokens(props.stats.tokens)
        }}</span>
      </div>
      <!-- Cost (Account) -->
      <div class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') }}:</span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{
          formatCurrency(props.stats.cost)
        }}</span>
      </div>
      <!-- Cost (User/API Key) -->
      <div v-if="props.stats.user_cost != null" class="flex items-center gap-1">
        <span class="text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') }}:</span>
        <span class="font-medium text-gray-700 dark:text-gray-300">{{
          formatCurrency(props.stats.user_cost)
        }}</span>
      </div>
      <!-- Kiro Credits -->
      <div
        v-if="showKiroCredits"
        class="flex items-center gap-1"
        :class="kiroCreditLossClass"
        data-testid="kiro-credits-row"
      >
        <span :class="kiroCreditLabelClass">{{ t('admin.accounts.stats.kiroCredits') }}:</span>
        <span class="font-medium" :class="kiroCreditValueClass">
          {{ formatCredits(props.stats.kiro_credits) }}
          <span v-if="kiroCreditEstimatedCost">
            ({{ t('admin.accounts.stats.approxCost', { amount: kiroCreditEstimatedCost }) }})
          </span>
        </span>
      </div>
    </div>

    <!-- No data -->
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatNumber, formatCurrency } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    stats?: WindowStats | null
    platform?: string | null
    kiroCreditUnitPriceUsd?: number | null
    loading?: boolean
    error?: string | null
  }>(),
  {
    stats: null,
    platform: null,
    kiroCreditUnitPriceUsd: null,
    loading: false,
    error: null
  }
)

const { t } = useI18n()

const showKiroCredits = computed(() => props.platform === 'kiro')

const kiroCreditEstimatedCostValue = computed(() => {
  if (!showKiroCredits.value) return null
  const unitPrice = Number(props.kiroCreditUnitPriceUsd ?? 0)
  if (!Number.isFinite(unitPrice) || unitPrice <= 0) return null
  const credits = Number(props.stats?.kiro_credits ?? 0)
  if (!Number.isFinite(credits)) return null
  return credits * unitPrice
})

const kiroCreditEstimatedCost = computed(() => {
  if (kiroCreditEstimatedCostValue.value == null) return ''
  return formatCurrency(kiroCreditEstimatedCostValue.value)
})

const isKiroCreditLoss = computed(() => {
  if (kiroCreditEstimatedCostValue.value == null) return false
  const userCost = Number(props.stats?.user_cost ?? 0)
  if (!Number.isFinite(userCost)) return false
  return kiroCreditEstimatedCostValue.value > userCost
})

const kiroCreditLossClass = computed(() =>
  isKiroCreditLoss.value ? 'text-red-600 dark:text-red-400' : ''
)

const kiroCreditLabelClass = computed(() =>
  isKiroCreditLoss.value ? '' : 'text-gray-500 dark:text-gray-400'
)

const kiroCreditValueClass = computed(() =>
  isKiroCreditLoss.value ? '' : 'text-gray-700 dark:text-gray-300'
)

// Format large token numbers (e.g., 1234567 -> 1.23M)
const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) {
    return `${(tokens / 1000000).toFixed(2)}M`
  } else if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)}K`
  }
  return tokens.toString()
}

const formatCredits = (value?: number | null): string => {
  const credits = Number(value ?? 0)
  if (!Number.isFinite(credits)) return '0'
  return credits.toLocaleString(undefined, {
    maximumFractionDigits: 2
  })
}
</script>
