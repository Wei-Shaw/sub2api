<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Flat hairline placeholders, sized like the panels they stand in for. -->
      <template v-if="loading">
        <div v-for="n in 2" :key="n" class="rounded border border-line bg-surface p-4">
          <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            <div v-for="cell in 4" :key="cell" class="space-y-2">
              <div class="skeleton h-3 w-20"></div>
              <div class="skeleton h-7 w-28"></div>
              <div class="skeleton h-3 w-24"></div>
            </div>
          </div>
        </div>
      </template>

      <template v-else-if="stats">
        <!--
          Headline numbers, type-led. The 48px pastel icon tile that led every
          one of these cards is gone: in a system with no colour decoration the
          type scale is the hierarchy, and the tile was spending the most
          prominent element in the card on decoration.
        -->
        <Surface v-for="panel in panels" :key="panel.key" :data-testid="panel.testId">
          <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            <div v-for="cell in panel.cells" :key="cell.key" class="min-w-0">
              <Metric
                :label="cell.label"
                :value="cell.value"
                :unit="cell.unit"
                :precision="cell.precision"
                :caption="cell.caption"
              />
              <!-- Secondary quantities. Same mono column, one step down. -->
              <dl v-if="cell.rows?.length" class="mt-2 space-y-0.5">
                <div
                  v-for="row in cell.rows"
                  :key="row.label"
                  class="flex items-baseline justify-between gap-2 text-xs"
                >
                  <dt class="min-w-0 truncate text-2xs text-ink-tertiary" :title="row.title">
                    {{ row.label }}
                  </dt>
                  <dd class="shrink-0">
                    <NumCell
                      :value="row.value"
                      :precision="row.precision"
                      :unit="row.unit"
                      :tone="row.tone"
                    />
                  </dd>
                </div>
              </dl>
            </div>
          </div>
        </Surface>

        <!--
          Quick actions as hairline rows, not tinted tiles. Each row was a
          40px pastel square plus a hue-shifted hover ground — two colours per
          action, neither of which meant anything.
        -->
        <Surface :title="t('admin.dashboard.quickActions')" flush>
          <div class="divide-y divide-line-subtle">
            <button
              v-for="action in quickActions"
              :key="action.key"
              type="button"
              class="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors duration-fast hover:bg-surface-sunken"
              @click="router.push(action.to)"
            >
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium text-ink">{{ action.label }}</span>
                <span class="block text-xs text-ink-tertiary">{{ action.description }}</span>
              </span>
              <span class="shrink-0 font-mono text-xs text-ink-tertiary" aria-hidden="true">→</span>
            </button>
          </div>
        </Surface>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Filter bar. Labels are 2xs caps, the same rank as a table header. -->
          <Surface>
            <div class="flex flex-wrap items-center gap-x-4 gap-y-3">
              <div class="flex items-center gap-2">
                <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('admin.dashboard.timeRange') }}
                </span>
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <Button :loading="chartsLoading" @click="loadDashboardStats">
                {{ t('common.refresh') }}
              </Button>
              <div class="ml-auto flex items-center gap-2">
                <label
                  for="admin-dashboard-granularity"
                  class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
                >
                  {{ t('admin.dashboard.granularity') }}
                </label>
                <div class="w-28">
                  <Select
                    id="admin-dashboard-granularity"
                    v-model="granularity"
                    :options="granularityOptions"
                    @change="loadChartData"
                  />
                </div>
              </div>
            </div>
          </Surface>

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
          <Surface :title="t('admin.dashboard.recentUsage')">
            <template #actions>
              <span class="font-mono text-2xs text-ink-tertiary">Top {{ rankingLimit }}</span>
            </template>
            <div class="h-64">
              <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                <LoadingSpinner size="md" />
              </div>
              <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div v-else class="flex h-full items-center justify-center text-sm text-ink-tertiary">
                {{ t('admin.dashboard.noDataAvailable') }}
              </div>
            </div>
          </Surface>
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
// Primitives are imported by path, never through `components/common/index.ts`:
// the barrel re-exports LocaleSwitcher, which pulls `createI18n` into the module
// graph and breaks any spec that mocks vue-i18n with a partial factory.
import Button from '@/components/common/Button.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import type { Tone } from '@/components/common/primitives'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import { useChartTheme } from '@/components/charts/chartTheme'
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

// Chart colors, from the design tokens.
//
// This used to be `computed(() => document.documentElement.classList.contains('dark'))`
// feeding two hardcoded hex pairs. That computed had no reactive dependency,
// so it cached forever and these charts never re-themed on toggle — they only
// picked up the theme once, at mount.
const chartTheme = useChartTheme()
const chartColors = computed(() => ({
  text: chartTheme.value.axis,
  grid: chartTheme.value.grid
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
  const colors = chartTheme.value.series

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]}20`,
    fill: false
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

/**
 * A missing measurement and a measurement of zero are different facts. The old
 * template ran every field through a formatter that coerced null to `0`, so a
 * stat the backend had not reported yet was indistinguishable from a real zero.
 * `NumCell` and `Metric` render null as an en dash instead.
 */
const numOrNull = (value: unknown): number | null => {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

interface StatRow {
  label: string
  value: number | null
  precision?: number
  unit?: string
  tone?: Tone
  title?: string
}

interface StatCell {
  key: string
  label: string
  value: number | null
  unit?: string
  precision?: number
  caption?: string
  rows?: StatRow[]
}

const CURRENCY = 'USD'

const coreCells = computed<StatCell[]>(() => {
  const s = stats.value
  return [
    {
      key: 'api-keys',
      label: t('admin.dashboard.apiKeys'),
      value: numOrNull(s?.total_api_keys),
      rows: [{ label: t('common.active'), value: numOrNull(s?.active_api_keys) }],
    },
    {
      key: 'accounts',
      label: t('admin.dashboard.accounts'),
      value: numOrNull(s?.total_accounts),
      rows: [
        { label: t('common.active'), value: numOrNull(s?.normal_accounts) },
        {
          label: t('common.error'),
          value: numOrNull(s?.error_accounts),
          // Colour only once the count has actually crossed zero: a permanently
          // red "0 errors" is decoration, and decoration is what makes a real
          // error stop registering.
          tone: (s?.error_accounts ?? 0) > 0 ? 'danger' : 'neutral',
        },
      ],
    },
    {
      key: 'today-requests',
      label: t('admin.dashboard.todayRequests'),
      value: numOrNull(s?.today_requests),
      rows: [{ label: t('common.total'), value: numOrNull(s?.total_requests) }],
    },
    {
      key: 'users',
      label: t('admin.dashboard.users'),
      value: numOrNull(s?.today_new_users),
      rows: [
        { label: t('common.total'), value: numOrNull(s?.total_users) },
        { label: t('admin.dashboard.activeUsers'), value: numOrNull(s?.active_users) },
      ],
    },
  ]
})

/** The three prices of the same traffic, always in the same order. */
const costRows = (actual: unknown, account: unknown, standard: unknown): StatRow[] => [
  {
    label: t('admin.dashboard.actual'),
    value: numOrNull(actual),
    precision: 4,
    unit: CURRENCY,
  },
  { label: t('admin.dashboard.accountCost'), value: numOrNull(account), precision: 4 },
  { label: t('admin.dashboard.standard'), value: numOrNull(standard), precision: 4 },
]

const tokenCells = computed<StatCell[]>(() => {
  const s = stats.value
  return [
    {
      key: 'today-tokens',
      label: t('admin.dashboard.todayTokens'),
      value: numOrNull(s?.today_tokens),
      rows: costRows(s?.today_actual_cost, s?.today_account_cost, s?.today_cost),
    },
    {
      key: 'total-tokens',
      label: t('admin.dashboard.totalTokens'),
      value: numOrNull(s?.total_tokens),
      rows: costRows(s?.total_actual_cost, s?.total_account_cost, s?.total_cost),
    },
    {
      key: 'performance',
      label: t('admin.dashboard.performance'),
      value: numOrNull(s?.rpm),
      unit: 'RPM',
      rows: [{ label: 'TPM', value: numOrNull(s?.tpm) }],
    },
    {
      key: 'avg-response',
      // Kept in milliseconds rather than switching to seconds past 1000: a
      // number that changes unit as it grows cannot be compared at a glance.
      label: t('admin.dashboard.avgResponse'),
      value: numOrNull(s?.average_duration_ms),
      unit: 'ms',
    },
  ]
})

const panels = computed(() => [
  { key: 'core', testId: 'admin-dashboard-core-stats', cells: coreCells.value },
  { key: 'tokens', testId: 'admin-dashboard-token-stats', cells: tokenCells.value },
])

const quickActions = computed(() =>
  [
    canUseBatchImage.value
      ? {
          key: 'batch-image',
          to: '/batch-image',
          label: t('admin.dashboard.batchImage'),
          description: t('admin.dashboard.batchImageDesc'),
        }
      : null,
    {
      key: 'group-pricing',
      to: '/admin/groups',
      label: t('admin.dashboard.groupPricing'),
      description: t('admin.dashboard.groupPricingDesc'),
    },
  ].filter((a): a is { key: string; to: string; label: string; description: string } => a !== null)
)

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
  void refreshBatchImageAccess()
  loadDashboardStats()
})
</script>

<style scoped>
</style>
