<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.usageStatistics')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-6">
      <!--
        Account Info Header. Was an accent gradient band carrying a gradient
        icon tile — two ramps and an accent fill spent on a caption. The
        account name is the heading here and the rule under it is the
        separation.
      -->
      <div v-if="account" class="flex items-center justify-between gap-4 border-b border-line pb-3">
        <div class="min-w-0">
          <div class="truncate font-semibold text-ink">{{ account.name }}</div>
          <div class="text-xs text-ink-tertiary">
            {{ t('admin.accounts.last30DaysUsage') }}
          </div>
        </div>
        <Badge :tone="account.status === 'active' ? 'success' : 'neutral'" caps>
          {{ account.status }}
        </Badge>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!--
          Row 1. Four cards, each with its own coloured border, its own tinted
          top-to-white gradient and its own pastel icon tile — four hues for
          four quantities that are not four categories. One panel, four
          metrics, one column of figures.
        -->
        <Surface>
          <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            <div class="min-w-0">
              <Metric
                :label="t('admin.accounts.stats.totalCost')"
                :value="stats.summary.total_cost"
                :precision="4"
                unit="USD"
                :caption="t('admin.accounts.stats.accumulatedCost')"
              />
              <dl class="mt-2 space-y-0.5">
                <div class="flex items-baseline justify-between gap-2 text-xs">
                  <dt class="truncate text-2xs text-ink-tertiary">{{ t('usage.userBilled') }}</dt>
                  <dd><NumCell :value="stats.summary.total_user_cost" :precision="4" /></dd>
                </div>
                <div class="flex items-baseline justify-between gap-2 text-xs">
                  <dt class="truncate text-2xs text-ink-tertiary">
                    {{ t('admin.accounts.stats.standardCost') }}
                  </dt>
                  <dd><NumCell :value="stats.summary.total_standard_cost" :precision="4" /></dd>
                </div>
              </dl>
            </div>

            <div class="min-w-0">
              <Metric
                :label="t('admin.accounts.stats.totalRequests')"
                :value="stats.summary.total_requests"
                :caption="t('admin.accounts.stats.totalCalls')"
              />
            </div>

            <div class="min-w-0">
              <Metric
                :label="t('admin.accounts.stats.avgDailyCost')"
                :value="stats.summary.avg_daily_cost"
                :precision="4"
                unit="USD"
                :caption="
                  t('admin.accounts.stats.basedOnActualDays', {
                    days: stats.summary.actual_days_used,
                  })
                "
              />
              <dl class="mt-2 space-y-0.5">
                <div class="flex items-baseline justify-between gap-2 text-xs">
                  <dt class="truncate text-2xs text-ink-tertiary">{{ t('usage.userBilled') }}</dt>
                  <dd><NumCell :value="stats.summary.avg_daily_user_cost" :precision="4" /></dd>
                </div>
              </dl>
            </div>

            <div class="min-w-0">
              <Metric
                :label="t('admin.accounts.stats.avgDailyRequests')"
                :value="Math.round(stats.summary.avg_daily_requests)"
                :caption="t('admin.accounts.stats.avgDailyUsage')"
              />
            </div>
          </div>
        </Surface>

        <!-- Row 2: Today, Highest Cost, Highest Requests -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Today Overview -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.todayOverview')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.accountBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.today?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.userBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.today?.user_cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatNumber(stats.summary.today?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.tokens')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                }}</span>
              </div>
            </div>
          </div>

          <!-- Highest Cost Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.highestCostDay')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.date')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  stats.summary.highest_cost_day?.label || '-'
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.accountBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.highest_cost_day?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.userBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.highest_cost_day?.user_cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatNumber(stats.summary.highest_cost_day?.requests || 0)
                }}</span>
              </div>
            </div>
          </div>

          <!-- Highest Request Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.highestRequestDay')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.date')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  stats.summary.highest_request_day?.label || '-'
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatNumber(stats.summary.highest_request_day?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.accountBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.highest_request_day?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{ t('usage.userBilled') }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.highest_request_day?.user_cost || 0) }}</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Row 3: Token Stats -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Accumulated Tokens -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.accumulatedTokens')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.totalTokens')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatTokens(stats.summary.total_tokens)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.dailyAvgTokens')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatTokens(Math.round(stats.summary.avg_daily_tokens))
                }}</span>
              </div>
            </div>
          </div>

          <!-- Performance -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.performance')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.avgResponseTime')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatDuration(stats.summary.avg_duration_ms)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.daysActive')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >{{ stats.summary.actual_days_used }} / {{ stats.summary.days }}</span
                >
              </div>
            </div>
          </div>

          <!-- Recent Activity -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <span class="text-sm font-semibold text-ink">{{
                t('admin.accounts.stats.recentActivity')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.todayRequests')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatNumber(stats.summary.today?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.todayTokens')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-ink-secondary">{{
                  t('admin.accounts.stats.todayCost')
                }}</span>
                <span class="font-mono text-sm tabular-nums text-ink"
                  >${{ formatCost(stats.summary.today?.cost || 0) }}</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Usage Trend Chart -->
        <div class="card p-4">
          <h3 class="mb-4 font-mono text-sm tabular-nums text-ink">
            {{ t('admin.accounts.stats.usageTrend') }}
          </h3>
          <div class="h-64">
            <Line v-if="trendChartData" :data="trendChartData" :options="lineChartOptions" />
            <div
              v-else
              class="flex h-full items-center justify-center text-sm text-ink-secondary"
            >
              {{ t('admin.dashboard.noDataAvailable') }}
            </div>
          </div>
        </div>

        <!-- Model Distribution -->
        <ModelDistributionChart :model-stats="stats.models" :loading="false" />

        <EndpointDistributionChart
          :endpoint-stats="stats.endpoints || []"
          :loading="false"
          :title="t('usage.inboundEndpoint')"
        />

        <EndpointDistributionChart
          :endpoint-stats="stats.upstream_endpoints || []"
          :loading="false"
          :title="t('usage.upstreamEndpoint')"
        />
      </template>

      <!-- No Data State -->
      <div
        v-else-if="!loading"
        class="flex flex-col items-center justify-center py-12 text-ink-secondary"
      >
        <Icon name="chartBar" size="xl" class="mb-4 h-12 w-12" />
        <p class="text-sm">{{ t('admin.accounts.stats.noData') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-ink-secondary transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useTheme } from '@/composables/useTheme'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
// Primitives by path, never through the barrel — it re-exports LocaleSwitcher
// and drags `createI18n` into the graph of every spec that mocks vue-i18n.
import Badge from '@/components/common/Badge.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageStatsResponse } from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const loading = ref(false)
const stats = ref<AccountUsageStatsResponse | null>(null)

// Dark mode detection
// Was a dependency-free `computed`, which caches forever — this chart never
// re-themed on toggle. `useTheme().isDark` is a real ref.
const { isDark: isDarkMode } = useTheme()

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

// Line chart data
const trendChartData = computed(() => {
  if (!stats.value?.history?.length) return null

  return {
    labels: stats.value.history.map((h) => h.label),
    datasets: [
      {
        label: t('usage.accountBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.actual_cost),
        borderColor: '#3b82f6',
        backgroundColor: 'rgba(59, 130, 246, 0.1)',
        fill: true,
        yAxisID: 'y'
      },
      {
        label: t('usage.userBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.user_cost),
        borderColor: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        fill: false,
        borderDash: [5, 5],
        yAxisID: 'y'
      },
      {
        label: t('admin.accounts.stats.requests'),
        data: stats.value.history.map((h) => h.requests),
        borderColor: '#f97316',
        backgroundColor: 'rgba(249, 115, 22, 0.1)',
        fill: false,
        yAxisID: 'y1'
      }
    ]
  }
})

// Line chart options with dual Y-axis
const lineChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const label = context.dataset.label || ''
          const value = context.raw
          if (label.includes('USD')) {
            return `${label}: $${formatCost(value)}`
          }
          return `${label}: ${formatNumber(value)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        maxRotation: 45,
        minRotation: 0
      }
    },
    y: {
      type: 'linear' as const,
      display: true,
      position: 'left' as const,
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: '#3b82f6',
        font: {
          size: 10
        },
        callback: (value: string | number) => '$' + formatCost(Number(value))
      },
      title: {
        display: true,
        text: t('usage.accountBilled') + ' (USD)',
        color: '#3b82f6',
        font: {
          size: 11
        }
      }
    },
    y1: {
      type: 'linear' as const,
      display: true,
      position: 'right' as const,
      grid: {
        drawOnChartArea: false
      },
      ticks: {
        color: '#f97316',
        font: {
          size: 10
        },
        callback: (value: string | number) => formatNumber(Number(value))
      },
      title: {
        display: true,
        text: t('admin.accounts.stats.requests'),
        color: '#f97316',
        font: {
          size: 11
        }
      }
    }
  }
}))

// Load stats when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      await loadStats()
    } else {
      stats.value = null
    }
  }
)

const loadStats = async () => {
  if (!props.account) return

  loading.value = true
  try {
    stats.value = await adminAPI.accounts.getStats(props.account.id, 30)
  } catch (error) {
    console.error('Failed to load account stats:', error)
    stats.value = null
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  emit('close')
}

// Format helpers
const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  } else if (value >= 1) {
    return value.toFixed(2)
  } else if (value >= 0.01) {
    return value.toFixed(3)
  }
  return value.toFixed(4)
}

const formatNumber = (value: number): string => {
  if (value >= 1_000_000) {
    return (value / 1_000_000).toFixed(2) + 'M'
  } else if (value >= 1_000) {
    return (value / 1_000).toFixed(2) + 'K'
  }
  return value.toLocaleString()
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}
</script>
