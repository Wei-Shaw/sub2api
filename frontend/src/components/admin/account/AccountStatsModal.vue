<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.usageStatistics')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-6">
      <!-- Account Info Header -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-primary-200 bg-gradient-to-r from-primary-50 to-primary-100 p-3 dark:border-primary-700/50 dark:from-primary-900/20 dark:to-primary-800/20"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="chartBar" size="md" class="text-white" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name REDACTEDREDACTED</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.last30DaysUsage') REDACTEDREDACTED
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status REDACTEDREDACTED
        </span>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Main Stats Cards -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- 30-Day Total Cost -->
          <div
            class="card border-emerald-200 bg-gradient-to-br from-emerald-50 to-white p-4 dark:border-emerald-800/30 dark:from-emerald-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.totalCost')
              REDACTEDREDACTED</span>
              <div class="rounded-lg bg-emerald-100 p-1.5 dark:bg-emerald-900/30">
                <Icon name="dollar" size="sm" class="text-emerald-600 dark:text-emerald-400" />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              ${{ formatCost(stats.summary.total_cost) REDACTEDREDACTED
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.accumulatedCost') REDACTEDREDACTED
              <span class="text-gray-400 dark:text-gray-500">
                ({{ t('usage.userBilled') REDACTEDREDACTED: ${{ formatCost(stats.summary.total_user_cost) REDACTEDREDACTED ·
                {{ t('admin.accounts.stats.standardCost') REDACTEDREDACTED: ${{
                  formatCost(stats.summary.total_standard_cost)
                REDACTEDREDACTED)
              </span>
            </p>
          </div>

          <!-- 30-Day Total Requests -->
          <div
            class="card border-blue-200 bg-gradient-to-br from-blue-50 to-white p-4 dark:border-blue-800/30 dark:from-blue-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.totalRequests')
              REDACTEDREDACTED</span>
              <div class="rounded-lg bg-blue-100 p-1.5 dark:bg-blue-900/30">
                <Icon name="bolt" size="sm" class="text-blue-600 dark:text-blue-400" />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              {{ formatNumber(stats.summary.total_requests) REDACTEDREDACTED
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.totalCalls') REDACTEDREDACTED
            </p>
          </div>

          <!-- Daily Average Cost -->
          <div
            class="card border-amber-200 bg-gradient-to-br from-amber-50 to-white p-4 dark:border-amber-800/30 dark:from-amber-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.avgDailyCost')
              REDACTEDREDACTED</span>
              <div class="rounded-lg bg-amber-100 p-1.5 dark:bg-amber-900/30">
                <Icon
                  name="calculator"
                  size="sm"
                  class="text-amber-600 dark:text-amber-400"
                />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              ${{ formatCost(stats.summary.avg_daily_cost) REDACTEDREDACTED
            </p>
             <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t('admin.accounts.stats.basedOnActualDays', {
                  days: stats.summary.actual_days_used
                REDACTED)
              REDACTEDREDACTED
              <span class="text-gray-400 dark:text-gray-500">
                ({{ t('usage.userBilled') REDACTEDREDACTED: ${{ formatCost(stats.summary.avg_daily_user_cost) REDACTEDREDACTED)
              </span>
            </p>
          </div>

          <!-- Daily Average Requests -->
          <div
            class="card border-purple-200 bg-gradient-to-br from-purple-50 to-white p-4 dark:border-purple-800/30 dark:from-purple-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.avgDailyRequests')
              REDACTEDREDACTED</span>
              <div class="rounded-lg bg-purple-100 p-1.5 dark:bg-purple-900/30">
                <svg
                  class="h-4 w-4 text-purple-600 dark:text-purple-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z"
                  />
                </svg>
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              {{ formatNumber(Math.round(stats.summary.avg_daily_requests)) REDACTEDREDACTED
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.avgDailyUsage') REDACTEDREDACTED
            </p>
          </div>
        </div>

        <!-- Row 2: Today, Highest Cost, Highest Requests -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Today Overview -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-cyan-100 p-1.5 dark:bg-cyan-900/30">
                <Icon name="clock" size="sm" class="text-cyan-600 dark:text-cyan-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.todayOverview')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.cost || 0) REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.user_cost || 0) REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.today?.requests || 0)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.tokens')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                REDACTEDREDACTED</span>
              </div>
            </div>
          </div>

          <!-- Highest Cost Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-orange-100 p-1.5 dark:bg-orange-900/30">
                <Icon name="fire" size="sm" class="text-orange-600 dark:text-orange-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.highestCostDay')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.date')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  stats.summary.highest_cost_day?.label || '-'
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-orange-600 dark:text-orange-400"
                  >${{ formatCost(stats.summary.highest_cost_day?.cost || 0) REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_cost_day?.user_cost || 0) REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.highest_cost_day?.requests || 0)
                REDACTEDREDACTED</span>
              </div>
            </div>
          </div>

          <!-- Highest Request Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-indigo-100 p-1.5 dark:bg-indigo-900/30">
                <Icon
                  name="trendingUp"
                  size="sm"
                  class="text-indigo-600 dark:text-indigo-400"
                />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.highestRequestDay')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.date')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  stats.summary.highest_request_day?.label || '-'
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-indigo-600 dark:text-indigo-400">{{
                  formatNumber(stats.summary.highest_request_day?.requests || 0)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_request_day?.cost || 0) REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_request_day?.user_cost || 0) REDACTEDREDACTED</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Row 3: Token Stats -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Accumulated Tokens -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-teal-100 p-1.5 dark:bg-teal-900/30">
                <Icon name="cube" size="sm" class="text-teal-600 dark:text-teal-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.accumulatedTokens')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.totalTokens')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.total_tokens)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.dailyAvgTokens')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(Math.round(stats.summary.avg_daily_tokens))
                REDACTEDREDACTED</span>
              </div>
            </div>
          </div>

          <!-- Performance -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-rose-100 p-1.5 dark:bg-rose-900/30">
                <Icon name="bolt" size="sm" class="text-rose-600 dark:text-rose-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.performance')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.avgResponseTime')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatDuration(stats.summary.avg_duration_ms)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.daysActive')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >{{ stats.summary.actual_days_used REDACTEDREDACTED / {{ stats.summary.days REDACTEDREDACTED</span
                >
              </div>
            </div>
          </div>

          <!-- Recent Activity -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-lime-100 p-1.5 dark:bg-lime-900/30">
                <Icon
                  name="clipboard"
                  size="sm"
                  class="text-lime-600 dark:text-lime-400"
                />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.recentActivity')
              REDACTEDREDACTED</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayRequests')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.today?.requests || 0)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayTokens')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                REDACTEDREDACTED</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayCost')
                REDACTEDREDACTED</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.cost || 0) REDACTEDREDACTED</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Usage Trend Chart -->
        <div class="card p-4">
          <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accounts.stats.usageTrend') REDACTEDREDACTED
          </h3>
          <div class="h-64">
            <Line v-if="trendChartData" :data="trendChartData" :options="lineChartOptions" />
            <div
              v-else
              class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
            </div>
          </div>
        </div>

        <!-- Model Distribution -->
        <ModelDistributionChart :model-stats="stats.models" :loading="false" />

        <EndpointDistributionChart
          :endpoint-stats="stats.endpoints || []"
          :loading="false"
          :title="t('usage.inboundEndpoint')"
        />

        <EndpointDistributionChart
          :endpoint-stats="stats.upstream_endpoints || []"
          :loading="false"
          :title="t('usage.upstreamEndpoint')"
        />
      </template>

      <!-- No Data State -->
      <div
        v-else-if="!loading"
        class="flex flex-col items-center justify-center py-12 text-gray-500 dark:text-gray-400"
      >
        <Icon name="chartBar" size="xl" class="mb-4 h-12 w-12" />
        <p class="text-sm">{{ t('admin.accounts.stats.noData') REDACTEDREDACTED</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
REDACTED from 'chart.js'
import { Line REDACTED from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, AccountUsageStatsResponse REDACTED from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t REDACTED = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
REDACTED>()

const emit = defineEmits<{
  (e: 'close'): void
REDACTED>()

const loading = ref(false)
const stats = ref<AccountUsageStatsResponse | null>(null)

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
REDACTED)

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
REDACTED))

// Line chart data
const trendChartData = computed(() => {
  if (!stats.value?.history?.length) return null

  return {
    labels: stats.value.history.map((h) => h.label),
    datasets: [
      {
        label: t('usage.accountBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.actual_cost),
        borderColor: '#3b82f6',
        backgroundColor: 'rgba(59, 130, 246, 0.1)',
        fill: true,
        tension: 0.3,
        yAxisID: 'y'
      REDACTED,
      {
        label: t('usage.userBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.user_cost),
        borderColor: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        fill: false,
        tension: 0.3,
        borderDash: [5, 5],
        yAxisID: 'y'
      REDACTED,
      {
        label: t('admin.accounts.stats.requests'),
        data: stats.value.history.map((h) => h.requests),
        borderColor: '#f97316',
        backgroundColor: 'rgba(249, 115, 22, 0.1)',
        fill: false,
        tension: 0.3,
        yAxisID: 'y1'
      REDACTED
    ]
  REDACTED
REDACTED)

// Line chart options with dual Y-axis
const lineChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  REDACTED,
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
        REDACTED
      REDACTED
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const label = context.dataset.label || ''
          const value = context.raw
          if (label.includes('USD')) {
            return `${labelREDACTED: $${formatCost(value)REDACTED`
          REDACTED
          return `${labelREDACTED: ${formatNumber(value)REDACTED`
        REDACTED
      REDACTED
    REDACTED
  REDACTED,
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        REDACTED,
        maxRotation: 45,
        minRotation: 0
      REDACTED
    REDACTED,
    y: {
      type: 'linear' as const,
      display: true,
      position: 'left' as const,
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: '#3b82f6',
        font: {
          size: 10
        REDACTED,
        callback: (value: string | number) => '$' + formatCost(Number(value))
      REDACTED,
      title: {
        display: true,
        text: t('usage.accountBilled') + ' (USD)',
        color: '#3b82f6',
        font: {
          size: 11
        REDACTED
      REDACTED
    REDACTED,
    y1: {
      type: 'linear' as const,
      display: true,
      position: 'right' as const,
      grid: {
        drawOnChartArea: false
      REDACTED,
      ticks: {
        color: '#f97316',
        font: {
          size: 10
        REDACTED,
        callback: (value: string | number) => formatNumber(Number(value))
      REDACTED,
      title: {
        display: true,
        text: t('admin.accounts.stats.requests'),
        color: '#f97316',
        font: {
          size: 11
        REDACTED
      REDACTED
    REDACTED
  REDACTED
REDACTED))

// Load stats when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      await loadStats()
    REDACTED else {
      stats.value = null
    REDACTED
  REDACTED
)

const loadStats = async () => {
  if (!props.account) return

  loading.value = true
  try {
    stats.value = await adminAPI.accounts.getStats(props.account.id, 30)
  REDACTED catch (error) {
    console.error('Failed to load account stats:', error)
    stats.value = null
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const handleClose = () => {
  emit('close')
REDACTED

// Format helpers
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

const formatNumber = (value: number): string => {
  if (value >= 1_000_000) {
    return (value / 1_000_000).toFixed(2) + 'M'
  REDACTED else if (value >= 1_000) {
    return (value / 1_000).toFixed(2) + 'K'
  REDACTED
  return value.toLocaleString()
REDACTED

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

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)REDACTEDs`
  REDACTED
  return `${Math.round(ms)REDACTEDms`
REDACTED
</script>
