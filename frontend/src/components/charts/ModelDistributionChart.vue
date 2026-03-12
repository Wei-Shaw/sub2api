<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ !enableRankingView || activeView === 'model_distribution'
          ? t('admin.dashboard.modelDistribution')
          : t('admin.dashboard.spendingRankingTitle') REDACTEDREDACTED
      </h3>
      <div class="flex items-center gap-2">
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
        <div v-if="enableRankingView" class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="
              activeView === 'model_distribution'
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            "
            @click="activeView = 'model_distribution'"
          >
            {{ t('admin.dashboard.viewModelDistribution') REDACTEDREDACTED
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="
              activeView === 'spending_ranking'
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            "
            @click="activeView = 'spending_ranking'"
          >
            {{ t('admin.dashboard.viewSpendingRanking') REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>

    <div v-if="activeView === 'model_distribution' && loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="activeView === 'model_distribution' && displayModelStats.length > 0 && chartData"
      class="flex items-center gap-6"
    >
      <div class="h-48 w-48">
        <Doughnut :data="chartData" :options="doughnutOptions" />
      </div>
      <div class="max-h-48 flex-1 overflow-y-auto">
        <table class="w-full text-xs">
          <thead>
            <tr class="text-gray-500 dark:text-gray-400">
              <th class="pb-2 text-left">{{ t('admin.dashboard.model') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.requests') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.tokens') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.actual') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.standard') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="model in displayModelStats"
              :key="model.model"
              class="border-t border-gray-100 dark:border-gray-700"
            >
              <td
                class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 dark:text-white"
                :title="model.model"
              >
                {{ model.model REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatNumber(model.requests) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatTokens(model.total_tokens) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-green-600 dark:text-green-400">
                ${{ formatCost(model.actual_cost) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
                ${{ formatCost(model.cost) REDACTEDREDACTED
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div
      v-else-if="activeView === 'model_distribution'"
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
    </div>

    <div v-else-if="rankingLoading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="rankingError"
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.failedToLoad') REDACTEDREDACTED
    </div>
    <div v-else-if="rankingItems.length > 0 && rankingChartData" class="flex items-center gap-6">
      <div class="h-48 w-48">
        <Doughnut :data="rankingChartData" :options="rankingDoughnutOptions" />
      </div>
      <div class="max-h-48 flex-1 overflow-y-auto">
        <table class="w-full text-xs">
          <thead>
            <tr class="text-gray-500 dark:text-gray-400">
              <th class="pb-2 text-left">{{ t('admin.dashboard.spendingRankingUser') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.spendingRankingRequests') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.spendingRankingTokens') REDACTEDREDACTED</th>
              <th class="pb-2 text-right">{{ t('admin.dashboard.spendingRankingSpend') REDACTEDREDACTED</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(item, index) in rankingItems"
              :key="`${item.user_idREDACTED-${indexREDACTED`"
              class="cursor-pointer border-t border-gray-100 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-dark-700/40"
              @click="emit('ranking-click', item)"
            >
              <td class="py-1.5">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="shrink-0 text-[11px] font-semibold text-gray-500 dark:text-gray-400">
                    #{{ index + 1 REDACTEDREDACTED
                  </span>
                  <span
                    class="block max-w-[140px] truncate font-medium text-gray-900 dark:text-white"
                    :title="getRankingUserLabel(item)"
                  >
                    {{ getRankingUserLabel(item) REDACTEDREDACTED
                  </span>
                </div>
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatNumber(item.requests) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
                {{ formatTokens(item.tokens) REDACTEDREDACTED
              </td>
              <td class="py-1.5 text-right text-green-600 dark:text-green-400">
                ${{ formatCost(item.actual_cost) REDACTEDREDACTED
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
import { computed, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend REDACTED from 'chart.js'
import { Doughnut REDACTED from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { ModelStat, UserSpendingRankingItem REDACTED from '@/types'

ChartJS.register(ArcElement, Tooltip, Legend)

const { t REDACTED = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'
const props = withDefaults(defineProps<{
  modelStats: ModelStat[]
  enableRankingView?: boolean
  rankingItems?: UserSpendingRankingItem[]
  rankingTotalActualCost?: number
  loading?: boolean
  metric?: DistributionMetric
  showMetricToggle?: boolean
  rankingLoading?: boolean
  rankingError?: boolean
REDACTED>(), {
  enableRankingView: false,
  rankingItems: () => [],
  rankingTotalActualCost: 0,
  loading: false,
  metric: 'tokens',
  showMetricToggle: false,
  rankingLoading: false,
  rankingError: false
REDACTED)

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
  'ranking-click': [item: UserSpendingRankingItem]
REDACTED>()

const enableRankingView = computed(() => props.enableRankingView)
const activeView = ref<'model_distribution' | 'spending_ranking'>('model_distribution')

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

const displayModelStats = computed(() => {
  if (!props.modelStats?.length) return []

  const metricKey = props.metric === 'actual_cost' ? 'actual_cost' : 'total_tokens'
  return [...props.modelStats].sort((a, b) => b[metricKey] - a[metricKey])
REDACTED)

const chartData = computed(() => {
  if (!props.modelStats?.length) return null

  return {
    labels: displayModelStats.value.map((m) => m.model),
    datasets: [
      {
        data: displayModelStats.value.map((m) => props.metric === 'actual_cost' ? m.actual_cost : m.total_tokens),
        backgroundColor: chartColors.slice(0, displayModelStats.value.length),
        borderWidth: 0
      REDACTED
    ]
  REDACTED
REDACTED)

const rankingChartData = computed(() => {
  if (!props.rankingItems?.length) return null

  const rankedTotal = props.rankingItems.reduce((sum, item) => sum + item.actual_cost, 0)
  const otherActualCost = Math.max((props.rankingTotalActualCost || 0) - rankedTotal, 0)
  const labels = props.rankingItems.map((item, index) => `#${index + 1REDACTED ${getRankingUserLabel(item)REDACTED`)
  const data = props.rankingItems.map((item) => item.actual_cost)

  if (otherActualCost > 0.000001) {
    labels.push(t('admin.dashboard.spendingRankingOther'))
    data.push(otherActualCost)
  REDACTED

  return {
    labels,
    datasets: [
      {
        data,
        backgroundColor: chartColors.slice(0, data.length),
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

const rankingDoughnutOptions = computed(() => ({
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
          return `${context.labelREDACTED: $${formatCost(value)REDACTED (${percentageREDACTED%)`
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

const getRankingUserLabel = (item: UserSpendingRankingItem): string => {
  if (item.email) return item.email
  return t('admin.redeem.userPrefix', { id: item.user_id REDACTED)
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
