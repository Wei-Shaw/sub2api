<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dashboard.recentUsage') }} (Top 12)</h3>
    <div class="h-64">
      <div v-if="loading" class="flex h-full items-center justify-center"><LoadingSpinner size="md" /></div>
      <Line v-else-if="chartData" :data="chartData" :options="options" />
      <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.noDataAvailable') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler } from 'chart.js'
import type { ChartData, ChartOptions } from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { UserUsageTrendPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const props = withDefaults(defineProps<{ items: UserUsageTrendPoint[]; loading?: boolean }>(), { loading: false })
const { t } = useI18n()
const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16', '#06b6d4', '#a855f7']
const formatTokens = (value: number) => value >= 1_000_000_000 ? `${(value / 1_000_000_000).toFixed(2)}B` : value >= 1_000_000 ? `${(value / 1_000_000).toFixed(2)}M` : value >= 1_000 ? `${(value / 1_000).toFixed(2)}K` : value.toLocaleString()

const chartData = computed<ChartData<'line'> | null>(() => {
  if (!props.items.length) return null
  const groups = new Map<number, { name: string; data: Map<string, number> }>()
  const dates = new Set<string>()
  for (const point of props.items) {
    dates.add(point.date)
    if (!groups.has(point.user_id)) groups.set(point.user_id, { name: point.username?.trim() || point.email?.trim() || `#${point.user_id}`, data: new Map() })
    groups.get(point.user_id)?.data.set(point.date, point.tokens)
  }
  const labels = [...dates].sort()
  return {
    labels,
    datasets: [...groups.values()].map((group, index) => ({ label: group.name, data: labels.map(date => group.data.get(date) || 0), borderColor: colors[index % colors.length], backgroundColor: `${colors[index % colors.length]}20`, tension: 0.3 })),
  }
})

const options = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' },
  plugins: { legend: { position: 'top', labels: { usePointStyle: true, pointStyle: 'circle', padding: 15, font: { size: 11 } } } },
  scales: { y: { ticks: { callback: value => formatTokens(Number(value)) } } },
}))
</script>
