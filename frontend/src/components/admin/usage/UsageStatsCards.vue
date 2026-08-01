<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30 text-blue-600">
        <Icon name="document" size="md" />
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500">{{ t('usage.totalRequests') }}</p>
        <p class="text-xl font-bold">{{ stats?.total_requests?.toLocaleString() || '0' }}</p>
        <p class="text-xs text-gray-400">{{ t('usage.inSelectedRange') }}</p>
      </div>
    </div>
    <UsageTokenSummaryCard
      :input-tokens="stats?.total_input_tokens || 0"
      :output-tokens="stats?.total_output_tokens || 0"
      :cache-creation-tokens="stats?.total_cache_creation_tokens || 0"
      :cache-read-tokens="stats?.total_cache_read_tokens || 0"
      :total-tokens="stats?.total_tokens || 0"
    />
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30 text-green-600">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-gray-500">{{ t('usage.totalCost') }}</p>
        <p class="text-xl font-bold text-green-600">
          ${{ (stats?.total_actual_cost || 0).toFixed(4) }}
        </p>
        <p class="text-xs text-gray-400">
          <template v-if="showAccountCost && totalAccountCost != null">
            <span class="text-orange-500">{{ t('usage.accountCost') }} ${{ totalAccountCost.toFixed(4) }}</span>
            <span> · </span>
          </template>
          <span>
            {{ t('usage.standardCost') }}
            <span :class="{ 'line-through': strikeStandardCost }">${{ (stats?.total_cost || 0).toFixed(4) }}</span>
          </span>
        </p>
      </div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30 text-purple-600">
        <Icon name="clock" size="md" />
      </div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.avgDuration') }}</p><p class="text-xl font-bold">{{ formatDuration(stats?.average_duration_ms || 0) }}</p></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import UsageTokenSummaryCard from '@/components/usage/UsageTokenSummaryCard.vue'

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

</script>
