<template>
  <div class="card p-4">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.usage.businessTrend') }}
      </h3>
      <!-- 金额 / 调用量切换：两张图的数据量级差几个数量级（美元都在 1e-3 级、
           调用量在 1e3 级），同轴画出来小额曲线会被压成一条贴着 0 的直线，
           看不出任何形状。所以不是「缩放」而是「换一个观察口径」。 -->
      <div class="inline-flex overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
        <button
          v-for="option in METRIC_OPTIONS"
          :key="option"
          type="button"
          class="px-3 py-1 text-xs font-medium transition-colors"
          :class="metric === option
            ? 'bg-blue-600 text-white'
            : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'"
          :aria-pressed="metric === option"
          :data-testid="`business-trend-metric-${option}`"
          @click="metric = option"
        >
          {{ t(`admin.usage.metric.${option}`) }}
        </button>
      </div>
    </div>
    <div v-if="loading" class="flex h-56 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="trendData.length > 0 && chartData" class="h-56">
      <Line :data="chartData" :options="lineOptions" />
    </div>
    <div
      v-else
      class="flex h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { TrendDataPoint } from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
}>()

/** 只看两种口径就够了：钱和量。加更多会让这个切换器变成一个需要解释的控件。 */
const METRIC_OPTIONS = ['amount', 'calls'] as const
type Metric = (typeof METRIC_OPTIONS)[number]

const metric = ref<Metric>('amount')

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  revenue: '#10b981',
  cost: '#f97316',
  profit: '#3b82f6',
  requests: '#9ca3af'
}))

/**
 * 金额口径下的三条线：下游收入、上游实扣、已对账毛利。
 *
 * 三者刻意同轴（都是美元原值、不做汇率换算），所以图上任意一点都能直接读出
 * 「收入 − 成本 = 毛利」，不需要换算也不需要看第二个刻度。
 *
 * 「记录总数」始终以浅色柱画在右侧计数轴上：它是分母，脱离它只看金额会误判——
 * 一笔大额调用和一百笔小额调用在金额曲线上可能长得一模一样。
 */
const chartData = computed(() => {
  if (!props.trendData?.length) return null

  const labels = props.trendData.map((d) => d.date)
  // chart.js 的混合图（line + bar）在类型上要求每个 dataset 单独收窄，写起来会
  // 变成一长串泛型体操；这里用一个宽松数组换取可读性，运行时行为完全一致。
  const datasets: any[] = []

  if (metric.value === 'amount') {
    datasets.push(
      {
        type: 'line' as const,
        label: t('admin.usage.seriesRevenue'),
        data: props.trendData.map((d) => d.actual_cost),
        borderColor: chartColors.value.revenue,
        backgroundColor: `${chartColors.value.revenue}20`,
        fill: false,
        tension: 0.3,
        yAxisID: 'yAmount'
      },
      {
        type: 'line' as const,
        label: t('admin.usage.seriesUpstreamCost'),
        data: props.trendData.map((d) => d.upstream_cost),
        borderColor: chartColors.value.cost,
        backgroundColor: `${chartColors.value.cost}20`,
        fill: false,
        tension: 0.3,
        yAxisID: 'yAmount'
      },
      {
        type: 'line' as const,
        label: t('admin.usage.seriesProfit'),
        data: props.trendData.map((d) => d.profit),
        borderColor: chartColors.value.profit,
        backgroundColor: `${chartColors.value.profit}20`,
        fill: false,
        tension: 0.3,
        yAxisID: 'yAmount'
      }
    )
  }

  datasets.push({
    type: 'bar' as const,
    label: t('admin.usage.seriesRequests'),
    data: props.trendData.map((d) => d.requests),
    backgroundColor: metric.value === 'calls' ? `${chartColors.value.profit}b0` : `${chartColors.value.requests}40`,
    borderColor: metric.value === 'calls' ? chartColors.value.profit : chartColors.value.requests,
    borderWidth: 1,
    // 金额口径下柱只是背景参照，用右侧计数轴；调用量口径下它才是主角，独占左轴。
    yAxisID: metric.value === 'calls' ? 'yCalls' : 'yRequests',
    order: 10
  })

  return { labels, datasets }
})

const formatMoney = (value: number): string => {
  const abs = Math.abs(value)
  if (abs >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (abs >= 1) return value.toFixed(2)
  if (abs >= 0.01) return value.toFixed(3)
  return value.toFixed(5)
}

const formatCount = (value: number): string => {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(2) + 'M'
  if (value >= 1_000) return (value / 1_000).toFixed(2) + 'K'
  return value.toLocaleString()
}

const lineOptions = computed(() => {
  const amountVisible = metric.value === 'amount'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' as const },
    plugins: {
      legend: {
        position: 'top' as const,
        labels: {
          color: chartColors.value.text,
          usePointStyle: true,
          pointStyle: 'circle',
          padding: 15,
          font: { size: 11 }
        }
      },
      tooltip: {
        callbacks: {
          label: (context: any) => {
            const raw = Number(context.raw)
            if (context.dataset.yAxisID === 'yAmount') {
              // 带符号：毛利为负时要一眼看出是亏的，而不是靠颜色猜。
              const sign = context.dataset.label === t('admin.usage.seriesProfit') && raw < 0 ? '-' : ''
              return `${context.dataset.label}: ${sign}$${formatMoney(Math.abs(raw))}`
            }
            return `${context.dataset.label}: ${formatCount(raw)}`
          }
        }
      }
    },
    scales: {
      x: {
        grid: { color: chartColors.value.grid },
        ticks: { color: chartColors.value.text, font: { size: 10 } }
      },
      yAmount: {
        display: amountVisible,
        position: 'left' as const,
        grid: { color: chartColors.value.grid },
        ticks: {
          color: chartColors.value.text,
          font: { size: 10 },
          callback: (value: string | number) => `$${formatMoney(Number(value))}`
        }
      },
      yCalls: {
        display: !amountVisible,
        position: 'left' as const,
        beginAtZero: true,
        grid: { color: chartColors.value.grid },
        ticks: {
          color: chartColors.value.text,
          font: { size: 10 },
          callback: (value: string | number) => formatCount(Number(value))
        }
      },
      yRequests: {
        display: amountVisible,
        position: 'right' as const,
        beginAtZero: true,
        grid: { drawOnChartArea: false },
        ticks: {
          color: chartColors.value.requests,
          font: { size: 10 },
          callback: (value: string | number) => formatCount(Number(value))
        }
      }
    }
  }
})
</script>
