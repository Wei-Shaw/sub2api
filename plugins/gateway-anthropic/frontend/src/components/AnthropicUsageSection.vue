<template>
  <!-- Loading state -->
  <div v-if="loading" class="space-y-1.5">
    <!-- OAuth: 3 rows, Setup Token: 1 row -->
    <div class="flex items-center gap-1">
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>
    <template v-if="account.type === 'oauth'">
      <div class="flex items-center gap-1">
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
      <div class="flex items-center gap-1">
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
    </template>
  </div>

  <!-- Error state -->
  <div v-else-if="error" class="text-semantic-danger text-xs">
    {{ error }}
  </div>

  <!-- Usage data -->
  <div v-else-if="usageInfo" class="space-y-1">
    <!-- API error (degraded response) -->
    <div v-if="usageInfo.error" class="text-semantic-warning text-xs truncate max-w-[200px]" :title="usageInfo.error">
      {{ usageInfo.error }}
    </div>
    <!-- 5h Window -->
    <UsageProgressBar
      v-if="usageInfo.five_hour"
      label="5h"
      :utilization="usageInfo.five_hour.utilization"
      :resets-at="usageInfo.five_hour.resets_at"
      :window-stats="usageInfo.five_hour.window_stats"
      color="indigo"
    />

    <!-- 7d Window (OAuth only) -->
    <UsageProgressBar
      v-if="usageInfo.seven_day"
      label="7d"
      :utilization="usageInfo.seven_day.utilization"
      :resets-at="usageInfo.seven_day.resets_at"
      color="emerald"
    />

    <!-- 7d Sonnet Window (OAuth only) -->
    <UsageProgressBar
      v-if="usageInfo.seven_day_sonnet"
      label="7d S"
      :utilization="usageInfo.seven_day_sonnet.utilization"
      :resets-at="usageInfo.seven_day_sonnet.resets_at"
      color="purple"
    />

    <!-- Passive sampling label + active query button -->
    <div class="flex items-center gap-1.5 mt-0.5">
      <span
        v-if="usageInfo.source === 'passive'"
        class="text-[9px] text-gray-400 dark:text-gray-500 italic"
      >
        {{ t('admin.accounts.usageWindow.passiveSampled') }}
      </span>
      <button
        type="button"
        class="btn-ghost-info inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[9px] font-medium transition-colors"
        :disabled="activeQueryLoading"
        @click="$emit('active-query')"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': activeQueryLoading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>
    </div>
  </div>

  <!-- No data yet -->
  <div v-else class="text-xs text-gray-400">-</div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { UsageProgressBar } from '@sub2api/plugin-sdk'

/** Minimal usage progress shape for display. */
interface UsageProgress {
  utilization: number
  resets_at: string | null
  remaining_seconds: number
  window_stats?: { requests: number; tokens: number; cost: number; standard_cost?: number; user_cost?: number } | null
}

/** Minimal account usage info shape. */
interface AccountUsageInfo {
  source?: 'passive' | 'active'
  five_hour: UsageProgress | null
  seven_day: UsageProgress | null
  seven_day_sonnet: UsageProgress | null
  error?: string
}

defineProps<{
  account: { type: string; [k: string]: unknown }
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
  activeQueryLoading: boolean
}>()

defineEmits<{
  'active-query': []
}>()

const { t } = useI18n()
</script>
