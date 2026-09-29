<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.dashboard.apiKeyDistribution') }}
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
          @click="setMetric(option.value)"
        >
          {{ t(option.label) }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="loadError"
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.failedToLoad') }}
    </div>
    <div
      v-else-if="displayItems.length > 0 && chartData"
      class="flex flex-col items-center gap-4 sm:flex-row sm:gap-6"
    >
      <div class="relative h-48 w-48 shrink-0">
        <Doughnut :data="chartData" :options="doughnutOptions" />
        <!-- 环中心合计（不拦截鼠标，保留分片 tooltip） -->
        <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ centerLabel }}</span>
          <span class="text-base font-bold tabular-nums text-gray-900 dark:text-white">{{ centerValue }}</span>
          <span class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('admin.dashboard.apiKeyCount', { count: totalKeys }) }}
          </span>
        </div>
      </div>
      <div class="max-h-48 w-full min-w-0 flex-1 overflow-auto">
        <div
          v-for="(item, index) in displayItems"
          :key="item.isOther ? 'others' : item.api_key_id"
          data-testid="key-dist-row"
          class="flex items-center gap-2.5 rounded-lg px-2 py-1 transition-colors"
          :class="isRowClickable(item)
            ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40'
            : ''"
          @click="isRowClickable(item) ? emit('key-click', item) : undefined"
        >
          <span class="w-4 shrink-0 text-right text-[11px] tabular-nums text-gray-400 dark:text-gray-500">
            {{ item.isOther ? '' : index + 1 }}
          </span>
          <span
            class="h-2.5 w-2.5 shrink-0 rounded-full"
            :style="{ backgroundColor: rowColor(item, index) }"
          ></span>
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5 text-xs font-medium text-gray-900 dark:text-white">
              <span class="truncate" :title="getRowLabel(item)">{{ getRowLabel(item) }}</span>
              <span
                v-if="item.key_deleted"
                class="shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-normal text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >{{ t('admin.dashboard.apiKeyDeletedBadge') }}</span>
            </span>
            <span
              v-if="getOwnerLabel(item)"
              class="block truncate text-[11px] text-gray-400 dark:text-gray-500"
              :title="getOwnerLabel(item)"
            >
              {{ getOwnerLabel(item) }}
            </span>
          </span>
          <span class="shrink-0 text-xs font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatMetricValue(item) }}
          </span>
          <span class="w-11 shrink-0 text-right text-[11px] tabular-nums text-gray-400 dark:text-gray-500">
            {{ rowPercent(item) }}
          </span>
        </div>
      </div>
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { ApiKeyUsageRankingItem } from '@/types'
import { getApiKeysRanking, type ApiKeyUsageRankingParams } from '@/api/admin/dashboard'
import { getMyApiKeysRanking } from '@/api/usage'

ChartJS.register(ArcElement, Tooltip, Legend)

const { t } = useI18n()

type KeyMetric = 'actual_cost' | 'tokens' | 'requests'
type KeyDisplayItem = ApiKeyUsageRankingItem & { isOther?: boolean }

// scope='admin' 走管理端接口(可选 userId 过滤)；scope='user' 走用户端接口(只看自己的 Key)。
const props = withDefaults(defineProps<{
  startDate?: string
  endDate?: string
  userId?: number
  limit?: number
  scope?: 'admin' | 'user'
}>(), {
  limit: 12,
  scope: 'admin'
})

const emit = defineEmits<{
  'key-click': [item: ApiKeyUsageRankingItem]
}>()

const metricOptions: { value: KeyMetric; label: string }[] = [
  { value: 'actual_cost', label: 'admin.dashboard.metricActualCost' },
  { value: 'tokens', label: 'admin.dashboard.metricTokens' },
  { value: 'requests', label: 'admin.dashboard.metricRequests' }
]

// Top-N 因指标不同而不同(按金额的 Top-N ≠ 按 Token 的 Top-N)，切指标必须重新请求。
const metricSortMap: Record<KeyMetric, NonNullable<ApiKeyUsageRankingParams['sort_by']>> = {
  actual_cost: 'actual_cost',
  tokens: 'total_tokens',
  requests: 'requests'
}

const metric = ref<KeyMetric>('actual_cost')
const items = ref<ApiKeyUsageRankingItem[]>([])
const totalActualCost = ref(0)
const totalRequests = ref(0)
const totalTokens = ref(0)
const totalKeys = ref(0)
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

const setMetric = (value: KeyMetric) => {
  if (metric.value === value) return
  metric.value = value
  load()
}

const load = async () => {
  const seq = ++reqSeq
  loading.value = true
  loadError.value = false
  try {
    const params: ApiKeyUsageRankingParams = {
      start_date: props.startDate,
      end_date: props.endDate,
      limit: props.limit,
      sort_by: metricSortMap[metric.value]
    }
    if (props.scope === 'admin' && props.userId) params.user_id = props.userId
    const res = props.scope === 'user'
      ? await getMyApiKeysRanking(params)
      : await getApiKeysRanking(params)
    if (seq !== reqSeq) return
    items.value = res.ranking || []
    totalActualCost.value = res.total_actual_cost || 0
    totalRequests.value = res.total_requests || 0
    totalTokens.value = res.total_tokens || 0
    totalKeys.value = res.total_keys || 0
  } catch {
    if (seq !== reqSeq) return
    items.value = []
    loadError.value = true
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

watch(
  () => [props.startDate, props.endDate, props.userId],
  () => load(),
  { immediate: true }
)

defineExpose({ reload: load })

const metricValue = (item: KeyDisplayItem): number => {
  if (metric.value === 'actual_cost') return toFiniteNumber(item.actual_cost)
  if (metric.value === 'requests') return toFiniteNumber(item.requests)
  return toFiniteNumber(item.total_tokens)
}

const otherItem = computed<KeyDisplayItem | null>(() => {
  if (!items.value.length) return null

  const rankedActualCost = items.value.reduce((sum, item) => sum + toFiniteNumber(item.actual_cost), 0)
  const rankedRequests = items.value.reduce((sum, item) => sum + toFiniteNumber(item.requests), 0)
  const rankedTokens = items.value.reduce((sum, item) => sum + toFiniteNumber(item.total_tokens), 0)

  const otherActualCost = Math.max(totalActualCost.value - rankedActualCost, 0)
  const otherRequests = Math.max(totalRequests.value - rankedRequests, 0)
  const otherTokens = Math.max(totalTokens.value - rankedTokens, 0)

  if (otherActualCost <= 0.000001 && otherRequests <= 0 && otherTokens <= 0) return null

  return {
    api_key_id: 0,
    key_name: '',
    key_deleted: false,
    user_id: 0,
    email: '',
    username: '',
    requests: otherRequests,
    input_tokens: 0,
    output_tokens: 0,
    cache_tokens: 0,
    total_tokens: otherTokens,
    cost: 0,
    actual_cost: otherActualCost,
    isOther: true
  }
})

const displayItems = computed<KeyDisplayItem[]>(() => {
  if (!items.value.length) return []
  return otherItem.value ? [...items.value, otherItem.value] : [...items.value]
})

const chartData = computed(() => {
  if (!items.value.length) return null

  const labels = items.value.map((item, index) => `#${index + 1} ${getKeyLabel(item)}`)
  const data = items.value.map((item) => metricValue(item))
  const backgroundColor = chartColors.slice(0, items.value.length)

  if (otherItem.value) {
    labels.push(t('admin.dashboard.spendingRankingOther'))
    data.push(metricValue(otherItem.value))
    backgroundColor.push('#94a3b8')
  }

  return {
    labels,
    datasets: [
      {
        data,
        backgroundColor,
        borderWidth: 0
      }
    ]
  }
})

const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      display: false
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const value = context.raw as number
          const total = context.dataset.data.reduce((a: number, b: number) => a + b, 0)
          const percentage = total > 0 ? ((value / total) * 100).toFixed(1) : '0.0'
          const formattedValue = metric.value === 'actual_cost'
            ? `$${formatCost(value)}`
            : metric.value === 'requests'
              ? formatNumber(value)
              : formatTokens(value)
          return `${context.label}: ${formattedValue} (${percentage}%)`
        }
      }
    }
  }
}))

// 环中心合计：随指标切换显示对应的总量
const centerLabel = computed(() => {
  if (metric.value === 'actual_cost') return t('admin.dashboard.totalCost')
  if (metric.value === 'requests') return t('admin.dashboard.totalRequests')
  return t('admin.dashboard.totalTokens')
})

const centerValue = computed(() => {
  if (metric.value === 'actual_cost') return `$${formatCost(totalActualCost.value)}`
  if (metric.value === 'requests') return formatNumber(totalRequests.value)
  return formatTokens(totalTokens.value)
})

const metricTotal = computed(() => {
  if (metric.value === 'actual_cost') return totalActualCost.value
  if (metric.value === 'requests') return totalRequests.value
  return totalTokens.value
})

const formatMetricValue = (item: KeyDisplayItem): string => {
  const value = metricValue(item)
  if (metric.value === 'actual_cost') return `$${formatCost(value)}`
  if (metric.value === 'requests') return formatNumber(value)
  return formatTokens(value)
}

const rowPercent = (item: KeyDisplayItem): string => {
  const total = metricTotal.value
  if (total <= 0) return '0.0%'
  return `${((metricValue(item) / total) * 100).toFixed(1)}%`
}

const rowColor = (item: KeyDisplayItem, index: number): string => {
  if (item.isOther) return '#94a3b8'
  return chartColors[index % chartColors.length]
}

// 用户视角下已删除的 Key 不可下钻：用户端接口按"活跃 Key"校验 api_key_id，
// 下钻已删 Key 会让整页请求 404。
const isRowClickable = (item: KeyDisplayItem): boolean => {
  if (item.isOther) return false
  if (props.scope === 'user' && item.key_deleted) return false
  return true
}

const getKeyLabel = (item: KeyDisplayItem): string => {
  if (item.key_name?.trim()) return item.key_name.trim()
  return t('admin.dashboard.apiKeyPrefix', { id: item.api_key_id })
}

const getRowLabel = (item: KeyDisplayItem): string => {
  if (item.isOther) return t('admin.dashboard.spendingRankingOther')
  return getKeyLabel(item)
}

const getOwnerLabel = (item: KeyDisplayItem): string => {
  if (item.isOther) {
    const unranked = totalKeys.value - items.value.length
    return unranked > 0 ? t('admin.dashboard.apiKeyOtherHint', { count: unranked }) : ''
  }
  // 用户视角全是自己的 Key，归属邮箱是噪音
  if (props.scope === 'user') return ''
  if (item.email?.trim()) return item.email.trim()
  if (item.username?.trim()) return item.username.trim()
  return ''
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

const formatNumber = (value: number): string => {
  return toFiniteNumber(value).toLocaleString()
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
