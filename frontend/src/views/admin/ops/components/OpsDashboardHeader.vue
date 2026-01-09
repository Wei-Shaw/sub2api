<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { adminAPI REDACTED from '@/api'
import type { OpsDashboardOverview, OpsWSStatus REDACTED from '@/api/admin/ops'
import { formatNumber REDACTED from '@/utils/format'

interface Props {
  overview?: OpsDashboardOverview | null
  wsStatus: OpsWSStatus
  wsReconnectInMs?: number | null
  wsHasData?: boolean
  realTimeQps: number
  realTimeTps: number
  platform: string
  groupId: number | null
  timeRange: string
  queryMode: string
  loading: boolean
  lastUpdated: Date | null
REDACTED

interface Emits {
  (e: 'update:platform', value: string): void
  (e: 'update:group', value: number | null): void
  (e: 'update:timeRange', value: string): void
  (e: 'update:queryMode', value: string): void
  (e: 'refresh'): void
  (e: 'openRequestDetails'): void
  (e: 'openErrorDetails', kind: 'request' | 'upstream'): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t REDACTED = useI18n()

const groups = ref<Array<{ id: number; name: string; platform: string REDACTED>>([])

const platformOptions = computed(() => [
  { value: '', label: t('common.all') REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const timeRangeOptions = computed(() => [
  { value: '5m', label: '5m' REDACTED,
  { value: '30m', label: '30m' REDACTED,
  { value: '1h', label: '1h' REDACTED,
  { value: '6h', label: '6h' REDACTED,
  { value: '24h', label: '24h' REDACTED
])

const queryModeOptions = computed(() => [
  { value: 'auto', label: t('admin.ops.queryMode.auto') REDACTED,
  { value: 'raw', label: t('admin.ops.queryMode.raw') REDACTED,
  { value: 'preagg', label: t('admin.ops.queryMode.preagg') REDACTED
])

const groupOptions = computed(() => {
  const filtered = props.platform ? groups.value.filter((g) => g.platform === props.platform) : groups.value
  return [{ value: null, label: t('common.all') REDACTED, ...filtered.map((g) => ({ value: g.id, label: g.name REDACTED))]
REDACTED)

watch(
  () => props.platform,
  (newPlatform) => {
    if (!newPlatform) return
    const currentGroup = groups.value.find((g) => g.id === props.groupId)
    if (currentGroup && currentGroup.platform !== newPlatform) {
      emit('update:group', null)
    REDACTED
  REDACTED
)

onMounted(async () => {
  try {
    const list = await adminAPI.groups.getAll()
    groups.value = list.map((g) => ({ id: g.id, name: g.name, platform: g.platform REDACTED))
  REDACTED catch (e) {
    console.error('[OpsDashboardHeader] Failed to load groups', e)
    groups.value = []
  REDACTED
REDACTED)

function handlePlatformChange(val: string | number | boolean | null) {
  emit('update:platform', String(val || ''))
REDACTED

function handleGroupChange(val: string | number | boolean | null) {
  if (val === null || val === '' || typeof val === 'boolean') {
    emit('update:group', null)
    return
  REDACTED
  const id = typeof val === 'number' ? val : Number.parseInt(String(val), 10)
  emit('update:group', Number.isFinite(id) && id > 0 ? id : null)
REDACTED

function handleTimeRangeChange(val: string | number | boolean | null) {
  emit('update:timeRange', String(val || '1h'))
REDACTED

function handleQueryModeChange(val: string | number | boolean | null) {
  emit('update:queryMode', String(val || 'auto'))
REDACTED

const updatedAtLabel = computed(() => {
  if (!props.lastUpdated) return t('common.unknown')
  return props.lastUpdated.toLocaleTimeString()
REDACTED)

const totalRequestsLabel = computed(() => {
  const n = props.overview?.request_count_total ?? 0
  return formatNumber(n)
REDACTED)

const totalTokensLabel = computed(() => {
  const n = props.overview?.token_consumed ?? 0
  return formatNumber(n)
REDACTED)

const qpsLabel = computed(() => {
  const useRealtime = props.wsStatus === 'connected' && !!props.wsHasData
  const n = useRealtime ? props.realTimeQps : props.overview?.qps?.current
  if (typeof n !== 'number') return '-'
  return n.toFixed(1)
REDACTED)

const tpsLabel = computed(() => {
  const useRealtime = props.wsStatus === 'connected' && !!props.wsHasData
  const n = useRealtime ? props.realTimeTps : props.overview?.tps?.current
  if (typeof n !== 'number') return '-'
  return n.toFixed(1)
REDACTED)

const qpsPeakLabel = computed(() => {
  const n = props.overview?.qps?.peak
  if (typeof n !== 'number') return '-'
  return n.toFixed(1)
REDACTED)

const tpsPeakLabel = computed(() => {
  const n = props.overview?.tps?.peak
  if (typeof n !== 'number') return '-'
  return n.toFixed(1)
REDACTED)

const slaLabel = computed(() => {
  const v = props.overview?.sla
  if (typeof v !== 'number') return '-'
  return `${(v * 100).toFixed(3)REDACTED%`
REDACTED)

const errorRateLabel = computed(() => {
  const v = props.overview?.error_rate
  if (typeof v !== 'number') return '-'
  return `${(v * 100).toFixed(2)REDACTED%`
REDACTED)

const upstreamErrorRateLabel = computed(() => {
  const v = props.overview?.upstream_error_rate
  if (typeof v !== 'number') return '-'
  return `${(v * 100).toFixed(2)REDACTED%`
REDACTED)

const wsStatusLabel = computed(() => {
  switch (props.wsStatus) {
    case 'connected':
      return t('admin.ops.realtime.connected')
    case 'connecting':
      return t('admin.ops.realtime.connecting')
    case 'reconnecting':
      return t('admin.ops.realtime.reconnecting')
    case 'offline':
      return t('admin.ops.realtime.offline')
    case 'closed':
    default:
      return t('admin.ops.realtime.closed')
  REDACTED
REDACTED)

const wsStatusDotClass = computed(() => {
  switch (props.wsStatus) {
    case 'connected':
      return 'bg-green-500'
    case 'reconnecting':
    case 'connecting':
      return 'bg-yellow-500'
    case 'offline':
      return 'bg-orange-500'
    case 'closed':
    default:
      return 'bg-gray-400'
  REDACTED
REDACTED)

const wsReconnectHint = computed(() => {
  if (props.wsStatus !== 'reconnecting') return ''
  const delayMs = props.wsReconnectInMs ?? null
  if (typeof delayMs !== 'number' || !Number.isFinite(delayMs) || delayMs <= 0) return ''
  const sec = Math.max(1, Math.ceil(delayMs / 1000))
  return t('admin.ops.realtime.reconnectIn', { seconds: sec REDACTED)
REDACTED)
</script>

<template>
  <div class="flex flex-col gap-4 rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <!-- Top Toolbar (style aligned with docs/sub2api baseline) -->
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-100 pb-4 dark:border-dark-700">
      <div>
        <h1 class="flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
          <svg class="h-6 w-6 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
            />
          </svg>
          {{ t('admin.ops.title') REDACTEDREDACTED
        </h1>
        <div class="mt-1 flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
          <span class="flex items-center gap-1.5" :title="props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready')">
            <span class="relative flex h-2 w-2">
              <span
                class="relative inline-flex h-2 w-2 rounded-full"
                :class="props.loading ? 'bg-gray-400' : 'bg-green-500'"
              ></span>
            </span>
            {{ props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready') REDACTEDREDACTED
          </span>
          <span>·</span>
          <span>{{ t('common.refresh') REDACTEDREDACTED: {{ updatedAtLabel REDACTEDREDACTED</span>
          <span>·</span>
          <span class="flex items-center gap-1.5">
            <span class="relative flex h-2 w-2">
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="wsStatusDotClass"></span>
            </span>
            <span>{{ wsStatusLabel REDACTEDREDACTED</span>
            <span v-if="wsReconnectHint" class="text-[11px] text-gray-400">({{ wsReconnectHint REDACTEDREDACTED)</span>
          </span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-3">
        <Select
          :model-value="platform"
          :options="platformOptions"
          class="w-full sm:w-[140px]"
          @update:model-value="handlePlatformChange"
        />

        <Select
          :model-value="groupId"
          :options="groupOptions"
          class="w-full sm:w-[140px]"
          @update:model-value="handleGroupChange"
        />

        <div class="mx-1 hidden h-4 w-[1px] bg-gray-200 dark:bg-dark-700 sm:block"></div>

        <Select
          :model-value="timeRange"
          :options="timeRangeOptions"
          class="relative w-full sm:w-[150px]"
          @update:model-value="handleTimeRangeChange"
        />

        <Select
          :model-value="queryMode"
          :options="queryModeOptions"
          class="relative w-full sm:w-[170px]"
          @update:model-value="handleQueryModeChange"
        />

        <button
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-500 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="emit('refresh')"
        >
          <svg class="h-4 w-4" :class="{ 'animate-spin': loading REDACTED" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
            />
          </svg>
        </button>
      </div>
    </div>

    <!-- Placeholder section to keep header height close to baseline.
         Will be progressively filled as Milestone 6 modules land. -->
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/30">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requests') REDACTEDREDACTED</div>
        <div class="mt-2 text-xl font-black text-gray-900 dark:text-white">
          {{ totalRequestsLabel REDACTEDREDACTED
        </div>
        <div class="mt-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.tokens') REDACTEDREDACTED: {{ totalTokensLabel REDACTEDREDACTED
        </div>
      </div>
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/30">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">QPS / TPS</div>
        <div class="mt-2 flex items-end justify-between gap-3">
          <div class="text-xl font-black text-gray-900 dark:text-white">
            {{ qpsLabel REDACTEDREDACTED <span class="text-xs font-semibold text-gray-400">/</span> {{ tpsLabel REDACTEDREDACTED
          </div>
          <button
            type="button"
            class="inline-flex items-center rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
            :disabled="props.loading"
            :title="t('admin.ops.requestDetails.title')"
            @click="emit('openRequestDetails')"
          >
            {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
          </button>
        </div>
        <div class="mt-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.peak') REDACTEDREDACTED: {{ qpsPeakLabel REDACTEDREDACTED / {{ tpsPeakLabel REDACTEDREDACTED
        </div>
      </div>
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/30">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">SLA</div>
        <div class="mt-2 text-xl font-black text-gray-900 dark:text-white">
          {{ slaLabel REDACTEDREDACTED
        </div>
        <div class="mt-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.businessLimited') REDACTEDREDACTED: {{ formatNumber(props.overview?.business_limited_count ?? 0) REDACTEDREDACTED
        </div>
      </div>
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900/30">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errors') REDACTEDREDACTED</div>
        <div class="mt-2 flex items-center justify-between gap-3">
          <div class="text-xs font-semibold text-gray-700 dark:text-gray-200">
            {{ t('admin.ops.errorRate') REDACTEDREDACTED: <span class="font-mono font-bold text-gray-900 dark:text-white">{{ errorRateLabel REDACTEDREDACTED</span>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="inline-flex items-center rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
              :disabled="props.loading"
              @click="emit('openErrorDetails', 'request')"
            >
              {{ t('admin.ops.errorDetails.requestErrors') REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="inline-flex items-center rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
              :disabled="props.loading"
              @click="emit('openErrorDetails', 'upstream')"
            >
              {{ t('admin.ops.errorDetails.upstreamErrors') REDACTEDREDACTED
            </button>
          </div>
        </div>
        <div class="mt-1 text-xs font-semibold text-gray-700 dark:text-gray-200">
          {{ t('admin.ops.upstreamRate') REDACTEDREDACTED: <span class="font-mono font-bold text-gray-900 dark:text-white">{{ upstreamErrorRateLabel REDACTEDREDACTED</span>
        </div>
        <div class="mt-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          429: {{ formatNumber(props.overview?.upstream_429_count ?? 0) REDACTEDREDACTED · 529:
          {{ formatNumber(props.overview?.upstream_529_count ?? 0) REDACTEDREDACTED
        </div>
      </div>
    </div>
  </div>
</template>
