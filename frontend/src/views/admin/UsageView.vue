<template>
  <AppLayout>
    <div class="space-y-6">
      <UsageStatsCards :stats="usageStats" />
      <UsageFilters v-model="filters" v-model:startDate="startDate" v-model:endDate="endDate" :exporting="exporting" @change="applyFilters" @reset="resetFilters" @export="exportToExcel" />
      <UsageTable :data="usageLogs" :loading="loading" />
      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" />
    </div>
  </AppLayout>
  <UsageExportProgress :show="exportProgress.show" :progress="exportProgress.progress" :current="exportProgress.current" :total="exportProgress.total" :estimated-time="exportProgress.estimatedTime" @cancel="cancelExport" />
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted REDACTED from 'vue'
import { saveAs REDACTED from 'file-saver'
import { useAppStore REDACTED from '@/stores/app'; import { adminAPI REDACTED from '@/api/admin'; import { adminUsageAPI REDACTED from '@/api/admin/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import Pagination from '@/components/common/Pagination.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'; import UsageFilters from '@/components/admin/usage/UsageFilters.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'; import UsageExportProgress from '@/components/admin/usage/UsageExportProgress.vue'
import type { UsageLog REDACTED from '@/types'; import type { AdminUsageStatsResponse, AdminUsageQueryParams REDACTED from '@/api/admin/usage'

const appStore = useAppStore()
const usageStats = ref<AdminUsageStatsResponse | null>(null); const usageLogs = ref<UsageLog[]>([]); const loading = ref(false); const exporting = ref(false)
let abortController: AbortController | null = null; let exportAbortController: AbortController | null = null
const exportProgress = reactive({ show: false, progress: 0, current: 0, total: 0, estimatedTime: '' REDACTED)

const formatLD = (d: Date) => d.toISOString().split('T')[0]
const now = new Date(); const weekAgo = new Date(Date.now() - 6 * 86400000)
const startDate = ref(formatLD(weekAgo)); const endDate = ref(formatLD(now))
const filters = ref<AdminUsageQueryParams>({ user_id: undefined, model: undefined, group_id: undefined, start_date: startDate.value, end_date: endDate.value REDACTED)
const pagination = reactive({ page: 1, page_size: 20, total: 0 REDACTED)

const loadLogs = async () => {
  abortController?.abort(); const c = new AbortController(); abortController = c; loading.value = true
  try {
    const res = await adminAPI.usage.list({ page: pagination.page, page_size: pagination.page_size, ...filters.value REDACTED, { signal: c.signal REDACTED)
    if(!c.signal.aborted) { usageLogs.value = res.items; pagination.total = res.total REDACTED
  REDACTED catch (error: any) { if(error?.name !== 'AbortError') console.error('Failed to load usage logs:', error) REDACTED finally { if(abortController === c) loading.value = false REDACTED
REDACTED
const loadStats = async () => { try { const s = await adminAPI.usage.getStats(filters.value); usageStats.value = s REDACTED catch (error) { console.error('Failed to load usage stats:', error) REDACTED REDACTED
const applyFilters = () => { pagination.page = 1; loadLogs(); loadStats() REDACTED
const resetFilters = () => { startDate.value = formatLD(weekAgo); endDate.value = formatLD(now); filters.value = { start_date: startDate.value, end_date: endDate.value REDACTED; applyFilters() REDACTED
const handlePageChange = (p: number) => { pagination.page = p; loadLogs() REDACTED
const handlePageSizeChange = (s: number) => { pagination.page_size = s; pagination.page = 1; loadLogs() REDACTED
const cancelExport = () => exportAbortController?.abort()

const exportToExcel = async () => {
  if (exporting.value) return; exporting.value = true; exportProgress.show = true
  const c = new AbortController(); exportAbortController = c
  try {
    const all: UsageLog[] = []; let p = 1; let total = pagination.total
    while (true) {
      const res = await adminUsageAPI.list({ page: p, page_size: 100, ...filters.value REDACTED, { signal: c.signal REDACTED)
      if (c.signal.aborted) break; if (p === 1) { total = res.total; exportProgress.total = total REDACTED
      if (res.items?.length) all.push(...res.items)
      exportProgress.current = all.length; exportProgress.progress = total > 0 ? Math.min(100, Math.round(all.length/total*100)) : 0
      if (all.length >= total || res.items.length < 100) break; p++
    REDACTED
    if(!c.signal.aborted) {
      // 动态加载 xlsx，降低首屏包体并减少高危依赖的常驻暴露面。
      const XLSX = await import('xlsx')
      const ws = XLSX.utils.json_to_sheet(all); const wb = XLSX.utils.book_new(); XLSX.utils.book_append_sheet(wb, ws, 'Usage')
      saveAs(new Blob([XLSX.write(wb, { bookType: 'xlsx', type: 'array' REDACTED)], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' REDACTED), `usage_${Date.now()REDACTED.xlsx`)
      appStore.showSuccess('Export Success')
    REDACTED
  REDACTED catch { appStore.showError('Export Failed') REDACTED
  finally { if(exportAbortController === c) { exportAbortController = null; exporting.value = false; exportProgress.show = false REDACTED REDACTED
REDACTED

onMounted(() => { loadLogs(); loadStats() REDACTED)
onUnmounted(() => { abortController?.abort(); exportAbortController?.abort() REDACTED)
</script>
