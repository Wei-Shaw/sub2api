<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total Requests -->
          <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <svg
                class="h-5 w-5 text-blue-600 dark:text-blue-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalRequests') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ usageStats?.total_requests?.toLocaleString() || '0' REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.inSelectedRange') REDACTEDREDACTED
              </p>
            </div>
          </div>
        </div>

        <!-- Total Tokens -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
              <svg
                class="h-5 w-5 text-amber-600 dark:text-amber-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalTokens') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatTokens(usageStats?.total_tokens || 0) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.in') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_input_tokens || 0) REDACTEDREDACTED /
                {{ t('usage.out') REDACTEDREDACTED: {{ formatTokens(usageStats?.total_output_tokens || 0) REDACTEDREDACTED
              </p>
            </div>
          </div>
        </div>

        <!-- Total Cost -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
              <svg
                class="h-5 w-5 text-green-600 dark:text-green-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalCost') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">
                ${{ (usageStats?.total_actual_cost || 0).toFixed(4) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.actualCost') REDACTEDREDACTED /
                <span class="line-through">${{ (usageStats?.total_cost || 0).toFixed(4) REDACTEDREDACTED</span>
                {{ t('usage.standardCost') REDACTEDREDACTED
              </p>
            </div>
          </div>
        </div>

        <!-- Average Duration -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
              <svg
                class="h-5 w-5 text-purple-600 dark:text-purple-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.avgDuration') REDACTEDREDACTED
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatDuration(usageStats?.average_duration_ms || 0) REDACTEDREDACTED
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.perRequest') REDACTEDREDACTED</p>
            </div>
          </div>
        </div>
        </div>
      </template>

      <template #filters>
        <div class="card">
          <div class="px-6 py-4">
          <div class="flex flex-wrap items-end gap-4">
            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') REDACTEDREDACTED</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
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
            <div class="ml-auto flex items-center gap-3">
              <button @click="resetFilters" class="btn btn-secondary">
                {{ t('common.reset') REDACTEDREDACTED
              </button>
              <button @click="exportToCSV" :disabled="exporting" class="btn btn-primary">
                <svg
                  v-if="exporting"
                  class="-ml-1 mr-2 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                {{ exporting ? t('usage.exporting') : t('usage.exportCsv') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="usageLogs" :loading="loading">
          <template #cell-api_key="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-white">{{
              row.api_key?.name || '-'
            REDACTEDREDACTED</span>
          </template>

          <template #cell-model="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-stream="{ row REDACTED">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.stream
                  ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                  : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
              "
            >
              {{ row.stream ? t('usage.stream') : t('usage.sync') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-tokens="{ row REDACTED">
            <div class="space-y-1.5 text-sm">
              <!-- Input / Output Tokens -->
              <div class="flex items-center gap-2">
                <!-- Input -->
                <div class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-emerald-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 14l-7 7m0 0l-7-7m7 7V3"
                    />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{
                    row.input_tokens.toLocaleString()
                  REDACTEDREDACTED</span>
                </div>
                <!-- Output -->
                <div class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-violet-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 10l7-7m0 0l7 7m-7-7v18"
                    />
                  </svg>
                  <span class="font-medium text-gray-900 dark:text-white">{{
                    row.output_tokens.toLocaleString()
                  REDACTEDREDACTED</span>
                </div>
              </div>
              <!-- Cache Tokens (Read + Write) -->
              <div
                v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0"
                class="flex items-center gap-2"
              >
                <!-- Cache Read -->
                <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-sky-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"
                    />
                  </svg>
                  <span class="font-medium text-sky-600 dark:text-sky-400">{{
                    formatCacheTokens(row.cache_read_tokens)
                  REDACTEDREDACTED</span>
                </div>
                <!-- Cache Write -->
                <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                  <svg
                    class="h-3.5 w-3.5 text-amber-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                    />
                  </svg>
                  <span class="font-medium text-amber-600 dark:text-amber-400">{{
                    formatCacheTokens(row.cache_creation_tokens)
                  REDACTEDREDACTED</span>
                </div>
              </div>
            </div>
          </template>

          <template #cell-cost="{ row REDACTED">
            <div class="flex items-center gap-1.5 text-sm">
              <span class="font-medium text-green-600 dark:text-green-400">
                ${{ row.actual_cost.toFixed(6) REDACTEDREDACTED
              </span>
              <!-- Cost Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTooltip($event, row)"
                @mouseleave="hideTooltip"
              >
                <div
                  class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50"
                >
                  <svg
                    class="h-3 w-3 text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
                      clip-rule="evenodd"
                    />
                  </svg>
                </div>
              </div>
            </div>
          </template>

          <template #cell-billing_type="{ row REDACTED">
            <span
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
              :class="
                row.billing_type === 1
                  ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
                  : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200'
              "
            >
              {{ row.billing_type === 1 ? t('usage.subscription') : t('usage.balance') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-first_token="{ row REDACTED">
            <span
              v-if="row.first_token_ms != null"
              class="text-sm text-gray-600 dark:text-gray-400"
            >
              {{ formatDuration(row.first_token_ms) REDACTEDREDACTED
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
          </template>

          <template #cell-duration="{ row REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDuration(row.duration_ms)
            REDACTEDREDACTED</span>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{
              formatDateTime(value)
            REDACTEDREDACTED</span>
          </template>

          <template #empty>
            <EmptyState :message="t('usage.noRecords')" />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>

  <!-- Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tooltipPosition.x + 'px',
        top: tooltipPosition.y + 'px'
      REDACTED"
    >
      <div
        class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
      >
        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.rate') REDACTEDREDACTED</span>
            <span class="font-semibold text-blue-400"
              >{{ (tooltipData?.rate_multiplier || 1).toFixed(2) REDACTEDREDACTEDx</span
            >
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.original') REDACTEDREDACTED</span>
            <span class="font-medium text-white">${{ tooltipData?.total_cost.toFixed(6) REDACTEDREDACTED</span>
          </div>
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.billed') REDACTEDREDACTED</span>
            <span class="font-semibold text-green-400"
              >${{ tooltipData?.actual_cost.toFixed(6) REDACTEDREDACTED</span
            >
          </div>
        </div>
        <!-- Tooltip Arrow (left side) -->
        <div
          class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
        ></div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { usageAPI, keysAPI REDACTED from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import type { UsageLog, ApiKey, UsageQueryParams, UsageStatsResponse REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import { formatDateTime REDACTED from '@/utils/format'

const { t REDACTED = useI18n()
const appStore = useAppStore()

let abortController: AbortController | null = null

// Tooltip state
const tooltipVisible = ref(false)
const tooltipPosition = ref({ x: 0, y: 0 REDACTED)
const tooltipData = ref<UsageLog | null>(null)

// Usage stats from API
const usageStats = ref<UsageStatsResponse | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'billing_type', label: t('usage.billingType'), sortable: false REDACTED,
  { key: 'first_token', label: t('usage.firstToken'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED
])

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<ApiKey[]>([])
const loading = ref(false)
const exporting = ref(false)

const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') REDACTED,
    ...apiKeys.value.map((key) => ({
      value: key.id,
      label: key.name
    REDACTED))
  ]
REDACTED)

// Date range state
const startDate = ref('')
const endDate = ref('')

const filters = ref<UsageQueryParams>({
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

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
REDACTED) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
REDACTED

const pagination = reactive({
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

const loadUsageLogs = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  const { signal REDACTED = currentAbortController
  loading.value = true
  try {
    const params: UsageQueryParams = {
      page: pagination.page,
      page_size: pagination.page_size,
      ...filters.value
    REDACTED

    const response = await usageAPI.query(params, { signal REDACTED)
    if (signal.aborted) {
      return
    REDACTED
    usageLogs.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error) {
    if (signal.aborted) {
      return
    REDACTED
    const abortError = error as { name?: string; code?: string REDACTED
    if (abortError?.name === 'AbortError' || abortError?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('usage.failedToLoad'))
  REDACTED finally {
    if (abortController === currentAbortController) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

const loadApiKeys = async () => {
  try {
    const response = await keysAPI.list(1, 100)
    apiKeys.value = response.items
  REDACTED catch (error) {
    console.error('Failed to load API keys:', error)
  REDACTED
REDACTED

const loadUsageStats = async () => {
  try {
    const apiKeyId = filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined
    const stats = await usageAPI.getStatsByDateRange(
      filters.value.start_date || startDate.value,
      filters.value.end_date || endDate.value,
      apiKeyId
    )
    usageStats.value = stats
  REDACTED catch (error) {
    console.error('Failed to load usage stats:', error)
  REDACTED
REDACTED

const applyFilters = () => {
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
REDACTED

const resetFilters = () => {
  filters.value = {
    api_key_id: undefined,
    start_date: undefined,
    end_date: undefined
  REDACTED
  // Reset date range to default (last 7 days)
  initializeDateRange()
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadUsageLogs()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadUsageLogs()
REDACTED

/**
 * Escape CSV value to prevent injection and handle special characters
 */
const escapeCSVValue = (value: unknown): string => {
  if (value == null) return ''

  const str = String(value)
  const escaped = str.replace(/"/g, '""')

  // Prevent formula injection by prefixing dangerous characters with single quote
  if (/^[=+\-@\t\r]/.test(str)) {
    return `"\'${escapedREDACTED"`
  REDACTED

  // Escape values containing comma, quote, or newline
  if (/[,"\n\r]/.test(str)) {
    return `"${escapedREDACTED"`
  REDACTED

  return str
REDACTED

const exportToCSV = async () => {
  if (pagination.total === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  REDACTED

  exporting.value = true
  appStore.showInfo(t('usage.preparingExport'))

  try {
    const allLogs: UsageLog[] = []
    const pageSize = 100 // Use a larger page size for export to reduce requests
    const totalRequests = Math.ceil(pagination.total / pageSize)

    for (let page = 1; page <= totalRequests; page++) {
      const params: UsageQueryParams = {
        page: page,
        page_size: pageSize,
        ...filters.value
      REDACTED
      const response = await usageAPI.query(params)
      allLogs.push(...response.items)
    REDACTED

    if (allLogs.length === 0) {
      appStore.showWarning(t('usage.noDataToExport'))
      return
    REDACTED

    const headers = [
      'Time',
      'API Key Name',
      'Model',
      'Type',
      'Input Tokens',
      'Output Tokens',
      'Cache Read Tokens',
      'Cache Creation Tokens',
      'Rate Multiplier',
      'Billed Cost',
      'Original Cost',
      'Billing Type',
      'First Token (ms)',
      'Duration (ms)'
    ]
    const rows = allLogs.map((log) =>
      [
        log.created_at,
        log.api_key?.name || '',
        log.model,
        log.stream ? 'Stream' : 'Sync',
        log.input_tokens,
        log.output_tokens,
        log.cache_read_tokens,
        log.cache_creation_tokens,
        log.rate_multiplier,
        log.actual_cost.toFixed(8),
        log.total_cost.toFixed(8),
        log.billing_type === 1 ? 'Subscription' : 'Balance',
        log.first_token_ms ?? '',
        log.duration_ms
      ].map(escapeCSVValue)
    )

    const csvContent = [
      headers.map(escapeCSVValue).join(','),
      ...rows.map((row) => row.join(','))
    ].join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' REDACTED)
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `usage_${filters.value.start_dateREDACTED_to_${filters.value.end_dateREDACTED.csv`
    link.click()
    window.URL.revokeObjectURL(url)

    appStore.showSuccess(t('usage.exportSuccess'))
  REDACTED catch (error) {
    appStore.showError(t('usage.exportFailed'))
    console.error('CSV Export failed:', error)
  REDACTED finally {
    exporting.value = false
  REDACTED
REDACTED

// Tooltip functions
const showTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tooltipData.value = row
  // Position to the right of the icon, vertically centered
  tooltipPosition.value.x = rect.right + 8
  tooltipPosition.value.y = rect.top + rect.height / 2
  tooltipVisible.value = true
REDACTED

const hideTooltip = () => {
  tooltipVisible.value = false
  tooltipData.value = null
REDACTED

onMounted(() => {
  initializeDateRange()
  loadApiKeys()
  loadUsageLogs()
  loadUsageStats()
REDACTED)
</script>
