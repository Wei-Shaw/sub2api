<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-6">
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
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30 text-amber-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg></div>
      <div>
        <p class="text-xs font-medium text-gray-500">{{ t('usage.totalTokens') }}</p>
        <p class="text-xl font-bold">{{ formatTokens(stats?.total_tokens || 0) }}</p>
        <p class="flex flex-wrap items-center gap-x-1 text-xs text-gray-500">
          <span>{{ t('usage.in') }}: {{ formatTokens(stats?.total_input_tokens || 0) }}</span>
          <span>/</span>
          <span>{{ t('usage.out') }}: {{ formatTokens(stats?.total_output_tokens || 0) }}</span>
          <span>/</span>
          <span class="group relative inline-flex cursor-help items-center gap-0.5" tabindex="0">
            <span>{{ cacheLabel() }}: {{ formatTokens(stats?.total_cache_tokens || 0) }}</span>
            <svg
              class="h-3.5 w-3.5 text-gray-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span
              class="pointer-events-none absolute left-1/2 top-full z-30 mt-2 hidden w-56 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs text-gray-700 shadow-lg group-hover:block group-focus:block dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
            >
              <span class="mb-2 block font-medium text-gray-900 dark:text-white">
                {{ cacheDetailLabel() }}
              </span>
              <span class="flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheCreationTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_creation_tokens || 0) }}
                </span>
              </span>
              <span class="mt-1 flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheReadTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_read_tokens || 0) }}
                </span>
              </span>
            </span>
          </span>
        </p>
      </div>
    </div>
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
    <!-- 总成本：紧跟「总消费」之后。总消费是收进来的钱，总成本是付给上游的钱，
         并排才看得出赚不赚；被 token/耗时卡片隔开的话就得来回找了。
         v-if：只给管理员看（2026-11-06 主人要求）。 -->
    <div v-if="showCostProfit" class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-sky-100 p-2 dark:bg-sky-900/30 text-sky-600">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.totalUpstreamCost') }}</p>
        <p class="text-xl font-bold text-sky-600 dark:text-sky-400">
          ${{ (stats?.total_upstream_cost || 0).toFixed(4) }}
        </p>
        <!-- 「已知成本」而不是「全部成本」：还在反查中的记录不计入 SUM。
             不把这点写出来，主人会拿一个偏小的成本当成真实成本。 -->
        <p class="text-xs text-gray-400">
          <template v-if="missingCostCount > 0">
            <span class="text-amber-500">{{ t('admin.usage.pendingCostCount', { count: missingCostCount }) }}</span>
          </template>
          <template v-else>{{ t('admin.usage.costFullyFetched') }}</template>
        </p>
      </div>
    </div>
    <!-- 总盈利 = 总消费 − 总成本。正绿负红，与明细行的盈亏列同一套语义。
         v-if：同「总成本」，只给管理员看。 -->
    <div v-if="showCostProfit" class="card p-4 flex items-center gap-3">
      <div
        class="rounded-lg p-2"
        :class="totalProfit >= 0 ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-600' : 'bg-red-100 dark:bg-red-900/30 text-red-600'"
      >
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-gray-500">{{ t('admin.usage.totalProfit') }}</p>
        <p
          class="text-xl font-bold tabular-nums"
          :class="totalProfit >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
        >
          {{ formatSignedUSD(totalProfit) }}
        </p>
        <p class="text-xs text-gray-400">
          <template v-if="totalProfitMargin != null">
            {{ t('admin.usage.profitMargin') }} {{ formatSignedPercent(totalProfitMargin) }}
          </template>
          <template v-else>—</template>
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

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
  /**
   * 是否显示「总成本」「总盈利」两张卡。
   *
   * 2026-11-06 主人要求：这两张卡是**经营数据**（我们付给上游多少钱、赚了多少），
   * 只给管理员看。普通用户的使用记录页也复用了这个组件，所以必须显式关掉，
   * 否则用户能直接看到我们的成本结构。
   */
  showCostProfit?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
  showCostProfit: true,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)
const showCostProfit = computed(() => props.showCostProfit)

const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

const formatTokens = (value: number) => {
  if (value >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (value >= 1e6) return (value / 1e6).toFixed(2) + 'M'
  if (value >= 1e3) return (value / 1e3).toFixed(2) + 'K'
  return value.toLocaleString()
}

const cacheLabel = () => t('usage.cacheTotal')
const cacheDetailLabel = () => t('usage.cacheBreakdown')

/** 尚未取到成本的记录数。>0 时卡片上要挂提示：此时毛利是偏乐观的下界。 */
const missingCostCount = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { upstream_cost_missing?: number }) | null
  return stats?.upstream_cost_missing ?? 0
})

/**
 * 总盈利优先用后端算好的 total_profit；后端没给（老版本接口）时才前端兜底相减。
 *
 * 不直接在前端算是有原因的：同一个数字要出现在顶部卡片、趋势图、明细行三处，
 * 各算各的迟早漂移成三个不一样的数，那是最难查的一类问题。
 */
const totalProfit = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_profit?: number }) | null
  if (stats?.total_profit != null) return stats.total_profit
  return (stats?.total_actual_cost || 0) - (stats?.total_upstream_cost || 0)
})

/** 总利润率；收入为 0 时无意义，返回 null 让调用方显示「—」。 */
const totalProfitMargin = computed(() => {
  const revenue = (props.stats as AdminUsageStatsResponse | null)?.total_actual_cost || 0
  if (revenue === 0) return null
  return (totalProfit.value / revenue) * 100
})

function formatSignedUSD(value: number): string {
  return `${value < 0 ? '-' : '+'}$${Math.abs(value).toFixed(4)}`
}

function formatSignedPercent(value: number): string {
  return `${value < 0 ? '-' : '+'}${Math.abs(value).toFixed(2)}%`
}
</script>
