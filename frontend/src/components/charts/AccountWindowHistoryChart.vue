<template>
  <div>
    <div v-if="loading" class="flex h-56 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="entries.length > 0 && chartData" class="h-56">
      <Chart type="bar" :data="chartData" :options="chartOptions" />
    </div>
    <div
      v-else
      class="flex h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.accounts.stats.windowHistory.empty') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Chart } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { AccountWindowUsageEntry } from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  PointElement,
  LineElement,
  Tooltip,
  Legend
)

const { t } = useI18n()

const props = defineProps<{
  /** One window type's entries, oldest first */
  entries: AccountWindowUsageEntry[]
  loading?: boolean
}>()

const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  tokens: '#3b82f6',
  peak: '#f59e0b',
  final: '#10b981',
  implied: '#8b5cf6'
}))

const wh = 'admin.accounts.stats.windowHistory'

const formatWindowLabel = (iso: string): string => {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
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

// 推算限额：由「窗口 token ÷ 最终使用率」反推。最终使用率过低（<5%）时
// 反推误差过大，跳过形成断点。
const impliedLimitOf = (entry: AccountWindowUsageEntry): number | null => {
  if (!entry.final_used_percent || entry.final_used_percent < 5) return null
  if (!entry.tokens_total) return null
  return entry.tokens_total / (entry.final_used_percent / 100)
}

const chartData = computed(() => {
  if (!props.entries?.length) return null

  return {
    labels: props.entries.map((e) => formatWindowLabel(e.window_end)),
    datasets: [
      {
        type: 'bar' as const,
        label: t(`${wh}.chartTokens`),
        data: props.entries.map((e) => e.tokens_total),
        backgroundColor: `${chartColors.value.tokens}99`,
        borderColor: chartColors.value.tokens,
        borderWidth: 1,
        yAxisID: 'yTokens'
      },
      {
        type: 'line' as const,
        label: t(`${wh}.chartPeak`),
        data: props.entries.map((e) => (e.peak_used_percent > 0 ? e.peak_used_percent : null)),
        borderColor: chartColors.value.peak,
        backgroundColor: chartColors.value.peak,
        tension: 0.2,
        pointRadius: 2,
        yAxisID: 'yPercent'
      },
      {
        type: 'line' as const,
        label: t(`${wh}.chartFinal`),
        data: props.entries.map((e) => e.final_used_percent),
        borderColor: chartColors.value.final,
        backgroundColor: chartColors.value.final,
        borderDash: [5, 5],
        tension: 0.2,
        pointRadius: 2,
        spanGaps: false,
        yAxisID: 'yPercent'
      },
      {
        type: 'line' as const,
        label: t(`${wh}.chartImplied`),
        data: props.entries.map((e) => impliedLimitOf(e)),
        borderColor: chartColors.value.implied,
        backgroundColor: chartColors.value.implied,
        borderDash: [2, 3],
        tension: 0,
        pointRadius: 2,
        spanGaps: false,
        yAxisID: 'yTokens'
      }
    ]
  }
})

const chartOptions = computed(() => ({
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
        padding: 12,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const label: string = context.dataset.label || ''
          const raw = context.raw
          if (raw == null) return
          if (label === t(`${wh}.chartPeak`) || label === t(`${wh}.chartFinal`)) {
            return `${label}: ${Number(raw).toFixed(1)}%`
          }
          return `${label}: ${formatTokens(Number(raw))}`
        }
      }
    }
  },
  scales: {
    x: {
      stacked: false,
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
    yTokens: {
      type: 'linear' as const,
      position: 'left' as const,
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.tokens,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    },
    yPercent: {
      type: 'linear' as const,
      position: 'right' as const,
      beginAtZero: true,
      grid: {
        drawOnChartArea: false
      },
      ticks: {
        color: chartColors.value.peak,
        font: {
          size: 10
        },
        callback: (value: string | number) => `${value}%`
      }
    }
  }
}))
</script>
