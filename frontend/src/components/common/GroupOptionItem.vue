<template>
  <div class="flex min-w-0 flex-1 items-start justify-between gap-3">
    <!-- Left: name + description -->
    <div
      class="flex min-w-0 flex-1 flex-col items-start"
      :title="description || undefined"
    >
      <GroupBadge
        :name="name"
        :platform="platform"
        :subscription-type="subscriptionType"
        :show-rate="false"
        class="groupOptionItemBadge"
      />
      <span
        v-if="description"
        class="mt-1.5 w-full text-left text-xs leading-relaxed text-gray-500 dark:text-gray-400 line-clamp-2"
      >
        {{ description }}
      </span>
    </div>

    <!-- Right: rate pill + discount pill + cache pill, stacked vertically, right-aligned -->
    <div class="flex shrink-0 flex-col items-end gap-1 pt-0.5">
      <!-- Row 1: rate pill + checkmark -->
      <div class="flex items-center gap-2">
        <span
          v-if="rateMultiplier !== undefined"
          :class="['inline-flex items-center whitespace-nowrap rounded-full px-3 py-1 text-xs font-semibold', ratePillClass]"
        >
          <template v-if="hasCustomRate">
            <span class="mr-1 line-through opacity-50">{{ rateMultiplier }}x</span>
            <span class="font-bold">{{ userRateMultiplier }}x</span>
          </template>
          <template v-else>
            {{ rateMultiplier }}x 倍率
          </template>
        </span>
        <svg
          v-if="showCheckmark && selected"
          class="h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          stroke-width="2"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
        </svg>
      </div>

      <!-- Row 2: discount pill -->
      <span
        v-if="discountLabel"
        :class="['inline-flex items-center whitespace-nowrap rounded-full px-2 py-0.5 text-[10px] font-medium', discountPillClass]"
      >
        {{ discountLabel }}
      </span>

      <!-- Row 3: cache pill -->
      <span
        v-if="cacheHitRate7d != null"
        class="inline-flex items-center whitespace-nowrap rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300"
      >
        {{ t('common.groupOption.cacheFormat', { pct: Math.round(cacheHitRate7d * 100) }) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import type { SubscriptionType, GroupPlatform } from '@/types'

interface Props {
  name: string
  platform: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null
  description?: string | null
  selected?: boolean
  showCheckmark?: boolean
  /** Global 7-day cache hit ratio (0..1); null/undefined hides the cache pill. */
  cacheHitRate7d?: number | null
}

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  selected: false,
  showCheckmark: true,
  userRateMultiplier: null,
  cacheHitRate7d: null
})

const { t } = useI18n()

// Matches the homepage pricing table formula: CNY = official_usd × multiplier,
// where 1 USD = 7 CNY. So multiplier/7 is the ratio vs official price.
const OFFICIAL_PRICE_DIVISOR = 7

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)

const hasCustomRate = computed(() => {
  return (
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier
  )
})

const discountLabel = computed<string | null>(() => {
  const r = effectiveRate.value
  if (r === undefined || r <= 0) return null
  if (r >= OFFICIAL_PRICE_DIVISOR) return t('common.groupOption.discountOriginal')
  const value = (r / OFFICIAL_PRICE_DIVISOR * 10).toFixed(1)
  return t('common.groupOption.discountFormat', { value })
})

const discountPillClass = computed(() => {
  const r = effectiveRate.value
  if (r !== undefined && r > 0 && r < OFFICIAL_PRICE_DIVISOR) {
    return 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
})

const ratePillClass = computed(() => {
  switch (props.platform) {
    case 'anthropic':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
    case 'openai':
      return 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400'
    case 'gemini':
      return 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400'
    default:
      return 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-400'
  }
})
</script>

<style scoped>
.groupOptionItemBadge :deep(span.truncate) {
  font-weight: 600;
}
</style>
