<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useMediaQuery REDACTED from '@vueuse/core'
import { useI18n REDACTED from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { opsAPI, type OpsOpenAITokenStatsResponse, type OpsOpenAITokenStatsTimeRange REDACTED from '@/api/admin/ops'
import { formatNumber REDACTED from '@/utils/format'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
REDACTED

type ViewMode = 'topn' | 'pagination'

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
REDACTED)

const { t REDACTED = useI18n()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAITokenStatsResponse | null>(null)

const timeRange = ref<OpsOpenAITokenStatsTimeRange>('30d')
const viewMode = ref<ViewMode>('topn')
const topN = ref<number>(20)
const page = ref<number>(1)
const pageSize = ref<number>(20)

const items = computed(() => response.value?.items ?? [])
const total = computed(() => response.value?.total ?? 0)
const totalPages = computed(() => {
  if (viewMode.value !== 'pagination') return 1
  const size = pageSize.value > 0 ? pageSize.value : 20
  return Math.max(1, Math.ceil(total.value / size))
REDACTED)

const timeRangeOptions = computed(() => [
  { value: '30m', label: t('admin.ops.timeRange.30m') REDACTED,
  { value: '1h', label: t('admin.ops.timeRange.1h') REDACTED,
  { value: '1d', label: t('admin.ops.timeRange.1d') REDACTED,
  { value: '15d', label: t('admin.ops.timeRange.15d') REDACTED,
  { value: '30d', label: t('admin.ops.timeRange.30d') REDACTED
])

const viewModeOptions = computed(() => [
  { value: 'topn', label: t('admin.ops.openaiTokenStats.viewModeTopN') REDACTED,
  { value: 'pagination', label: t('admin.ops.openaiTokenStats.viewModePagination') REDACTED
])

const topNOptions = computed(() => [
  { value: 10, label: 'Top 10' REDACTED,
  { value: 20, label: 'Top 20' REDACTED,
  { value: 50, label: 'Top 50' REDACTED,
  { value: 100, label: 'Top 100' REDACTED
])

const pageSizeOptions = computed(() => [
  { value: 10, label: '10' REDACTED,
  { value: 20, label: '20' REDACTED,
  { value: 50, label: '50' REDACTED,
  { value: 100, label: '100' REDACTED
])

function formatRate(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return v.toFixed(2)
REDACTED

function formatInt(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return formatNumber(Math.round(v))
REDACTED

function buildParams() {
  const params: Record<string, any> = {
    time_range: timeRange.value,
    platform: props.platformFilter || undefined,
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined
  REDACTED

  if (viewMode.value === 'topn') {
    params.top_n = topN.value
  REDACTED else {
    params.page = page.value
    params.page_size = pageSize.value
  REDACTED
  return params
REDACTED

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getOpenAITokenStats(buildParams())
    // 防御：若 total 变化导致当前页超出最大页，则回退到末页并重新拉取一次。
    if (viewMode.value === 'pagination' && page.value > totalPages.value) {
      page.value = totalPages.value
      response.value = await opsAPI.getOpenAITokenStats(buildParams())
    REDACTED
  REDACTED catch (err: any) {
    console.error('[OpsOpenAITokenStatsCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.openaiTokenStats.failedToLoad')
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

watch(
  () => ({
    timeRange: timeRange.value,
    viewMode: viewMode.value,
    topN: topN.value,
    page: page.value,
    pageSize: pageSize.value,
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken
  REDACTED),
  (next, prev) => {
    // 避免“筛选变化 -> 重置页码 -> 触发两次请求”：
    // 先只重置页码，等待下一次 watch（仅 page 变化）再发起请求。
    const filtersChanged = !prev ||
      next.timeRange !== prev.timeRange ||
      next.viewMode !== prev.viewMode ||
      next.pageSize !== prev.pageSize ||
      next.platform !== prev.platform ||
      next.groupId !== prev.groupId

    if (next.viewMode === 'pagination' && filtersChanged && next.page !== 1) {
      page.value = 1
      return
    REDACTED

    void loadData()
  REDACTED,
  { immediate: true REDACTED
)

function onPrevPage() {
  if (viewMode.value !== 'pagination') return
  if (page.value > 1) page.value -= 1
REDACTED

function onNextPage() {
  if (viewMode.value !== 'pagination') return
  if (page.value < totalPages.value) page.value += 1
REDACTED
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.openaiTokenStats.title') REDACTEDREDACTED
      </h3>
      <div class="flex flex-wrap items-center gap-2">
        <div class="w-36">
          <Select v-model="timeRange" :options="timeRangeOptions" />
        </div>
        <div class="w-36">
          <Select v-model="viewMode" :options="viewModeOptions" />
        </div>
        <div v-if="viewMode === 'topn'" class="w-28">
          <Select v-model="topN" :options="topNOptions" />
        </div>
        <template v-else>
          <div class="w-24">
            <Select v-model="pageSize" :options="pageSizeOptions" />
          </div>
          <button
            class="btn btn-secondary btn-sm"
            :disabled="loading || page <= 1"
            @click="onPrevPage"
          >
            {{ t('admin.ops.openaiTokenStats.prevPage') REDACTEDREDACTED
          </button>
          <button
            class="btn btn-secondary btn-sm"
            :disabled="loading || page >= totalPages"
            @click="onNextPage"
          >
            {{ t('admin.ops.openaiTokenStats.nextPage') REDACTEDREDACTED
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.openaiTokenStats.pageInfo', { page, total: totalPages REDACTED) REDACTEDREDACTED
          </span>
        </template>
      </div>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage REDACTEDREDACTED
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') REDACTEDREDACTED
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('common.noData')"
      :description="t('admin.ops.openaiTokenStats.empty')"
    />

    <div v-else class="space-y-3">
      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="max-h-[420px] overflow-auto">
          <div v-if="!isDesktopViewport" class="divide-y divide-gray-100 dark:divide-dark-800">
            <div v-for="row in items" :key="row.model" class="space-y-2 p-3">
              <div class="break-all text-xs font-medium text-gray-900 dark:text-gray-100">{{ row.model REDACTEDREDACTED</div>
              <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.requestCount') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatInt(row.request_count) REDACTEDREDACTED</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.avgTokensPerSec') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatRate(row.avg_tokens_per_sec) REDACTEDREDACTED</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.avgFirstTokenMs') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatRate(row.avg_first_token_ms) REDACTEDREDACTED</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.totalOutputTokens') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatInt(row.total_output_tokens) REDACTEDREDACTED</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.avgDurationMs') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatInt(row.avg_duration_ms) REDACTEDREDACTED</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.ops.openaiTokenStats.table.requestsWithFirstToken') REDACTEDREDACTED</span>
                  <span class="text-gray-700 dark:text-gray-200">{{ formatInt(row.requests_with_first_token) REDACTEDREDACTED</span>
                </div>
              </div>
            </div>
          </div>
          <table v-else class="min-w-full text-left text-xs md:text-sm">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.model') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.requestCount') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.avgTokensPerSec') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.avgFirstTokenMs') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.totalOutputTokens') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.avgDurationMs') REDACTEDREDACTED</th>
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.openaiTokenStats.table.requestsWithFirstToken') REDACTEDREDACTED</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in items"
                :key="row.model"
                class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-2 py-2 font-medium">{{ row.model REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatInt(row.request_count) REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatRate(row.avg_tokens_per_sec) REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatRate(row.avg_first_token_ms) REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatInt(row.total_output_tokens) REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatInt(row.avg_duration_ms) REDACTEDREDACTED</td>
                <td class="px-2 py-2">{{ formatInt(row.requests_with_first_token) REDACTEDREDACTED</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div v-if="viewMode === 'topn'" class="mt-3 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.openaiTokenStats.totalModels', { total REDACTED) REDACTEDREDACTED
      </div>
    </div>
  </section>
</template>
