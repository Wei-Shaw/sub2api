<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Core Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total API Keys -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
                <Icon name="key" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.apiKeys') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ stats.total_api_keys }}
                </p>
                <p class="text-xs text-green-600 dark:text-green-400">
                  {{ stats.active_api_keys }} {{ t('common.active') }}
                </p>
              </div>
            </div>
          </div>

          <!-- Service Accounts -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
                <Icon name="server" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.accounts') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ stats.total_accounts }}
                </p>
                <p class="text-xs">
                  <span class="text-green-600 dark:text-green-400"
                    >{{ stats.normal_accounts }} {{ t('common.active') }}</span
                  >
                  <span v-if="stats.error_accounts > 0" class="ml-1 text-red-500"
                    >{{ stats.error_accounts }} {{ t('common.error') }}</span
                  >
                </p>
              </div>
            </div>
          </div>

          <!-- Today Requests -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
                <Icon name="chart" size="md" class="text-green-600 dark:text-green-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.todayRequests') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ stats.today_requests }}
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('common.total') }}: {{ formatNumber(stats.total_requests) }}
                </p>
              </div>
            </div>
          </div>

          <!-- New Users Today -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30">
                <Icon name="userPlus" size="md" class="text-emerald-600 dark:text-emerald-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.users') }}
                </p>
                <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">
                  +{{ stats.today_new_users }}
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('common.total') }}: {{ formatNumber(stats.total_users) }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Row 2: Token Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Today Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
                <Icon name="cube" size="md" class="text-amber-600 dark:text-amber-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.todayTokens') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatTokens(stats.today_tokens) }}
                </p>
                <p class="text-xs">
                  <span
                    class="text-amber-600 dark:text-amber-400"
                    :title="t('admin.dashboard.actual')"
                    >${{ formatCost(stats.today_actual_cost) }}</span
                  >
                  <span
                    class="text-gray-400 dark:text-gray-500"
                    :title="t('admin.dashboard.standard')"
                  >
                    / ${{ formatCost(stats.today_cost) }}</span
                  >
                </p>
              </div>
            </div>
          </div>

          <!-- Total Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-indigo-100 p-2 dark:bg-indigo-900/30">
                <Icon name="database" size="md" class="text-indigo-600 dark:text-indigo-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.totalTokens') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatTokens(stats.total_tokens) }}
                </p>
                <p class="text-xs">
                  <span
                    class="text-indigo-600 dark:text-indigo-400"
                    :title="t('admin.dashboard.actual')"
                    >${{ formatCost(stats.total_actual_cost) }}</span
                  >
                  <span
                    class="text-gray-400 dark:text-gray-500"
                    :title="t('admin.dashboard.standard')"
                  >
                    / ${{ formatCost(stats.total_cost) }}</span
                  >
                </p>
              </div>
            </div>
          </div>

          <!-- Performance (RPM/TPM) -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-violet-100 p-2 dark:bg-violet-900/30">
                <Icon name="bolt" size="md" class="text-violet-600 dark:text-violet-400" :stroke-width="2" />
              </div>
              <div class="flex-1">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.performance') }}
                </p>
                <div class="flex items-baseline gap-2">
                  <p class="text-xl font-bold text-gray-900 dark:text-white">
                    {{ formatTokens(stats.rpm) }}
                  </p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">RPM</span>
                </div>
                <div class="flex items-baseline gap-2">
                  <p class="text-sm font-semibold text-violet-600 dark:text-violet-400">
                    {{ formatTokens(stats.tpm) }}
                  </p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">TPM</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Avg Response Time -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="rounded-lg bg-rose-100 p-2 dark:bg-rose-900/30">
                <Icon name="clock" size="md" class="text-rose-600 dark:text-rose-400" :stroke-width="2" />
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.avgResponse') }}
                </p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">
                  {{ formatDuration(stats.average_duration_ms) }}
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ stats.active_users }} {{ t('admin.dashboard.activeUsers') }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="card p-4">
            <div class="flex flex-wrap items-center gap-4">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300"
                  >{{ t('admin.dashboard.timeRange') }}:</span
                >
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <div class="ml-auto flex items-center gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300"
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
              @ranking-click="goToUserUsage"
            />
            <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
          </div>

          <!-- User Usage Trend (Full Width) -->
          <div class="card p-4">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.dashboard.recentUsage') }} (Top 12)
            </h3>
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

const getTodayLocalDate = () => formatLocalDate(new Date())

// Date range
const granularity = ref<'day' | 'hour'>('hour')
const startDate = ref(getTodayLocalDate())
const endDate = ref(getTodayLocalDate())

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
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
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
        callback: (value: string | number) => formatTokens(Number(value))
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
    userGroups.get(key)!.data.set(point.date, point.tokens)
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

const formatNumber = (value: number): string => {
  return value.toLocaleString()
}

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

const loadUsersTrend = async () => {
  const currentSeq = ++usersTrendLoadSeq
  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      limit: 12
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
  loadDashboardStats()
})
</script>

<style scoped>
</style>
