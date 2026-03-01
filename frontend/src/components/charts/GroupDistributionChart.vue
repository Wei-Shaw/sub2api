<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.groupDistribution') REDACTEDREDACTED
    </h3>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="groupStats.length > 0 && chartData" class="flex items-center gap-6">
      <div class="h-48 w-48">
        <Doughnut :data="chartData" :options="doughnutOptions" />
      </div>
      <div class="max-h-48 flex-1 overflow-y-auto">
        <table class="w-full text-xs">
          <thead>
            <tr class="text-gray-500 dark:text-gray-400">
              <th class="pb-2 text-left">{{ t('admin.dashboard.group') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.requests') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.tokens') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.actual') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.standard') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="group in groupStats"
              :key="group.group_id"
              class="border-t border-gray-100 dark:border-gray-700"
            >
              <td
                class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 dark:text-white"
                :title="group.group_name || String(group.group_id)"
              >
                {{ group.group_name || t('admin.dashboard.noGroup') REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatNumber(group.requests) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatTokens(group.total_tokens) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-green-600 dark:text-green-400">
                ${{ formatCost(group.actual_cost) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
                ${{ formatCost(group.cost) REDACTEDREDACTED
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend REDACTED from 'chart.js'
import { Doughnut REDACTED from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { GroupStat REDACTED from '@/types'

ChartJS.register(ArcElement, Tooltip, Legend)

const { t REDACTED = useI18n()

const props = defineProps<{
  groupStats: GroupStat[]
  loading?: boolean
REDACTED>()

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
  '#84cc16'
]

const chartData = computed(() => {
  if (!props.groupStats?.length) return null

  return {
    labels: props.groupStats.map((g) => g.group_name || String(g.group_id)),
    datasets: [
      {
        data: props.groupStats.map((g) => g.total_tokens),
        backgroundColor: chartColors.slice(0, props.groupStats.length),
        borderWidth: 0
      REDACTED
    ]
  REDACTED
REDACTED)

const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      display: false
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const value = context.raw as number
          const total = context.dataset.data.reduce((a: number, b: number) => a + b, 0)
          const percentage = ((value / total) * 100).toFixed(1)
          return `${context.labelREDACTED: ${formatTokens(value)REDACTED (${percentageREDACTED%)`
        REDACTED
      REDACTED
    REDACTED
  REDACTED
REDACTED))

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)REDACTEDB`
  REDACTED else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

const formatNumber = (value: number): string => {
  return value.toLocaleString()
REDACTED

const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  REDACTED else if (value >= 1) {
    return value.toFixed(2)
  REDACTED else if (value >= 0.01) {
    return value.toFixed(3)
  REDACTED
  return value.toFixed(4)
REDACTED
</script>
