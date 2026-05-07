<template>
  <div v-if="hasUsageFallback" class="space-y-1">
    <UsageProgressBar
      v-if="usageInfo?.five_hour"
      label="5h"
      :utilization="usageInfo.five_hour.utilization"
      :resets-at="usageInfo.five_hour.resets_at"
      :window-stats="usageInfo.five_hour.window_stats"
      :show-now-when-idle="true"
      color="indigo"
    />
    <UsageProgressBar
      v-if="usageInfo?.seven_day"
      label="7d"
      :utilization="usageInfo.seven_day.utilization"
      :resets-at="usageInfo.seven_day.resets_at"
      :window-stats="usageInfo.seven_day.window_stats"
      :show-now-when-idle="true"
      color="emerald"
    />
  </div>
  <div v-else-if="loading" class="space-y-1.5">
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
  </div>
  <div v-else class="text-xs text-gray-400">-</div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Account, AccountUsageInfo } from '@/types'
import UsageProgressBar from '../UsageProgressBar.vue'

const props = defineProps<{
  account: Account
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
}>()

const hasUsageFallback = computed(() => {
  return !!props.usageInfo?.five_hour || !!props.usageInfo?.seven_day
})
</script>
