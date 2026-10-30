<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Core Stats -->
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-2xl border border-gray-100 bg-gray-100 shadow-card dark:border-dark-700/50 dark:bg-dark-700/50 lg:grid-cols-4">
          <!-- Total API Keys -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.apiKeys') }}
              </p>
              <Icon name="key" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ stats.total_api_keys }}
            </p>
            <p class="mt-1 text-xs tabular-nums text-green-600 dark:text-green-400">
              {{ stats.active_api_keys }} {{ t('common.active') }}
            </p>
          </div>
          <!-- Service Accounts -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.accounts') }}
              </p>
              <Icon name="server" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ stats.total_accounts }}
            </p>
            <p class="mt-1 text-xs tabular-nums">
              <span class="text-green-600 dark:text-green-400"
                >{{ stats.normal_accounts }} {{ t('common.active') }}</span
              >
              <span v-if="stats.error_accounts > 0" class="ml-1 text-red-500"
                >{{ stats.error_accounts }} {{ t('common.error') }}</span
              >
            </p>
          </div>
          <!-- Today Requests -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.todayRequests') }}
              </p>
              <Icon name="chart" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ stats.today_requests }}
            </p>
            <p class="mt-1 text-xs tabular-nums text-gray-500 dark:text-dark-400">
              {{ t('common.total') }}: {{ formatNumber(stats.total_requests) }}
            </p>
          </div>
          <!-- New Users Today -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.users') }}
              </p>
              <Icon name="userPlus" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              +{{ stats.today_new_users }}
            </p>
            <p class="mt-1 text-xs tabular-nums text-gray-500 dark:text-dark-400">
              {{ t('common.total') }}: {{ formatNumber(stats.total_users) }}
            </p>
          </div>
        </div>

        <!-- Row 2: Token Stats -->
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-2xl border border-gray-100 bg-gray-100 shadow-card dark:border-dark-700/50 dark:bg-dark-700/50 lg:grid-cols-4">
          <!-- Today Tokens -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.todayTokens') }}
              </p>
              <Icon name="cube" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ formatTokens(stats.today_tokens) }}
            </p>
            <p class="mt-1 text-xs tabular-nums">
              <span
                class="text-green-600 dark:text-green-400"
                :title="t('admin.dashboard.actual')"
                >${{ formatCost(stats.today_actual_cost) }}</span
              >
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span
                class="text-orange-500 dark:text-orange-400"
                :title="t('admin.dashboard.accountCost')"
                >${{ formatCost(stats.today_account_cost) }}</span
              >
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span
                class="text-gray-400 dark:text-dark-500"
                :title="t('admin.dashboard.standard')"
                >${{ formatCost(stats.today_cost) }}</span
              >
            </p>
          </div>
          <!-- Total Tokens -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.totalTokens') }}
              </p>
              <Icon name="database" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ formatTokens(stats.total_tokens) }}
            </p>
            <p class="mt-1 text-xs tabular-nums">
              <span
                class="text-green-600 dark:text-green-400"
                :title="t('admin.dashboard.actual')"
                >${{ formatCost(stats.total_actual_cost) }}</span
              >
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span
                class="text-orange-500 dark:text-orange-400"
                :title="t('admin.dashboard.accountCost')"
                >${{ formatCost(stats.total_account_cost) }}</span
              >
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span
                class="text-gray-400 dark:text-dark-500"
                :title="t('admin.dashboard.standard')"
                >${{ formatCost(stats.total_cost) }}</span
              >
            </p>
          </div>
          <!-- Performance (RPM/TPM) -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.performance') }}
              </p>
              <Icon name="bolt" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 flex items-baseline gap-1.5">
              <span class="text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
                {{ formatTokens(stats.rpm) }}
              </span>
              <span class="text-xs text-gray-500 dark:text-dark-400">RPM</span>
            </p>
            <p class="mt-1 flex items-baseline gap-1.5 text-xs">
              <span class="font-medium tabular-nums text-gray-700 dark:text-dark-200">{{ formatTokens(stats.tpm) }}</span>
              <span class="text-gray-500 dark:text-dark-400">TPM</span>
            </p>
          </div>
          <!-- Avg Response Time -->
          <div class="min-w-0 bg-white p-4 dark:bg-dark-900">
            <div class="flex items-center justify-between gap-2">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.dashboard.avgResponse') }}
              </p>
              <Icon name="clock" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            </div>
            <p class="mt-2 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 dark:text-white">
              {{ formatDuration(stats.average_duration_ms) }}
            </p>
            <p class="mt-1 text-xs tabular-nums text-gray-500 dark:text-dark-400">
              {{ stats.active_users }} {{ t('admin.dashboard.activeUsers') }}
            </p>
          </div>
        </div>

        <!-- Quick Actions -->
        <div class="card p-4">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.dashboard.quickActions') }}
            </h2>
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <button
              v-if="canUseBatchImage"
              type="button"
              class="group flex items-center gap-3 rounded-lg border border-gray-200/80 p-3 text-left transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-800/60"
              @click="router.push('/batch-image')"
            >
              <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-400">
                <Icon name="sparkles" size="md" />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.dashboard.batchImage') }}
                </span>
                <span class="block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.batchImageDesc') }}
                </span>
              </span>
              <Icon name="chevronRight" size="sm" class="text-gray-300 transition-colors group-hover:text-gray-500 dark:text-dark-500 dark:group-hover:text-dark-300" />
            </button>
            <button
              type="button"
              class="group flex items-center gap-3 rounded-lg border border-gray-200/80 p-3 text-left transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-800/60"
              @click="router.push('/admin/groups')"
            >
              <span class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-400">
                <Icon name="grid" size="md" />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.dashboard.groupPricing') }}
                </span>
                <span class="block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.groupPricingDesc') }}
                </span>
              </span>
              <Icon name="chevronRight" size="sm" class="text-gray-300 transition-colors group-hover:text-gray-500 dark:text-dark-500 dark:group-hover:text-dark-300" />
            </button>
          </div>
        </div>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="border-t border-gray-200/80 pt-6 dark:border-dark-800">
            <div class="flex flex-wrap items-center gap-3">
              <div class="flex items-center gap-2">
                <span class="text-sm text-gray-500 dark:text-dark-400"
                  >{{ t('admin.dashboard.timeRange') }}:</span
                >
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <button @click="loadDashboardStats" :disabled="chartsLoading" class="btn btn-secondary">
                {{ t('common.refresh') }}
              </button>
              <div class="ml-auto flex items-center gap-2">
                <span class="text-sm text-gray-500 dark:text-dark-400"
                  >{{ t('admin.dashboard.granularity') }}:</span
                >
                <div class="w-28">
                  <Select
                    v-model="granularity"
                    :options="granularityOptions"
                    @change="loadChartData"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- Charts Grid -->
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <ModelDistributionChart
              :model-stats="modelStats"
              :enable-ranking-view="true"
              :ranking-items="rankingItems"
              :ranking-total-actual-cost="rankingTotalActualCost"
              :ranking-total-requests="rankingTotalRequests"
              :ranking-total-tokens="rankingTotalTokens"
              :loading="chartsLoading"
              :ranking-loading="rankingLoading"
              :ranking-error="rankingError"
              :start-date="startDate"
              :end-date="endDate"
              @ranking-click="goToUserUsage"
            />
            <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
          </div>

          <!-- User Usage Trend (Full Width) -->
          <div class="card p-4">
            <div class="mb-4 flex items-center justify-between gap-3">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dashboard.recentUsage') }} (Top 12)</h3>
              <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800" role="group" :aria-label="t('admin.dashboard.recentUsage')">
                <button v-for="metric in (['tokens', 'actual_cost'] as const)" :key="metric" type="button"
                  class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors" :class="userTrendMetric === metric ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
                  :aria-pressed="userTrendMetric === metric" @click="setUserTrendMetric(metric)">
                  {{ t(metric === 'tokens' ? 'admin.dashboard.tokens' : 'admin.dashboard.actualSpending') }}
                </button>
              </div>
            </div>
            <div class="h-64">
              <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                <LoadingSpinner size="md" />
              </div>
              <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div
                v-else
                class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.dashboard.noDataAvailable') }}
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
import { adminAPI } from '@/api/admin'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  UserUsageTrendPoint,
  UserSpendingRankingItem
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
)

const appStore = useAppStore()
const router = useRouter()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const chartsLoading = ref(false)
const userTrendLoading = ref(false)
const rankingLoading = ref(false)
const rankingError = ref(false)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const userTrend = ref<UserUsageTrendPoint[]>([])
const userTrendMetric = ref<'tokens' | 'actual_cost'>('tokens')
const rankingItems = ref<UserSpendingRankingItem[]>([])
const rankingTotalActualCost = ref(0)
const rankingTotalRequests = ref(0)
const rankingTotalTokens = ref(0)
let chartLoadSeq = 0
let usersTrendLoadSeq = 0
let rankingLoadSeq = 0
const rankingLimit = 12

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

const getLast24HoursRangeDates = (): { start: string; end: string } => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return {
    start: formatLocalDate(start),
    end: formatLocalDate(end)
  }
}

// Date range
const granularity = ref<'day' | 'hour'>('hour')
const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') }
])

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

// Line chart options (for user trend chart)
const lineOptions = computed(() => ({
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
      itemSort: (a: any, b: any) => {
        const aValue = typeof a?.raw === 'number' ? a.raw : Number(a?.parsed?.y ?? 0)
        const bValue = typeof b?.raw === 'number' ? b.raw : Number(b?.parsed?.y ?? 0)
        return bValue - aValue
      },
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.label}: ${formatUserTrendValue(Number(context.raw))}`
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
        }
      }
    },
    y: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatUserTrendValue(Number(value))
      }
    }
  }
}))

// User trend chart data
const userTrendChartData = computed(() => {
  if (!userTrend.value?.length) return null

  const getDisplayName = (point: UserUsageTrendPoint): string => {
    const username = point.username?.trim()
    if (username) {
      return username
    }

    const email = point.email?.trim()
    if (email) {
      return email
    }

    return t('admin.redeem.userPrefix', { id: point.user_id })
  }

  // Group by user_id to avoid merging different users with the same display name
  const userGroups = new Map<number, { name: string; data: Map<string, number> }>()
  const allDates = new Set<string>()

  userTrend.value.forEach((point) => {
    allDates.add(point.date)
    const key = point.user_id
    if (!userGroups.has(key)) {
      userGroups.set(key, { name: getDisplayName(point), data: new Map() })
    }
    userGroups.get(key)!.data.set(point.date, userTrendMetric.value === 'tokens' ? point.tokens : point.actual_cost)
  })

  const sortedDates = Array.from(allDates).sort()
  const colors = [
    '#3b82f6',
    '#10b981',
    '#f59e0b',
    '#ef4444',
    '#8b5cf6',
    '#ec4899',
    '#14b8a6',
    '#f97316',
    '#6366f1',
    '#84cc16',
    '#06b6d4',
    '#a855f7'
  ]

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]}20`,
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

// Format helpers
const formatUserTrendValue = (value: number): string =>
  userTrendMetric.value === 'tokens' ? formatTokens(value) : `$${formatCost(value)}`

const formatTokens = (value: number | undefined): string => {
  if (value === undefined || value === null) return '0'
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

const formatNumber = (value: number | null | undefined): string => {
  return toFiniteNumber(value).toLocaleString()
}

const formatCost = (value: number | null | undefined): string => {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1000) {
    return (safeValue / 1000).toFixed(2) + 'K'
  } else if (safeValue >= 1) {
    return safeValue.toFixed(2)
  } else if (safeValue >= 0.01) {
    return safeValue.toFixed(3)
  }
  return safeValue.toFixed(4)
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}

const goToUserUsage = (item: UserSpendingRankingItem) => {
  void router.push({
    path: '/admin/usage',
    query: {
      user_id: String(item.user_id),
      start_date: startDate.value,
      end_date: endDate.value
    }
  })
}

// Date range change handler
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  // Auto-select granularity based on date range
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  // If range is 1 day, use hourly granularity
  if (daysDiff <= 1) {
    granularity.value = 'hour'
  } else {
    granularity.value = 'day'
  }

  loadChartData()
}

// Load data
const loadDashboardSnapshot = async (includeStats: boolean) => {
  const currentSeq = ++chartLoadSeq
  if (includeStats && !stats.value) {
    loading.value = true
  }
  chartsLoading.value = true
  try {
    const response = await adminAPI.dashboard.getSnapshotV2({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      include_stats: includeStats,
      include_trend: true,
      include_model_stats: true,
      include_group_stats: false,
      include_users_trend: false
    })
    if (currentSeq !== chartLoadSeq) return
    if (includeStats && response.stats) {
      stats.value = response.stats
    }
    trendData.value = response.trend || []
    modelStats.value = response.models || []
  } catch (error) {
    if (currentSeq !== chartLoadSeq) return
    appStore.showError(t('admin.dashboard.failedToLoad'))
    console.error('Error loading dashboard snapshot:', error)
  } finally {
    if (currentSeq === chartLoadSeq) {
      loading.value = false
      chartsLoading.value = false
    }
  }
}

const setUserTrendMetric = (metric: 'tokens' | 'actual_cost') => {
  if (userTrendMetric.value === metric) return
  userTrendMetric.value = metric
  userTrend.value = []
  loadUsersTrend()
}

const loadUsersTrend = async () => {
  const currentSeq = ++usersTrendLoadSeq
  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      limit: 12,
      metric: userTrendMetric.value
    })
    if (currentSeq !== usersTrendLoadSeq) return
    userTrend.value = response.trend || []
  } catch (error) {
    if (currentSeq !== usersTrendLoadSeq) return
    console.error('Error loading users trend:', error)
    userTrend.value = []
  } finally {
    if (currentSeq === usersTrendLoadSeq) {
      userTrendLoading.value = false
    }
  }
}

const loadUserSpendingRanking = async () => {
  const currentSeq = ++rankingLoadSeq
  rankingLoading.value = true
  rankingError.value = false
  try {
    const response = await adminAPI.dashboard.getUserSpendingRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: rankingLimit
    })
    if (currentSeq !== rankingLoadSeq) return
    rankingItems.value = response.ranking || []
    rankingTotalActualCost.value = response.total_actual_cost || 0
    rankingTotalRequests.value = response.total_requests || 0
    rankingTotalTokens.value = response.total_tokens || 0
  } catch (error) {
    if (currentSeq !== rankingLoadSeq) return
    console.error('Error loading user spending ranking:', error)
    rankingItems.value = []
    rankingTotalActualCost.value = 0
    rankingTotalRequests.value = 0
    rankingTotalTokens.value = 0
    rankingError.value = true
  } finally {
    if (currentSeq === rankingLoadSeq) {
      rankingLoading.value = false
    }
  }
}

const loadDashboardStats = async () => {
  await Promise.all([
    loadDashboardSnapshot(true),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

const loadChartData = async () => {
  await Promise.all([
    loadDashboardSnapshot(false),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

onMounted(() => {
  void refreshBatchImageAccess()
  loadDashboardStats()
})
</script>

<style scoped>
</style>
