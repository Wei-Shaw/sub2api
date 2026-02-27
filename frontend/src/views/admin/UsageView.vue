<template>
  <AppLayout>
    <div class="space-y-6">
      <UsageStatsCards :stats="usageStats" />
      <!-- Charts Section -->
      <div class="space-y-4">
        <div class="card p-4">
          <div class="flex items-center gap-4">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') REDACTEDREDACTED:</span>
            <div class="w-28">
              <Select v-model="granularity" :options="granularityOptions" @change="loadChartData" />
            </div>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ModelDistributionChart :model-stats="modelStats" :loading="chartsLoading" />
          <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
        </div>
      </div>
      <UsageFilters v-model="filters" v-model:startDate="startDate" v-model:endDate="endDate" :exporting="exporting" @change="applyFilters" @refresh="refreshData" @reset="resetFilters" @cleanup="openCleanupDialog" @export="exportToExcel">
        <template #after-reset>
          <div class="relative" ref="columnDropdownRef">
            <button
              @click="showColumnDropdown = !showColumnDropdown"
              class="btn btn-secondary px-2 md:px-3"
              :title="t('admin.users.columnSettings')"
            >
              <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
              </svg>
              <span class="hidden md:inline">{{ t('admin.users.columnSettings') REDACTEDREDACTED</span>
            </button>
            <div
              v-if="showColumnDropdown"
              class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                v-for="col in toggleableColumns"
                :key="col.key"
                @click="toggleColumn(col.key)"
                class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <span>{{ col.label REDACTEDREDACTED</span>
                <Icon
                  v-if="isColumnVisible(col.key)"
                  name="check"
                  size="sm"
                  class="text-primary-500"
                  :stroke-width="2"
                />
              </button>
            </div>
          </div>
        </template>
      </UsageFilters>
      <UsageTable :data="usageLogs" :loading="loading" :columns="visibleColumns" />
      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" />
    </div>
  </AppLayout>
  <UsageExportProgress :show="exportProgress.show" :progress="exportProgress.progress" :current="exportProgress.current" :total="exportProgress.total" :estimated-time="exportProgress.estimatedTime" @cancel="cancelExport" />
  <UsageCleanupDialog
    :show="cleanupDialogVisible"
    :filters="filters"
    :start-date="startDate"
    :end-date="endDate"
    @close="cleanupDialogVisible = false"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { saveAs REDACTED from 'file-saver'
import { useAppStore REDACTED from '@/stores/app'; import { adminAPI REDACTED from '@/api/admin'; import { adminUsageAPI REDACTED from '@/api/admin/usage'
import { formatReasoningEffort REDACTED from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'; import Pagination from '@/components/common/Pagination.vue'; import Select from '@/components/common/Select.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'; import UsageFilters from '@/components/admin/usage/UsageFilters.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'; import UsageExportProgress from '@/components/admin/usage/UsageExportProgress.vue'
import UsageCleanupDialog from '@/components/admin/usage/UsageCleanupDialog.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'; import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminUsageLog, TrendDataPoint, ModelStat REDACTED from '@/types'; import type { AdminUsageStatsResponse, AdminUsageQueryParams REDACTED from '@/api/admin/usage'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const usageStats = ref<AdminUsageStatsResponse | null>(null); const usageLogs = ref<AdminUsageLog[]>([]); const loading = ref(false); const exporting = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const chartsLoading = ref(false); const granularity = ref<'day' | 'hour'>('day')
let abortController: AbortController | null = null; let exportAbortController: AbortController | null = null
const exportProgress = reactive({ show: false, progress: 0, current: 0, total: 0, estimatedTime: '' REDACTED)
const cleanupDialogVisible = ref(false)

const granularityOptions = computed(() => [{ value: 'day', label: t('admin.dashboard.day') REDACTED, { value: 'hour', label: t('admin.dashboard.hour') REDACTED])
// Use local timezone to avoid UTC timezone issues
const formatLD = (d: Date) => {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${yearREDACTED-${monthREDACTED-${dayREDACTED`
REDACTED
const now = new Date(); const weekAgo = new Date(); weekAgo.setDate(weekAgo.getDate() - 6)
const startDate = ref(formatLD(weekAgo)); const endDate = ref(formatLD(now))
const filters = ref<AdminUsageQueryParams>({ user_id: undefined, model: undefined, group_id: undefined, billing_type: null, start_date: startDate.value, end_date: endDate.value REDACTED)
const pagination = reactive({ page: 1, page_size: 20, total: 0 REDACTED)

const loadLogs = async () => {
  abortController?.abort(); const c = new AbortController(); abortController = c; loading.value = true
  try {
    const res = await adminAPI.usage.list({ page: pagination.page, page_size: pagination.page_size, ...filters.value REDACTED, { signal: c.signal REDACTED)
    if(!c.signal.aborted) { usageLogs.value = res.items; pagination.total = res.total REDACTED
  REDACTED catch (error: any) { if(error?.name !== 'AbortError') console.error('Failed to load usage logs:', error) REDACTED finally { if(abortController === c) loading.value = false REDACTED
REDACTED
const loadStats = async () => { try { const s = await adminAPI.usage.getStats(filters.value); usageStats.value = s REDACTED catch (error) { console.error('Failed to load usage stats:', error) REDACTED REDACTED
const loadChartData = async () => {
  chartsLoading.value = true
  try {
    const params = { start_date: filters.value.start_date || startDate.value, end_date: filters.value.end_date || endDate.value, granularity: granularity.value, user_id: filters.value.user_id, model: filters.value.model, api_key_id: filters.value.api_key_id, account_id: filters.value.account_id, group_id: filters.value.group_id, stream: filters.value.stream, billing_type: filters.value.billing_type REDACTED
    const [trendRes, modelRes] = await Promise.all([adminAPI.dashboard.getUsageTrend(params), adminAPI.dashboard.getModelStats({ start_date: params.start_date, end_date: params.end_date, user_id: params.user_id, model: params.model, api_key_id: params.api_key_id, account_id: params.account_id, group_id: params.group_id, stream: params.stream, billing_type: params.billing_type REDACTED)])
    trendData.value = trendRes.trend || []; modelStats.value = modelRes.models || []
  REDACTED catch (error) { console.error('Failed to load chart data:', error) REDACTED finally { chartsLoading.value = false REDACTED
REDACTED
const applyFilters = () => { pagination.page = 1; loadLogs(); loadStats(); loadChartData() REDACTED
const refreshData = () => { loadLogs(); loadStats(); loadChartData() REDACTED
const resetFilters = () => { startDate.value = formatLD(weekAgo); endDate.value = formatLD(now); filters.value = { start_date: startDate.value, end_date: endDate.value, billing_type: null REDACTED; granularity.value = 'day'; applyFilters() REDACTED
const handlePageChange = (p: number) => { pagination.page = p; loadLogs() REDACTED
const handlePageSizeChange = (s: number) => { pagination.page_size = s; pagination.page = 1; loadLogs() REDACTED
const cancelExport = () => exportAbortController?.abort()
const openCleanupDialog = () => { cleanupDialogVisible.value = true REDACTED

const exportToExcel = async () => {
  if (exporting.value) return; exporting.value = true; exportProgress.show = true
  const c = new AbortController(); exportAbortController = c
  try {
    let p = 1; let total = pagination.total; let exportedCount = 0
    const XLSX = await import('xlsx')
    const headers = [
      t('usage.time'), t('admin.usage.user'), t('usage.apiKeyFilter'),
      t('admin.usage.account'), t('usage.model'), t('usage.reasoningEffort'), t('admin.usage.group'),
      t('usage.type'),
      t('admin.usage.inputTokens'), t('admin.usage.outputTokens'),
      t('admin.usage.cacheReadTokens'), t('admin.usage.cacheCreationTokens'),
      t('admin.usage.inputCost'), t('admin.usage.outputCost'),
      t('admin.usage.cacheReadCost'), t('admin.usage.cacheCreationCost'),
      t('usage.rate'), t('usage.accountMultiplier'), t('usage.original'), t('usage.userBilled'), t('usage.accountBilled'),
      t('usage.firstToken'), t('usage.duration'),
      t('admin.usage.requestId'), t('usage.userAgent'), t('admin.usage.ipAddress')
    ]
    const ws = XLSX.utils.aoa_to_sheet([headers])
    while (true) {
      const res = await adminUsageAPI.list({ page: p, page_size: 100, ...filters.value REDACTED, { signal: c.signal REDACTED)
      if (c.signal.aborted) break; if (p === 1) { total = res.total; exportProgress.total = total REDACTED
      const rows = (res.items || []).map((log: AdminUsageLog) => [
        log.created_at, log.user?.email || '', log.api_key?.name || '', log.account?.name || '', log.model,
        formatReasoningEffort(log.reasoning_effort), log.group?.name || '', log.stream ? t('usage.stream') : t('usage.sync'),
        log.input_tokens, log.output_tokens, log.cache_read_tokens, log.cache_creation_tokens,
        log.input_cost?.toFixed(6) || '0.000000', log.output_cost?.toFixed(6) || '0.000000',
        log.cache_read_cost?.toFixed(6) || '0.000000', log.cache_creation_cost?.toFixed(6) || '0.000000',
        log.rate_multiplier?.toFixed(2) || '1.00', (log.account_rate_multiplier ?? 1).toFixed(2),
        log.total_cost?.toFixed(6) || '0.000000', log.actual_cost?.toFixed(6) || '0.000000',
        (log.total_cost * (log.account_rate_multiplier ?? 1)).toFixed(6), log.first_token_ms ?? '', log.duration_ms,
        log.request_id || '', log.user_agent || '', log.ip_address || ''
      ])
      if (rows.length) {
        XLSX.utils.sheet_add_aoa(ws, rows, { origin: -1 REDACTED)
      REDACTED
      exportedCount += rows.length
      exportProgress.current = exportedCount
      exportProgress.progress = total > 0 ? Math.min(100, Math.round(exportedCount / total * 100)) : 0
      if (exportedCount >= total || res.items.length < 100) break; p++
    REDACTED
    if(!c.signal.aborted) {
      const wb = XLSX.utils.book_new()
      XLSX.utils.book_append_sheet(wb, ws, 'Usage')
      saveAs(new Blob([XLSX.write(wb, { bookType: 'xlsx', type: 'array' REDACTED)], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' REDACTED), `usage_${filters.value.start_dateREDACTED_to_${filters.value.end_dateREDACTED.xlsx`)
      appStore.showSuccess(t('usage.exportSuccess'))
    REDACTED
  REDACTED catch (error) { console.error('Failed to export:', error); appStore.showError('Export Failed') REDACTED
  finally { if(exportAbortController === c) { exportAbortController = null; exporting.value = false; exportProgress.show = false REDACTED REDACTED
REDACTED

// Column visibility
const ALWAYS_VISIBLE = ['user', 'created_at']
const DEFAULT_HIDDEN_COLUMNS = ['reasoning_effort', 'user_agent']
const HIDDEN_COLUMNS_KEY = 'usage-hidden-columns'

const allColumns = computed(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false REDACTED,
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false REDACTED,
  { key: 'account', label: t('admin.usage.account'), sortable: false REDACTED,
  { key: 'model', label: t('usage.model'), sortable: true REDACTED,
  { key: 'reasoning_effort', label: t('usage.reasoningEffort'), sortable: false REDACTED,
  { key: 'group', label: t('admin.usage.group'), sortable: false REDACTED,
  { key: 'stream', label: t('usage.type'), sortable: false REDACTED,
  { key: 'tokens', label: t('usage.tokens'), sortable: false REDACTED,
  { key: 'cost', label: t('usage.cost'), sortable: false REDACTED,
  { key: 'first_token', label: t('usage.firstToken'), sortable: false REDACTED,
  { key: 'duration', label: t('usage.duration'), sortable: false REDACTED,
  { key: 'created_at', label: t('usage.time'), sortable: true REDACTED,
  { key: 'user_agent', label: t('usage.userAgent'), sortable: false REDACTED,
  { key: 'ip_address', label: t('admin.usage.ipAddress'), sortable: false REDACTED
])

const hiddenColumns = reactive<Set<string>>(new Set())

const toggleableColumns = computed(() =>
  allColumns.value.filter(col => !ALWAYS_VISIBLE.includes(col.key))
)

const visibleColumns = computed(() =>
  allColumns.value.filter(col =>
    ALWAYS_VISIBLE.includes(col.key) || !hiddenColumns.has(col.key)
  )
)

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  REDACTED else {
    hiddenColumns.add(key)
  REDACTED
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  REDACTED catch (e) {
    console.error('Failed to save columns:', e)
  REDACTED
REDACTED

const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      (JSON.parse(saved) as string[]).forEach(key => hiddenColumns.add(key))
    REDACTED else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    REDACTED
  REDACTED catch {
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  REDACTED
REDACTED

const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

const handleColumnClickOutside = (event: MouseEvent) => {
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(event.target as HTMLElement)) {
    showColumnDropdown.value = false
  REDACTED
REDACTED

onMounted(() => { loadLogs(); loadStats(); loadChartData(); loadSavedColumns(); document.addEventListener('click', handleColumnClickOutside) REDACTED)
onUnmounted(() => { abortController?.abort(); exportAbortController?.abort(); document.removeEventListener('click', handleColumnClickOutside) REDACTED)
</script>
