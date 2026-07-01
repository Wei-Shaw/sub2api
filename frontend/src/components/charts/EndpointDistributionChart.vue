<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ title || t('usage.endpointDistribution') REDACTEDREDACTED
      </h3>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <div
          v-if="showSourceToggle"
          class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-dark-800"
        >
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="source === 'inbound'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:source', 'inbound')"
          >
            {{ t('usage.inbound') REDACTEDREDACTED
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="source === 'upstream'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:source', 'upstream')"
          >
            {{ t('usage.upstream') REDACTEDREDACTED
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="source === 'path'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:source', 'path')"
          >
            {{ t('usage.path') REDACTEDREDACTED
          </button>
        </div>

        <div
          v-if="showMetricToggle"
          class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-dark-800"
        >
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="metric === 'tokens'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:metric', 'tokens')"
          >
            {{ t('admin.dashboard.metricTokens') REDACTEDREDACTED
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="metric === 'actual_cost'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:metric', 'actual_cost')"
          >
            {{ t('admin.dashboard.metricActualCost') REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="displayEndpointStats.length > 0 && chartData" class="flex items-center gap-6">
      <div class="h-48 w-48">
        <Doughnut :data="chartData" :options="doughnutOptions" />
      </div>
      <div class="max-h-48 flex-1 overflow-y-auto">
        <table class="w-full text-xs">
          <thead>
            <tr class="text-gray-500 dark:text-gray-400">
              <th class="pb-2 text-left">{{ t('usage.endpoint') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.requests') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.tokens') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.actual') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.standard') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="item in displayEndpointStats" :key="item.endpoint">
              <tr
                class="border-t border-gray-100 transition-colors dark:border-gray-700"
                :class="enableBreakdown ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40' : ''"
                @click="enableBreakdown && toggleBreakdown(item.endpoint)"
              >
                <td class="max-w-[180px] truncate py-1.5 font-medium" :class="enableBreakdown ? 'text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300' : 'text-gray-900 dark:text-white'" :title="item.endpoint">
                  <span class="inline-flex items-center gap-1">
                    <svg v-if="enableBreakdown && expandedKey === item.endpoint" class="h-3 w-3 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
                    <svg v-else-if="enableBreakdown" class="h-3 w-3 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
                    {{ item.endpoint REDACTEDREDACTED
                  </span>
                </td>
                <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                  {{ formatNumber(item.requests) REDACTEDREDACTED
                </td>
                <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                  {{ formatTokens(item.total_tokens) REDACTEDREDACTED
                </td>
                <td class="py-1.5 text-right text-green-600 dark:text-green-400">
                  ${{ formatCost(item.actual_cost) REDACTEDREDACTED
                </td>
                <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
                  ${{ formatCost(item.cost) REDACTEDREDACTED
                </td>
              </tr>
              <tr v-if="expandedKey === item.endpoint">
                <td colspan="5" class="p-0">
                  <UserBreakdownSubTable
                    :items="breakdownItems"
                    :loading="breakdownLoading"
                  />
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
    <div v-else class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend REDACTED from 'chart.js'
import { Doughnut REDACTED from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserBreakdownSubTable from './UserBreakdownSubTable.vue'
import type { EndpointStat, UserBreakdownItem REDACTED from '@/types'
import { getUserBreakdown REDACTED from '@/api/admin/dashboard'

ChartJS.register(ArcElement, Tooltip, Legend)

const { t REDACTED = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'
type EndpointSource = 'inbound' | 'upstream' | 'path'

const props = withDefaults(
  defineProps<{
    endpointStats: EndpointStat[]
    upstreamEndpointStats?: EndpointStat[]
    endpointPathStats?: EndpointStat[]
    loading?: boolean
    title?: string
    metric?: DistributionMetric
    source?: EndpointSource
    showMetricToggle?: boolean
    showSourceToggle?: boolean
    enableBreakdown?: boolean
    startDate?: string
    endDate?: string
    filters?: Record<string, any>
  REDACTED>(),
  {
    upstreamEndpointStats: () => [],
    endpointPathStats: () => [],
    loading: false,
    title: '',
    metric: 'tokens',
    source: 'inbound',
    showMetricToggle: false,
    showSourceToggle: false,
    enableBreakdown: true
  REDACTED
)

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
  'update:source': [value: EndpointSource]
REDACTED>()

const expandedKey = ref<string | null>(null)
const breakdownItems = ref<UserBreakdownItem[]>([])
const breakdownLoading = ref(false)

const toggleBreakdown = async (endpoint: string) => {
  if (expandedKey.value === endpoint) {
    expandedKey.value = null
    return
  REDACTED
  expandedKey.value = endpoint
  breakdownLoading.value = true
  breakdownItems.value = []
  try {
    const res = await getUserBreakdown({
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      endpoint,
      endpoint_type: props.source,
    REDACTED)
    breakdownItems.value = res.users || []
  REDACTED catch {
    breakdownItems.value = []
  REDACTED finally {
    breakdownLoading.value = false
  REDACTED
REDACTED

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

const displayEndpointStats = computed(() => {
  const sourceStats = props.source === 'upstream'
    ? props.upstreamEndpointStats
    : props.source === 'path'
      ? props.endpointPathStats
      : props.endpointStats
  if (!sourceStats?.length) return []

  const metricKey = props.metric === 'actual_cost' ? 'actual_cost' : 'total_tokens'
  return [...sourceStats].sort((a, b) => b[metricKey] - a[metricKey])
REDACTED)

const chartData = computed(() => {
  if (!displayEndpointStats.value?.length) return null

  return {
    labels: displayEndpointStats.value.map((item) => item.endpoint),
    datasets: [
      {
        data: displayEndpointStats.value.map((item) =>
          props.metric === 'actual_cost' ? item.actual_cost : item.total_tokens
        ),
        backgroundColor: chartColors.slice(0, displayEndpointStats.value.length),
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
          const percentage = total > 0 ? ((value / total) * 100).toFixed(1) : '0.0'
          const formattedValue = props.metric === 'actual_cost'
            ? `$${formatCost(value)REDACTED`
            : formatTokens(value)
          return `${context.labelREDACTED: ${formattedValueREDACTED (${percentageREDACTED%)`
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
