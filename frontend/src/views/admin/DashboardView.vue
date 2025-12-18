<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Core Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total API Keys -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-blue-100 dark:bg-blue-900/30">
                <svg class="w-5 h-5 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.apiKeys') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.total_api_keys REDACTEDREDACTED</p>
                <p class="text-xs text-green-600 dark:text-green-400">{{ stats.active_api_keys REDACTEDREDACTED {{ t('common.active') REDACTEDREDACTED</p>
              </div>
            </div>
          </div>

          <!-- Service Accounts -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-purple-100 dark:bg-purple-900/30">
                <svg class="w-5 h-5 text-purple-600 dark:text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.accounts') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.total_accounts REDACTEDREDACTED</p>
                <p class="text-xs">
                  <span class="text-green-600 dark:text-green-400">{{ stats.normal_accounts REDACTEDREDACTED {{ t('common.active') REDACTEDREDACTED</span>
                  <span v-if="stats.error_accounts > 0" class="text-red-500 ml-1">{{ stats.error_accounts REDACTEDREDACTED {{ t('common.error') REDACTEDREDACTED</span>
                </p>
              </div>
            </div>
          </div>

          <!-- Today Requests -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-green-100 dark:bg-green-900/30">
                <svg class="w-5 h-5 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.todayRequests') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.today_requests REDACTEDREDACTED</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.total') REDACTEDREDACTED: {{ formatNumber(stats.total_requests) REDACTEDREDACTED</p>
              </div>
            </div>
          </div>

          <!-- New Users Today -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-emerald-100 dark:bg-emerald-900/30">
                <svg class="w-5 h-5 text-emerald-600 dark:text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.users') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">+{{ stats.today_new_users REDACTEDREDACTED</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.total') REDACTEDREDACTED: {{ formatNumber(stats.total_users) REDACTEDREDACTED</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Row 2: Token Stats -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Today Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-amber-100 dark:bg-amber-900/30">
                <svg class="w-5 h-5 text-amber-600 dark:text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.todayTokens') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(stats.today_tokens) REDACTEDREDACTED</p>
                <p class="text-xs">
                  <span class="text-amber-600 dark:text-amber-400" :title="t('admin.dashboard.actual')">${{ formatCost(stats.today_actual_cost) REDACTEDREDACTED</span>
                  <span class="text-gray-400 dark:text-gray-500" :title="t('admin.dashboard.standard')"> / ${{ formatCost(stats.today_cost) REDACTEDREDACTED</span>
                </p>
              </div>
            </div>
          </div>

          <!-- Total Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-indigo-100 dark:bg-indigo-900/30">
                <svg class="w-5 h-5 text-indigo-600 dark:text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.totalTokens') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(stats.total_tokens) REDACTEDREDACTED</p>
                <p class="text-xs">
                  <span class="text-indigo-600 dark:text-indigo-400" :title="t('admin.dashboard.actual')">${{ formatCost(stats.total_actual_cost) REDACTEDREDACTED</span>
                  <span class="text-gray-400 dark:text-gray-500" :title="t('admin.dashboard.standard')"> / ${{ formatCost(stats.total_cost) REDACTEDREDACTED</span>
                </p>
              </div>
            </div>
          </div>

          <!-- Cache Tokens -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-cyan-100 dark:bg-cyan-900/30">
                <svg class="w-5 h-5 text-cyan-600 dark:text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m.75 12l3 3m0 0l3-3m-3 3v-6m-1.5-9H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.cacheToday') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(stats.today_cache_read_tokens) REDACTEDREDACTED</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('common.create') REDACTEDREDACTED: {{ formatTokens(stats.today_cache_creation_tokens) REDACTEDREDACTED
                </p>
              </div>
            </div>
          </div>

          <!-- Avg Response Time -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-rose-100 dark:bg-rose-900/30">
                <svg class="w-5 h-5 text-rose-600 dark:text-rose-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.dashboard.avgResponse') REDACTEDREDACTED</p>
                <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatDuration(stats.average_duration_ms) REDACTEDREDACTED</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ stats.active_users REDACTEDREDACTED {{ t('admin.dashboard.activeUsers') REDACTEDREDACTED</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="card p-4">
            <div class="flex flex-wrap items-center gap-4">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') REDACTEDREDACTED:</span>
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <div class="flex items-center gap-2 ml-auto">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') REDACTEDREDACTED:</span>
                <div class="w-28">
                  <Select
                    v-model="granularity"
                    :options="granularityOptions"
                    @change="loadChartData"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- Charts Grid -->
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <!-- Model Distribution Chart -->
            <div class="card p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white mb-4">{{ t('admin.dashboard.modelDistribution') REDACTEDREDACTED</h3>
              <div class="flex items-center gap-6">
                <div class="w-48 h-48">
                  <Doughnut v-if="modelChartData" :data="modelChartData" :options="doughnutOptions" />
                  <div v-else class="flex items-center justify-center h-full text-gray-500 dark:text-gray-400 text-sm">
                    {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
                  </div>
                </div>
                <div class="flex-1 max-h-48 overflow-y-auto">
                  <table class="w-full text-xs">
                    <thead>
                      <tr class="text-gray-500 dark:text-gray-400">
                        <th class="text-left pb-2">{{ t('admin.dashboard.model') REDACTEDREDACTED</th>
                        <th class="text-right pb-2">{{ t('admin.dashboard.requests') REDACTEDREDACTED</th>
                        <th class="text-right pb-2">{{ t('admin.dashboard.tokens') REDACTEDREDACTED</th>
                        <th class="text-right pb-2">{{ t('admin.dashboard.actual') REDACTEDREDACTED</th>
                        <th class="text-right pb-2">{{ t('admin.dashboard.standard') REDACTEDREDACTED</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="model in modelStats" :key="model.model" class="border-t border-gray-100 dark:border-gray-700">
                        <td class="py-1.5 text-gray-900 dark:text-white font-medium truncate max-w-[100px]" :title="model.model">{{ model.model REDACTEDREDACTED</td>
                        <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatNumber(model.requests) REDACTEDREDACTED</td>
                        <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatTokens(model.total_tokens) REDACTEDREDACTED</td>
                        <td class="py-1.5 text-right text-green-600 dark:text-green-400">${{ formatCost(model.actual_cost) REDACTEDREDACTED</td>
                        <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">${{ formatCost(model.cost) REDACTEDREDACTED</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Token Usage Trend Chart -->
            <div class="card p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white mb-4">{{ t('admin.dashboard.tokenUsageTrend') REDACTEDREDACTED</h3>
              <div class="h-48">
                <Line v-if="trendChartData" :data="trendChartData" :options="lineOptions" />
                <div v-else class="flex items-center justify-center h-full text-gray-500 dark:text-gray-400 text-sm">
                  {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
                </div>
              </div>
            </div>
          </div>

          <!-- User Usage Trend (Full Width) -->
          <div class="card p-4">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white mb-4">{{ t('admin.dashboard.recentUsage') REDACTEDREDACTED (Top 12)</h3>
            <div class="h-64">
              <Line v-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div v-else class="flex items-center justify-center h-full text-gray-500 dark:text-gray-400 text-sm">
                {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'

const { t REDACTED = useI18n()
import { adminAPI REDACTED from '@/api/admin'
import type { DashboardStats, TrendDataPoint, ModelStat, UserUsageTrendPoint REDACTED from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
REDACTED from 'chart.js'
import { Line, Doughnut REDACTED from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const appStore = useAppStore()
const stats = ref<DashboardStats | null>(null)
const loading = ref(false)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const userTrend = ref<UserUsageTrendPoint[]>([])

// Date range
const granularity = ref<'day' | 'hour'>('day')
const startDate = ref('')
const endDate = ref('')

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') REDACTED,
  { value: 'hour', label: t('admin.dashboard.hour') REDACTED,
])

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
REDACTED)

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  cache: '#f59e0b',
  total: '#8b5cf6',
REDACTED))

// Doughnut chart options
const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      display: false,
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const value = context.raw as number
          const total = context.dataset.data.reduce((a: number, b: number) => a + b, 0)
          const percentage = ((value / total) * 100).toFixed(1)
          return `${context.labelREDACTED: ${formatTokens(value)REDACTED (${percentageREDACTED%)`
        REDACTED,
      REDACTED,
    REDACTED,
  REDACTED,
REDACTED))

// Line chart options
const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const,
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
          size: 11,
        REDACTED,
      REDACTED,
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.labelREDACTED: ${formatTokens(context.raw)REDACTED`
        REDACTED,
        footer: (tooltipItems: any) => {
          // Show both costs for the day if we have trend data
          const dataIndex = tooltipItems[0]?.dataIndex
          if (dataIndex !== undefined && trendData.value[dataIndex]) {
            const data = trendData.value[dataIndex]
            return `Actual: $${formatCost(data.actual_cost)REDACTED | Standard: $${formatCost(data.cost)REDACTED`
          REDACTED
          return ''
        REDACTED,
      REDACTED,
    REDACTED,
  REDACTED,
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid,
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10,
        REDACTED,
      REDACTED,
    REDACTED,
    y: {
      grid: {
        color: chartColors.value.grid,
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10,
        REDACTED,
        callback: (value: string | number) => formatTokens(Number(value)),
      REDACTED,
    REDACTED,
  REDACTED,
REDACTED))

// Model chart data
const modelChartData = computed(() => {
  if (!modelStats.value.length) return null

  const colors = [
    '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
    '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16'
  ]

  return {
    labels: modelStats.value.map(m => m.model),
    datasets: [{
      data: modelStats.value.map(m => m.total_tokens),
      backgroundColor: colors.slice(0, modelStats.value.length),
      borderWidth: 0,
    REDACTED],
  REDACTED
REDACTED)

// Trend chart data
const trendChartData = computed(() => {
  if (!trendData.value.length) return null

  return {
    labels: trendData.value.map(d => d.date),
    datasets: [
      {
        label: 'Input',
        data: trendData.value.map(d => d.input_tokens),
        borderColor: chartColors.value.input,
        backgroundColor: `${chartColors.value.inputREDACTED20`,
        fill: true,
        tension: 0.3,
      REDACTED,
      {
        label: 'Output',
        data: trendData.value.map(d => d.output_tokens),
        borderColor: chartColors.value.output,
        backgroundColor: `${chartColors.value.outputREDACTED20`,
        fill: true,
        tension: 0.3,
      REDACTED,
      {
        label: 'Cache',
        data: trendData.value.map(d => d.cache_tokens),
        borderColor: chartColors.value.cache,
        backgroundColor: `${chartColors.value.cacheREDACTED20`,
        fill: true,
        tension: 0.3,
      REDACTED,
    ],
  REDACTED
REDACTED)

// User trend chart data
const userTrendChartData = computed(() => {
  if (!userTrend.value.length) return null

  // Group by user
  const userGroups = new Map<string, { name: string; data: Map<string, number> REDACTED>()
  const allDates = new Set<string>()

  userTrend.value.forEach(point => {
    allDates.add(point.date)
    const key = point.username || `User #${point.user_idREDACTED`
    if (!userGroups.has(key)) {
      userGroups.set(key, { name: key, data: new Map() REDACTED)
    REDACTED
    userGroups.get(key)!.data.set(point.date, point.tokens)
  REDACTED)

  const sortedDates = Array.from(allDates).sort()
  const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16', '#06b6d4', '#a855f7']

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map(date => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]REDACTED20`,
    fill: false,
    tension: 0.3,
  REDACTED))

  return {
    labels: sortedDates,
    datasets,
  REDACTED
REDACTED)

// Format helpers
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

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)REDACTEDs`
  REDACTED
  return `${Math.round(ms)REDACTEDms`
REDACTED

// Date range change handler
const onDateRangeChange = (range: { startDate: string; endDate: string; preset: string | null REDACTED) => {
  // Auto-select granularity based on date range
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  // If range is 1 day, use hourly granularity
  if (daysDiff <= 1) {
    granularity.value = 'hour'
  REDACTED else {
    granularity.value = 'day'
  REDACTED

  loadChartData()
REDACTED

// Initialize default date range
const initializeDateRange = () => {
  const now = new Date()
  const today = now.toISOString().split('T')[0]
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)

  startDate.value = weekAgo.toISOString().split('T')[0]
  endDate.value = today
  granularity.value = 'day'
REDACTED

// Load data
const loadDashboardStats = async () => {
  loading.value = true
  try {
    stats.value = await adminAPI.dashboard.getStats()
  REDACTED catch (error) {
    appStore.showError(t('admin.dashboard.failedToLoad'))
    console.error('Error loading dashboard stats:', error)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const loadChartData = async () => {
  try {
    const params = {
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
    REDACTED

    const [trendResponse, modelResponse, userResponse] = await Promise.all([
      adminAPI.dashboard.getUsageTrend(params),
      adminAPI.dashboard.getModelStats({ start_date: startDate.value, end_date: endDate.value REDACTED),
      adminAPI.dashboard.getUserUsageTrend({ ...params, limit: 12 REDACTED),
    ])

    trendData.value = trendResponse.trend || []
    modelStats.value = modelResponse.models || []
    userTrend.value = userResponse.trend || []
  REDACTED catch (error) {
    console.error('Error loading chart data:', error)
  REDACTED
REDACTED

onMounted(() => {
  loadDashboardStats()
  initializeDateRange()
  loadChartData()
REDACTED)

// Watch for dark mode changes
watch(isDarkMode, () => {
  // Force chart re-render on theme change
REDACTED)
</script>

<style scoped>
/* Compact Select styling for dashboard */
:deep(.select-trigger) {
  @apply px-3 py-1.5 text-sm rounded-lg;
REDACTED

:deep(.select-dropdown) {
  @apply rounded-lg;
REDACTED

:deep(.select-option) {
  @apply px-3 py-2 text-sm;
REDACTED
</style>
