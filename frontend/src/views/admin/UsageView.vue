<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Summary Stats Cards -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <!-- Total Requests -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="p-2 rounded-lg bg-blue-100 dark:bg-blue-900/30">
              <svg class="w-5 h-5 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.totalRequests') REDACTEDREDACTED</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">{{ usageStats?.total_requests?.toLocaleString() || '0' REDACTEDREDACTED</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.inSelectedRange') REDACTEDREDACTED</p>
            </div>
          </div>
        </div>

        <!-- Total Tokens -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="p-2 rounded-lg bg-amber-100 dark:bg-amber-900/30">
              <svg class="w-5 h-5 text-amber-600 dark:text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.totalTokens') REDACTEDREDACTED</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(usageStats?.total_tokens || 0) REDACTEDREDACTED</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.in') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_input_tokens || 0) REDACTEDREDACTED / {{ t('usage.out') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_output_tokens || 0) REDACTEDREDACTED</p>
            </div>
          </div>
        </div>

        <!-- Total Cost -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="p-2 rounded-lg bg-green-100 dark:bg-green-900/30">
              <svg class="w-5 h-5 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.totalCost') REDACTEDREDACTED</p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">${{ (usageStats?.total_actual_cost || 0).toFixed(4) REDACTEDREDACTED</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                <span class="line-through">${{ (usageStats?.total_cost || 0).toFixed(4) REDACTEDREDACTED</span> {{ t('usage.standardCost') REDACTEDREDACTED
              </p>
            </div>
          </div>
        </div>

        <!-- Average Duration -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="p-2 rounded-lg bg-purple-100 dark:bg-purple-900/30">
              <svg class="w-5 h-5 text-purple-600 dark:text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.avgDuration') REDACTEDREDACTED</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatDuration(usageStats?.average_duration_ms || 0) REDACTEDREDACTED</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.perRequest') REDACTEDREDACTED</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Charts Section -->
      <div class="space-y-4">
        <!-- Chart Controls -->
        <div class="card p-4">
          <div class="flex items-center gap-4">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') REDACTEDREDACTED:</span>
            <div class="w-28">
              <Select
                v-model="granularity"
                :options="granularityOptions"
                @change="onGranularityChange"
              />
            </div>
          </div>
        </div>

        <!-- Charts Grid -->
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ModelDistributionChart
            :model-stats="modelStats"
            :loading="chartsLoading"
          />
          <TokenUsageTrend
            :trend-data="trendData"
            :loading="chartsLoading"
          />
        </div>
      </div>

      <!-- Filters -->
      <div class="card">
        <div class="px-6 py-4">
          <div class="flex flex-wrap items-end gap-4">
            <!-- User Search -->
            <div class="min-w-[200px]">
              <label class="input-label">{{ t('admin.usage.userFilter') REDACTEDREDACTED</label>
              <div class="relative">
                <input
                  v-model="userSearchKeyword"
                  type="text"
                  class="input pr-8"
                  :placeholder="t('admin.usage.searchUserPlaceholder')"
                  @input="debounceSearchUsers"
                  @focus="showUserDropdown = true"
                />
                <button
                  v-if="selectedUser"
                  @click="clearUserFilter"
                  class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                >
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
                <!-- User Dropdown -->
                <div
                  v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
                  class="absolute z-50 w-full mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg max-h-60 overflow-auto"
                >
                  <div v-if="userSearchLoading" class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {{ t('common.loading') REDACTEDREDACTED
                  </div>
                  <div v-else-if="userSearchResults.length === 0 && userSearchKeyword" class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {{ t('common.noOptionsFound') REDACTEDREDACTED
                  </div>
                  <button
                    v-for="user in userSearchResults"
                    :key="user.id"
                    @click="selectUser(user)"
                    class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                  >
                    <span class="font-medium text-gray-900 dark:text-white">{{ user.email REDACTEDREDACTED</span>
                    <span class="text-gray-500 dark:text-gray-400 ml-2">#{{ user.id REDACTEDREDACTED</span>
                  </button>
                </div>
              </div>
            </div>

            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') REDACTEDREDACTED</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
                :disabled="!selectedUser && apiKeys.length === 0"
                @change="applyFilters"
              />
            </div>

            <!-- Date Range Filter -->
            <div>
              <label class="input-label">{{ t('usage.timeRange') REDACTEDREDACTED</label>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-3 ml-auto">
              <button
                @click="resetFilters"
                class="btn btn-secondary"
              >
                {{ t('common.reset') REDACTEDREDACTED
              </button>
              <button
                @click="exportToCSV"
                class="btn btn-primary"
              >
                {{ t('usage.exportCsv') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Usage Table -->
      <div class="card overflow-hidden">
        <DataTable
          :columns="columns"
          :data="usageLogs"
          :loading="loading"
        >
          <template #cell-user="{ row REDACTED">
            <div class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.user?.email || '-' REDACTEDREDACTED</span>
              <span class="text-gray-500 dark:text-gray-400 ml-1">#{{ row.user_id REDACTEDREDACTED</span>
            </div>
          </template>

          <template #cell-api_key="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">{{ row.api_key?.name || '-' REDACTEDREDACTED</span>
          </template>

          <template #cell-model="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-stream="{ row REDACTED">
            <span
              class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
              :class="row.stream
                ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'"
            >
              {{ row.stream ? t('usage.stream') : t('usage.sync') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-tokens="{ row REDACTED">
            <div class="text-sm space-y-1.5">
              <!-- Input / Output Tokens -->
              <div class="flex items-center gap-2">
                <!-- Input -->
                <div class="inline-flex items-center gap-1">
                  <svg class="w-3.5 h-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{ row.input_tokens.toLocaleString() REDACTEDREDACTED</span>
                </div>
                <!-- Output -->
                <div class="inline-flex items-center gap-1">
                  <svg class="w-3.5 h-3.5 text-violet-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 10l7-7m0 0l7 7m-7-7v18" />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{ row.output_tokens.toLocaleString() REDACTEDREDACTED</span>
                </div>
              </div>
              <!-- Cache Tokens (Read + Write) -->
              <div v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0" class="flex items-center gap-2">
                <!-- Cache Read -->
                <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                  <svg class="w-3.5 h-3.5 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
                  </svg>
                  <span class="text-sky-600 dark:text-sky-400 font-medium">{{ formatCacheTokens(row.cache_read_tokens) REDACTEDREDACTED</span>
                </div>
                <!-- Cache Write -->
                <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                  <svg class="w-3.5 h-3.5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                  </svg>
                  <span class="text-amber-600 dark:text-amber-400 font-medium">{{ formatCacheTokens(row.cache_creation_tokens) REDACTEDREDACTED</span>
                </div>
              </div>
            </div>
          </template>

          <template #cell-cost="{ row REDACTED">
            <div class="text-sm flex items-center gap-1.5">
              <span class="font-medium text-green-600 dark:text-green-400">
                ${{ row.actual_cost.toFixed(6) REDACTEDREDACTED
              </span>
              <!-- Cost Detail Tooltip -->
              <div class="relative group">
                <div class="flex items-center justify-center w-4 h-4 rounded-full bg-gray-100 dark:bg-gray-700 cursor-help transition-colors group-hover:bg-blue-100 dark:group-hover:bg-blue-900/50">
                  <svg class="w-3 h-3 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
                  </svg>
                </div>
                <!-- Tooltip Content (right side) -->
                <div class="absolute z-[100] invisible group-hover:visible opacity-0 group-hover:opacity-100 transition-all duration-200 left-full top-1/2 -translate-y-1/2 ml-2">
                  <div class="bg-gray-900 dark:bg-gray-800 text-white text-xs rounded-lg py-2.5 px-3 shadow-xl whitespace-nowrap border border-gray-700 dark:border-gray-600">
                    <div class="space-y-1.5">
                      <div class="flex items-center justify-between gap-6">
                        <span class="text-gray-400">{{ t('usage.rate') REDACTEDREDACTED</span>
                        <span class="font-semibold text-blue-400">{{ (row.rate_multiplier || 1).toFixed(2) REDACTEDREDACTEDx</span>
                      </div>
                      <div class="flex items-center justify-between gap-6">
                        <span class="text-gray-400">{{ t('usage.original') REDACTEDREDACTED</span>
                        <span class="font-medium text-white">${{ row.total_cost.toFixed(6) REDACTEDREDACTED</span>
                      </div>
                      <div class="flex items-center justify-between gap-6 pt-1.5 border-t border-gray-700">
                        <span class="text-gray-400">{{ t('usage.billed') REDACTEDREDACTED</span>
                        <span class="font-semibold text-green-400">${{ row.actual_cost.toFixed(6) REDACTEDREDACTED</span>
                      </div>
                    </div>
                    <!-- Tooltip Arrow (left side) -->
                    <div class="absolute right-full top-1/2 -translate-y-1/2 w-0 h-0 border-t-[6px] border-t-transparent border-b-[6px] border-b-transparent border-r-[6px] border-r-gray-900 dark:border-r-gray-800"></div>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template #cell-billing_type="{ row REDACTED">
            <span
              class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
              :class="row.billing_type === 1
                ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
                : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'"
            >
              {{ row.billing_type === 1 ? t('usage.subscription') : t('usage.balance') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-first_token="{ row REDACTED">
            <span v-if="row.first_token_ms != null" class="text-sm text-gray-600 dark:text-gray-400">
              {{ formatDuration(row.first_token_ms) REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-duration="{ row REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDuration(row.duration_ms) REDACTEDREDACTED</span>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
          </template>

          <template #empty>
            <EmptyState :message="t('usage.noRecords')" />
          </template>
        </DataTable>
      </div>

      <!-- Pagination -->
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { UsageLog, TrendDataPoint, ModelStat REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import type { SimpleUser, SimpleApiKey, AdminUsageStatsResponse, AdminUsageQueryParams REDACTED from '@/api/admin/usage'

const { t REDACTED = useI18n()
const appStore = useAppStore()

// Usage stats from API
const usageStats = ref<AdminUsageStatsResponse | null>(null)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const chartsLoading = ref(false)
const granularity = ref<'day' | 'hour'>('day')

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') REDACTED,
  { value: 'hour', label: t('admin.dashboard.hour') REDACTED,
])

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false REDACTED,
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'billing_type', label: t('usage.billingType'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED
])

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<SimpleApiKey[]>([])
const loading = ref(false)

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let searchTimeout: ReturnType<typeof setTimeout> | null = null

// API Key options computed from selected user's keys
const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') REDACTED,
    ...apiKeys.value.map(key => ({
      value: key.id,
      label: key.name
    REDACTED))
  ]
REDACTED)

// Date range state
const startDate = ref('')
const endDate = ref('')

const filters = ref<AdminUsageQueryParams>({
  user_id: undefined,
  api_key_id: undefined,
  start_date: undefined,
  end_date: undefined
REDACTED)

// Initialize default date range (last 7 days)
const initializeDateRange = () => {
  const now = new Date()
  const today = now.toISOString().split('T')[0]
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)

  startDate.value = weekAgo.toISOString().split('T')[0]
  endDate.value = today
  filters.value.start_date = startDate.value
  filters.value.end_date = endDate.value
REDACTED

// User search with debounce
const debounceSearchUsers = () => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  REDACTED
  searchTimeout = setTimeout(searchUsers, 300)
REDACTED

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()
  if (!keyword) {
    userSearchResults.value = []
    return
  REDACTED

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  REDACTED catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  REDACTED finally {
    userSearchLoading.value = false
  REDACTED
REDACTED

const selectUser = async (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  filters.value.user_id = user.id
  filters.value.api_key_id = undefined

  // Load API keys for selected user
  await loadApiKeysForUser(user.id)
  applyFilters()
REDACTED

const clearUserFilter = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  filters.value.user_id = undefined
  filters.value.api_key_id = undefined
  apiKeys.value = []
  applyFilters()
REDACTED

const loadApiKeysForUser = async (userId: number) => {
  try {
    apiKeys.value = await adminAPI.usage.searchApiKeys(userId)
  REDACTED catch (error) {
    console.error('Failed to load API keys:', error)
    apiKeys.value = []
  REDACTED
REDACTED

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: { startDate: string; endDate: string; preset: string | null REDACTED) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
REDACTED

const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms.toFixed(0)REDACTEDms`
  return `${(ms / 1000).toFixed(2)REDACTEDs`
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

// Compact format for cache tokens in table cells
const formatCacheTokens = (value: number): string => {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

const formatDateTime = (dateString: string): string => {
  const date = new Date(dateString)
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  REDACTED)
REDACTED

const loadUsageLogs = async () => {
  loading.value = true
  try {
    const params: AdminUsageQueryParams = {
      page: pagination.value.page,
      page_size: pagination.value.page_size,
      ...filters.value
    REDACTED

    const response = await adminAPI.usage.list(params)
    usageLogs.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages
  REDACTED catch (error) {
    appStore.showError(t('usage.failedToLoad'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const loadUsageStats = async () => {
  try {
    const stats = await adminAPI.usage.getStats({
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined,
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value
    REDACTED)
    usageStats.value = stats
  REDACTED catch (error) {
    console.error('Failed to load usage stats:', error)
  REDACTED
REDACTED

const loadChartData = async () => {
  chartsLoading.value = true
  try {
    const params = {
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value,
      granularity: granularity.value,
      user_id: filters.value.user_id,
      api_key_id: filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined,
    REDACTED

    const [trendResponse, modelResponse] = await Promise.all([
      adminAPI.dashboard.getUsageTrend(params),
      adminAPI.dashboard.getModelStats({
        start_date: params.start_date,
        end_date: params.end_date,
        user_id: params.user_id,
        api_key_id: params.api_key_id,
      REDACTED),
    ])

    trendData.value = trendResponse.trend || []
    modelStats.value = modelResponse.models || []
  REDACTED catch (error) {
    console.error('Failed to load chart data:', error)
  REDACTED finally {
    chartsLoading.value = false
  REDACTED
REDACTED

const onGranularityChange = () => {
  loadChartData()
REDACTED

const applyFilters = () => {
  pagination.value.page = 1
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
REDACTED

const resetFilters = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  apiKeys.value = []
  filters.value = {
    user_id: undefined,
    api_key_id: undefined,
    start_date: undefined,
    end_date: undefined
  REDACTED
  granularity.value = 'day'
  // Reset date range to default (last 7 days)
  initializeDateRange()
  pagination.value.page = 1
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
REDACTED

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadUsageLogs()
REDACTED

const exportToCSV = () => {
  if (usageLogs.value.length === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  REDACTED

  const headers = ['User', 'API Key', 'Model', 'Type', 'Input Tokens', 'Output Tokens', 'Cache Read Tokens', 'Cache Write Tokens', 'Total Cost', 'Billing Type', 'Duration (ms)', 'Time']
  const rows = usageLogs.value.map(log => [
    log.user?.email || '',
    log.api_key?.name || '',
    log.model,
    log.stream ? 'Stream' : 'Sync',
    log.input_tokens,
    log.output_tokens,
    log.cache_read_tokens,
    log.cache_creation_tokens,
    log.total_cost.toFixed(6),
    log.billing_type === 1 ? 'Subscription' : 'Balance',
    log.duration_ms,
    log.created_at
  ])

  const csvContent = [
    headers.join(','),
    ...rows.map(row => row.join(','))
  ].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv' REDACTED)
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `admin_usage_${new Date().toISOString().split('T')[0]REDACTED.csv`
  link.click()
  window.URL.revokeObjectURL(url)

  appStore.showSuccess(t('usage.exportSuccess'))
REDACTED

// Click outside to close dropdown
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.relative')) {
    showUserDropdown.value = false
  REDACTED
REDACTED

onMounted(() => {
  initializeDateRange()
  loadUsageLogs()
  loadUsageStats()
  loadChartData()
  document.addEventListener('click', handleClickOutside)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  REDACTED
REDACTED)
</script>
