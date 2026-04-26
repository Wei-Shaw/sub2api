<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.dashboard.groupDistribution') REDACTEDREDACTED
      </h3>
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
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="displayGroupStats.length > 0 && chartData" class="flex items-center gap-6">
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
              <th v-if="showAccountCost" class="pb-2 text-right">{{ t('admin.dashboard.accountCost') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.standard') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="group in displayGroupStats" :key="group.group_id">
              <tr
                class="border-t border-gray-100 transition-colors dark:border-gray-700"
                :class="enableBreakdown && group.group_id > 0 ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40' : ''"
                @click="enableBreakdown && group.group_id > 0 && toggleBreakdown('group', group.group_id)"
              >
                <td
                  class="max-w-[100px] truncate py-1.5 font-medium"
                  :class="enableBreakdown && group.group_id > 0 ? 'text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300' : 'text-gray-900 dark:text-white'"
                  :title="group.group_name || String(group.group_id)"
                >
                  <span class="inline-flex items-center gap-1">
                    <svg v-if="enableBreakdown && group.group_id > 0 && expandedKey === `group-${group.group_idREDACTED`" class="h-3 w-3 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
                    <svg v-else-if="enableBreakdown && group.group_id > 0" class="h-3 w-3 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
                    {{ group.group_name || t('admin.dashboard.noGroup') REDACTEDREDACTED
                  </span>
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
                <td v-if="showAccountCost" class="py-1.5 text-right text-orange-500 dark:text-orange-400">
                  ${{ formatCost(group.account_cost) REDACTEDREDACTED
                </td>
                <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
                  ${{ formatCost(group.cost) REDACTEDREDACTED
                </td>
              </tr>
              <!-- User breakdown sub-rows -->
              <tr v-if="expandedKey === `group-${group.group_idREDACTED`">
                <td :colspan="distributionColspan" class="p-0">
                  <UserBreakdownSubTable
                    :items="breakdownItems"
                    :loading="breakdownLoading"
                    :show-account-cost="showAccountCost"
                  />
                </td>
              </tr>
            </template>
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
import { computed, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend REDACTED from 'chart.js'
import { Doughnut REDACTED from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserBreakdownSubTable from './UserBreakdownSubTable.vue'
import type { GroupStat, UserBreakdownItem REDACTED from '@/types'
import { getUserBreakdown REDACTED from '@/api/admin/dashboard'

ChartJS.register(ArcElement, Tooltip, Legend)

const { t REDACTED = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'

const props = withDefaults(defineProps<{
  groupStats: GroupStat[]
  loading?: boolean
  metric?: DistributionMetric
  showMetricToggle?: boolean
  enableBreakdown?: boolean
  showAccountCost?: boolean
  startDate?: string
  endDate?: string
  filters?: Record<string, any>
REDACTED>(), {
  loading: false,
  metric: 'tokens',
  showMetricToggle: false,
  enableBreakdown: true,
  showAccountCost: true,
REDACTED)

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
REDACTED>()

const expandedKey = ref<string | null>(null)
const breakdownItems = ref<UserBreakdownItem[]>([])
const breakdownLoading = ref(false)
const showAccountCost = computed(() => props.showAccountCost)
const distributionColspan = computed(() => showAccountCost.value ? 6 : 5)

const toggleBreakdown = async (type: string, id: number | string) => {
  const key = `${typeREDACTED-${idREDACTED`
  if (expandedKey.value === key) {
    expandedKey.value = null
    return
  REDACTED
  expandedKey.value = key
  breakdownLoading.value = true
  breakdownItems.value = []
  try {
    const res = await getUserBreakdown({
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      group_id: Number(id),
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
  '#84cc16'
]

const displayGroupStats = computed(() => {
  if (!props.groupStats?.length) return []

  const metricKey = props.metric === 'actual_cost' ? 'actual_cost' : 'total_tokens'
  return [...props.groupStats].sort((a, b) => toFiniteNumber(b[metricKey]) - toFiniteNumber(a[metricKey]))
REDACTED)

const chartData = computed(() => {
  if (!props.groupStats?.length) return null

  return {
    labels: displayGroupStats.value.map((g) => g.group_name || String(g.group_id)),
    datasets: [
      {
        data: displayGroupStats.value.map((g) => toFiniteNumber(props.metric === 'actual_cost' ? g.actual_cost : g.total_tokens)),
        backgroundColor: chartColors.slice(0, displayGroupStats.value.length),
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
  return toFiniteNumber(value).toLocaleString()
REDACTED

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
REDACTED

const formatCost = (value: number | null | undefined): string => {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1000) {
    return (safeValue / 1000).toFixed(2) + 'K'
  REDACTED else if (safeValue >= 1) {
    return safeValue.toFixed(2)
  REDACTED else if (safeValue >= 0.01) {
    return safeValue.toFixed(3)
  REDACTED
  return safeValue.toFixed(4)
REDACTED
</script>
