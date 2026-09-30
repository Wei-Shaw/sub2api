<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- 读数：上期 → 本期 → 今日用量（本期 = 累计，上期 = 累计 − 今日） -->
        <div class="meter">
          <StatCard
            v-for="reading in tokenReadings"
            :key="reading.key"
            :label="reading.label"
            :value="formatTokens(reading.tokens)"
            :current="reading.current"
          >
            <template #sub>
              <span class="font-semibold" :title="t('admin.dashboard.actual')">${{ formatCost(reading.actual) }}</span>
              <span class="text-fg-subtle">/</span>
              <span :title="t('admin.dashboard.accountCost')">${{ formatCost(reading.account) }}</span>
              <span class="text-fg-subtle">/</span>
              <span :title="t('admin.dashboard.standard')">${{ formatCost(reading.standard) }}</span>
            </template>
          </StatCard>
        </div>

        <!-- 其余指标：印刷登记表 -->
        <section class="card">
          <dl class="grid grid-cols-1 px-5 text-body md:grid-cols-2 md:gap-x-10">
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.apiKeys') }}</dt>
              <dd class="text-right tabular-nums text-fg">
                {{ formatNumber(stats.total_api_keys) }}
                <span class="text-meta text-fg-muted">· {{ formatNumber(stats.active_api_keys) }} {{ t('common.active') }}</span>
              </dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.accounts') }}</dt>
              <dd class="text-right tabular-nums text-fg">
                {{ formatNumber(stats.total_accounts) }}
                <span class="text-meta text-fg-muted">· {{ formatNumber(stats.normal_accounts) }} {{ t('common.active') }}</span>
                <span v-if="stats.error_accounts > 0" class="text-meta text-danger">
                  · {{ formatNumber(stats.error_accounts) }} {{ t('common.error') }}
                </span>
              </dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.users') }}</dt>
              <dd class="text-right tabular-nums text-fg">
                {{ formatNumber(stats.total_users) }}
                <span class="text-meta text-success">+{{ formatNumber(stats.today_new_users) }}</span>
              </dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.activeUsers') }}</dt>
              <dd class="text-right tabular-nums text-fg">{{ formatNumber(stats.active_users) }}</dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.todayRequests') }}</dt>
              <dd class="text-right tabular-nums text-fg">{{ formatNumber(stats.today_requests) }}</dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.totalRequests') }}</dt>
              <dd class="text-right tabular-nums text-fg">{{ formatNumber(stats.total_requests) }}</dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 border-b border-border py-2 md:border-b-0">
              <dt class="text-fg-muted">{{ t('admin.dashboard.performance') }}</dt>
              <dd class="text-right tabular-nums text-fg">
                {{ formatTokens(stats.rpm) }} <span class="text-meta text-fg-muted">RPM</span>
                · {{ formatTokens(stats.tpm) }} <span class="text-meta text-fg-muted">TPM</span>
              </dd>
            </div>
            <div class="flex items-baseline justify-between gap-4 py-2">
              <dt class="text-fg-muted">{{ t('admin.dashboard.avgResponse') }}</dt>
              <dd class="text-right tabular-nums text-fg">{{ formatDuration(stats.average_duration_ms) }}</dd>
            </div>
          </dl>
        </section>

        <!-- Quick Actions -->
        <section class="card">
          <div class="card-header">
            <h2 class="card-title">
              {{ t('admin.dashboard.quickActions') }}
            </h2>
          </div>
          <div class="grid grid-cols-1 divide-y divide-border md:grid-cols-2 md:divide-x md:divide-y-0">
            <button
              v-for="action in quickActions"
              :key="action.to"
              type="button"
              class="group flex min-h-[40px] items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-accent-weak focus:outline-none focus-visible:bg-accent-weak"
              @click="router.push(action.to)"
            >
              <Icon :name="action.icon" size="sm" class="shrink-0 text-accent" aria-hidden="true" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-body font-semibold text-fg group-hover:text-accent-strong">
                  {{ t(action.title) }}
                </span>
                <span class="block truncate text-meta text-fg-muted">
                  {{ t(action.desc) }}
                </span>
              </span>
              <Icon name="chevronRight" size="sm" class="shrink-0 text-fg-subtle group-hover:text-accent-strong" aria-hidden="true" />
            </button>
          </div>
        </section>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="flex flex-wrap items-center gap-3 border-y border-border bg-surface px-4 py-2">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-label font-medium text-fg-muted">{{ t('admin.dashboard.timeRange') }}</span>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>
            <button type="button" @click="loadDashboardStats" :disabled="chartsLoading" class="btn btn-secondary btn-sm">
              {{ t('common.refresh') }}
            </button>
            <div class="flex items-center gap-2 sm:ml-auto">
              <span class="text-label font-medium text-fg-muted">{{ t('admin.dashboard.granularity') }}</span>
              <div class="w-28">
                <Select
                  v-model="granularity"
                  :options="granularityOptions"
                  @change="loadChartData"
                />
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
          <section class="card">
            <div class="card-header flex items-center justify-between gap-3">
              <h2 class="card-title">
                {{ t('admin.dashboard.recentUsage') }}
              </h2>
              <div class="flex items-center gap-3">
                <span class="text-meta font-medium tabular-nums text-fg-muted">Top {{ rankingLimit }}</span>
                <div class="flex gap-1" role="group" :aria-label="t('admin.dashboard.recentUsage')">
                  <button v-for="metric in (['tokens', 'actual_cost'] as const)" :key="metric" type="button"
                    class="rounded-sm px-2 py-1 text-meta font-medium transition-colors"
                    :class="userTrendMetric === metric ? 'bg-accent text-white dark:text-surface-sunken' : 'text-fg-muted hover:bg-accent-weak hover:text-accent-strong'"
                    :aria-pressed="userTrendMetric === metric" @click="setUserTrendMetric(metric)">
                    {{ t(metric === 'tokens' ? 'admin.dashboard.tokens' : 'admin.dashboard.actualSpending') }}
                  </button>
                </div>
              </div>
            </div>
            <div class="card-body">
              <div class="h-64">
                <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                  <LoadingSpinner size="md" />
                </div>
                <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
                <div v-else class="flex h-full items-center justify-center text-body text-fg-muted">
                  {{ t('admin.dashboard.noDataAvailable') }}
                </div>
              </div>
            </div>
          </section>
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
import StatCard from '@/components/common/StatCard.vue'
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
import { useChartTheme, withAlpha } from '@/components/charts/chartTheme'

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

type IconName = InstanceType<typeof Icon>['$props']['name']
const quickActions = computed<{ to: string; icon: IconName; title: string; desc: string }[]>(() => [
  ...(canUseBatchImage.value
    ? [{ to: '/batch-image', icon: 'sparkles' as IconName, title: 'admin.dashboard.batchImage', desc: 'admin.dashboard.batchImageDesc' }]
    : []),
  { to: '/admin/groups', icon: 'grid', title: 'admin.dashboard.groupPricing', desc: 'admin.dashboard.groupPricingDesc' }
])

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') }
])

// Chart colors (bill palette, follows .dark)
const chartTheme = useChartTheme()
const chartColors = computed(() => ({
  text: chartTheme.value.fgMuted,
  grid: chartTheme.value.border
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
      ...chartTheme.value.tooltip,
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
  const theme = chartTheme.value

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: theme.seriesColor(idx),
    backgroundColor: withAlpha(theme.seriesColor(idx), 0.12),
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

// 读数条：上期（累计 − 今日）→ 本期（累计，当前读数）→ 今日用量
const tokenReadings = computed(() => {
  const s = stats.value
  if (!s) return []
  const n = toFiniteNumber
  const before = (total: unknown, today: unknown) => Math.max(n(total) - n(today), 0)
  return [
    {
      key: 'previous',
      label: t('admin.dashboard.beforeToday'),
      current: false,
      tokens: before(s.total_tokens, s.today_tokens),
      actual: before(s.total_actual_cost, s.today_actual_cost),
      account: before(s.total_account_cost, s.today_account_cost),
      standard: before(s.total_cost, s.today_cost)
    },
    {
      key: 'current',
      label: t('admin.dashboard.totalTokens'),
      current: true,
      tokens: n(s.total_tokens),
      actual: n(s.total_actual_cost),
      account: n(s.total_account_cost),
      standard: n(s.total_cost)
    },
    {
      key: 'today',
      label: t('admin.dashboard.todayTokens'),
      current: false,
      tokens: n(s.today_tokens),
      actual: n(s.today_actual_cost),
      account: n(s.today_account_cost),
      standard: n(s.today_cost)
    }
  ]
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
