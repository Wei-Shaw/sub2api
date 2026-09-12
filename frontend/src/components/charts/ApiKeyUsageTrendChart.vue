<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.dashboard.apiKeyTrendTitle', { count: limit }) }}
      </h3>
      <div
        class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800"
      >
        <button
          v-for="option in metricOptions"
          :key="option.value"
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="metric === option.value
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="metric = option.value"
        >
          {{ t(option.label) }}
        </button>
      </div>
    </div>

    <div class="h-48">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner />
      </div>
      <div
        v-else-if="loadError"
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.dashboard.failedToLoad') }}
      </div>
      <Line v-else-if="chartData" :data="chartData" :options="lineOptions" />
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.dashboard.noDataAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { ApiKeyUsageTrendPoint } from '@/types'
import { getApiKeyUsageTrend } from '@/api/admin/dashboard'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const { t } = useI18n()

type TrendMetric = 'tokens' | 'requests' | 'actual_cost'

const props = withDefaults(defineProps<{
  startDate?: string
  endDate?: string
  granularity?: 'day' | 'hour'
  limit?: number
  userId?: number
}>(), {
  granularity: 'day',
  limit: 5
})

const metricOptions: { value: TrendMetric; label: string }[] = [
  { value: 'actual_cost', label: 'admin.dashboard.metricActualCost' },
  { value: 'tokens', label: 'admin.dashboard.metricTokens' },
  { value: 'requests', label: 'admin.dashboard.metricRequests' }
]

const metric = ref<TrendMetric>('tokens')
const trend = ref<ApiKeyUsageTrendPoint[]>([])
const loading = ref(false)
const loadError = ref(false)
let reqSeq = 0

const chartColors = [
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

const load = async () => {
  const seq = ++reqSeq
  loading.value = true
  loadError.value = false
  try {
    const res = await getApiKeyUsageTrend({
      start_date: props.startDate,
      end_date: props.endDate,
      granularity: props.granularity,
      limit: props.limit,
      ...(props.userId ? { user_id: props.userId } : {})
    })
    if (seq !== reqSeq) return
    trend.value = res.trend || []
  } catch {
    if (seq !== reqSeq) return
    trend.value = []
    loadError.value = true
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

watch(
  () => [props.startDate, props.endDate, props.granularity, props.userId],
  () => load(),
  { immediate: true }
)

defineExpose({ reload: load })

const metricValue = (point: ApiKeyUsageTrendPoint): number => {
  if (metric.value === 'requests') return toFiniteNumber(point.requests)
  if (metric.value === 'actual_cost') return toFiniteNumber(point.actual_cost)
  return toFiniteNumber(point.tokens)
}

const chartData = computed(() => {
  if (!trend.value.length) return null

  // 按 api_key_id 分组避免同名 Key 串线
  const keyGroups = new Map<number, { name: string; data: Map<string, number> }>()
  const allDates = new Set<string>()

  trend.value.forEach((point) => {
    allDates.add(point.date)
    if (!keyGroups.has(point.api_key_id)) {
      const name = point.key_name?.trim() || t('admin.dashboard.apiKeyPrefix', { id: point.api_key_id })
      keyGroups.set(point.api_key_id, { name, data: new Map() })
    }
    keyGroups.get(point.api_key_id)!.data.set(point.date, metricValue(point))
  })

  const sortedDates = Array.from(allDates).sort()
  const datasets = Array.from(keyGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: chartColors[idx % chartColors.length],
    backgroundColor: `${chartColors[idx % chartColors.length]}20`,
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

const axisColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

const formatMetric = (value: number): string => {
  if (metric.value === 'actual_cost') return `$${formatCost(value)}`
  if (metric.value === 'requests') return toFiniteNumber(value).toLocaleString()
  return formatTokens(value)
}

const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'bottom' as const,
      labels: {
        color: axisColors.value.text,
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
          return `${context.dataset.label}: ${formatMetric(context.raw)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: axisColors.value.grid
      },
      ticks: {
        color: axisColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      grid: {
        color: axisColors.value.grid
      },
      ticks: {
        color: axisColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatMetric(Number(value))
      }
    }
  }
}))

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

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
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
</script>
