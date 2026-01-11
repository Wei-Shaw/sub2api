<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI REDACTED from '@/api'
import type { OpsDashboardOverview, OpsWSStatus REDACTED from '@/api/admin/ops'
import type { OpsRequestDetailsPreset REDACTED from './OpsRequestDetailsModal.vue'
import { formatNumber REDACTED from '@/utils/format'

type RealtimeWindow = '1min' | '5min' | '30min' | '1h'

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
  (e: 'openRequestDetails', preset?: OpsRequestDetailsPreset): void
  (e: 'openErrorDetails', kind: 'request' | 'upstream'): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t REDACTED = useI18n()

const realtimeWindow = ref<RealtimeWindow>('1min')

const overview = computed(() => props.overview ?? null)
const systemMetrics = computed(() => overview.value?.system_metrics ?? null)

// --- Filters ---

const groups = ref<Array<{ id: number; name: string; platform: string REDACTED>>([])

const platformOptions = computed(() => [
  { value: '', label: t('common.all') REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const timeRangeOptions = computed(() => [
  { value: '5m', label: t('admin.ops.timeRange.5m') REDACTED,
  { value: '30m', label: t('admin.ops.timeRange.30m') REDACTED,
  { value: '1h', label: t('admin.ops.timeRange.1h') REDACTED,
  { value: '6h', label: t('admin.ops.timeRange.6h') REDACTED,
  { value: '24h', label: t('admin.ops.timeRange.24h') REDACTED
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

function openDetails(preset?: OpsRequestDetailsPreset) {
  emit('openRequestDetails', preset)
REDACTED

function openErrorDetails(kind: 'request' | 'upstream') {
  emit('openErrorDetails', kind)
REDACTED

const updatedAtLabel = computed(() => {
  if (!props.lastUpdated) return t('common.unknown')
  return props.lastUpdated.toLocaleTimeString()
REDACTED)

// --- Color coding for latency/TTFT ---
function getLatencyColor(ms: number | null | undefined): string {
  if (ms == null) return 'text-gray-900 dark:text-white'
  if (ms < 500) return 'text-green-600 dark:text-green-400'
  if (ms < 1000) return 'text-yellow-600 dark:text-yellow-400'
  if (ms < 2000) return 'text-orange-600 dark:text-orange-400'
  return 'text-red-600 dark:text-red-400'
REDACTED

// --- Realtime / Overview labels ---

const totalRequestsLabel = computed(() => formatNumber(overview.value?.request_count_total ?? 0))
const totalTokensLabel = computed(() => formatNumber(overview.value?.token_consumed ?? 0))

const displayRealTimeQps = computed(() => {
  const ov = overview.value
  if (!ov) return 0
  const useRealtime = props.wsStatus === 'connected' && !!props.wsHasData
  const v = useRealtime ? props.realTimeQps : ov.qps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
REDACTED)

const displayRealTimeTps = computed(() => {
  const ov = overview.value
  if (!ov) return 0
  const useRealtime = props.wsStatus === 'connected' && !!props.wsHasData
  const v = useRealtime ? props.realTimeTps : ov.tps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
REDACTED)

// Sparkline history (keep last 60 data points)
const qpsHistory = ref<number[]>([])
const tpsHistory = ref<number[]>([])
const MAX_HISTORY_POINTS = 60

watch([displayRealTimeQps, displayRealTimeTps], ([newQps, newTps]) => {
  // Add new data points
  qpsHistory.value.push(newQps)
  tpsHistory.value.push(newTps)

  // Keep only last N points
  if (qpsHistory.value.length > MAX_HISTORY_POINTS) {
    qpsHistory.value.shift()
  REDACTED
  if (tpsHistory.value.length > MAX_HISTORY_POINTS) {
    tpsHistory.value.shift()
  REDACTED
REDACTED)

const qpsPeakLabel = computed(() => {
  const v = overview.value?.qps?.peak
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
REDACTED)

const tpsPeakLabel = computed(() => {
  const v = overview.value?.tps?.peak
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
REDACTED)

const qpsAvgLabel = computed(() => {
  const v = overview.value?.qps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
REDACTED)

const tpsAvgLabel = computed(() => {
  const v = overview.value?.tps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
REDACTED)

const slaPercent = computed(() => {
  const v = overview.value?.sla
  if (typeof v !== 'number') return null
  return v * 100
REDACTED)

const errorRatePercent = computed(() => {
  const v = overview.value?.error_rate
  if (typeof v !== 'number') return null
  return v * 100
REDACTED)

const upstreamErrorRatePercent = computed(() => {
  const v = overview.value?.upstream_error_rate
  if (typeof v !== 'number') return null
  return v * 100
REDACTED)

const durationP99Ms = computed(() => overview.value?.duration?.p99_ms ?? null)
const durationP95Ms = computed(() => overview.value?.duration?.p95_ms ?? null)
const durationP90Ms = computed(() => overview.value?.duration?.p90_ms ?? null)
const durationP50Ms = computed(() => overview.value?.duration?.p50_ms ?? null)
const durationAvgMs = computed(() => overview.value?.duration?.avg_ms ?? null)
const durationMaxMs = computed(() => overview.value?.duration?.max_ms ?? null)

const ttftP99Ms = computed(() => overview.value?.ttft?.p99_ms ?? null)
const ttftP95Ms = computed(() => overview.value?.ttft?.p95_ms ?? null)
const ttftP90Ms = computed(() => overview.value?.ttft?.p90_ms ?? null)
const ttftP50Ms = computed(() => overview.value?.ttft?.p50_ms ?? null)
const ttftAvgMs = computed(() => overview.value?.ttft?.avg_ms ?? null)
const ttftMaxMs = computed(() => overview.value?.ttft?.max_ms ?? null)

// --- WebSocket status ---

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

// --- Health Score & Diagnosis (primary) ---

const isSystemIdle = computed(() => {
  const ov = overview.value
  if (!ov) return true
  const qps = props.wsStatus === 'connected' && props.wsHasData ? props.realTimeQps : ov.qps?.current
  const errorRate = ov.error_rate ?? 0
  return (qps ?? 0) === 0 && errorRate === 0
REDACTED)

const healthScoreValue = computed<number | null>(() => {
  const v = overview.value?.health_score
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const healthScoreColor = computed(() => {
  if (isSystemIdle.value) return '#9ca3af' // gray-400
  const score = healthScoreValue.value
  if (score == null) return '#9ca3af'
  if (score >= 90) return '#10b981' // green
  if (score >= 60) return '#f59e0b' // yellow
  return '#ef4444' // red
REDACTED)

const healthScoreClass = computed(() => {
  if (isSystemIdle.value) return 'text-gray-400'
  const score = healthScoreValue.value
  if (score == null) return 'text-gray-400'
  if (score >= 90) return 'text-green-500'
  if (score >= 60) return 'text-yellow-500'
  return 'text-red-500'
REDACTED)

const circleSize = 100
const strokeWidth = 8
const radius = (circleSize - strokeWidth) / 2
const circumference = 2 * Math.PI * radius
const dashOffset = computed(() => {
  if (isSystemIdle.value) return 0
  if (healthScoreValue.value == null) return 0
  const score = Math.max(0, Math.min(100, healthScoreValue.value))
  return circumference - (score / 100) * circumference
REDACTED)

interface DiagnosisItem {
  type: 'critical' | 'warning' | 'info'
  message: string
  impact: string
REDACTED

const diagnosisReport = computed<DiagnosisItem[]>(() => {
  const ov = overview.value
  if (!ov) return []

  const report: DiagnosisItem[] = []

  if (isSystemIdle.value) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.idle'),
      impact: t('admin.ops.diagnosis.idleImpact')
    REDACTED)
    return report
  REDACTED

  const upstreamRatePct = (ov.upstream_error_rate ?? 0) * 100
  if (upstreamRatePct > 10) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.upstreamCritical', { rate: upstreamRatePct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.upstreamCriticalImpact')
    REDACTED)
  REDACTED else if (upstreamRatePct > 3) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.upstreamHigh', { rate: upstreamRatePct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.upstreamHighImpact')
    REDACTED)
  REDACTED

  const slaPct = (ov.sla ?? 0) * 100
  if (slaPct < 90) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.slaCritical', { sla: slaPct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.slaCriticalImpact')
    REDACTED)
  REDACTED else if (slaPct < 98) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.slaLow', { sla: slaPct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.slaLowImpact')
    REDACTED)
  REDACTED

  const errorPct = (ov.error_rate ?? 0) * 100
  if (errorPct > 5) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.errorHigh', { rate: errorPct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.errorHighImpact')
    REDACTED)
  REDACTED else if (errorPct > 1) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.errorElevated', { rate: errorPct.toFixed(2) REDACTED),
      impact: t('admin.ops.diagnosis.errorElevatedImpact')
    REDACTED)
  REDACTED

  if (healthScoreValue.value != null) {
    if (healthScoreValue.value < 60) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.healthCritical', { score: healthScoreValue.value REDACTED),
        impact: t('admin.ops.diagnosis.healthCriticalImpact')
      REDACTED)
    REDACTED else if (healthScoreValue.value < 90) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.healthLow', { score: healthScoreValue.value REDACTED),
        impact: t('admin.ops.diagnosis.healthLowImpact')
      REDACTED)
    REDACTED
  REDACTED

  if (report.length === 0) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.healthy'),
      impact: t('admin.ops.diagnosis.healthyImpact')
    REDACTED)
  REDACTED

  return report
REDACTED)

// --- System health (secondary) ---

function formatTimeShort(ts?: string | null): string {
  if (!ts) return '-'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleTimeString()
REDACTED

const cpuPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.cpu_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const cpuPercentClass = computed(() => {
  const v = cpuPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 80) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
REDACTED)

const memPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.memory_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const memPercentClass = computed(() => {
  const v = memPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 85) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
REDACTED)

const dbConnActiveValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_active
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const dbConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const dbConnWaitingValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_waiting
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const dbConnOpenValue = computed<number | null>(() => {
  if (dbConnActiveValue.value == null || dbConnIdleValue.value == null) return null
  return dbConnActiveValue.value + dbConnIdleValue.value
REDACTED)

const dbMaxOpenConnsValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_max_open_conns
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const dbUsagePercent = computed<number | null>(() => {
  if (dbConnOpenValue.value == null || dbMaxOpenConnsValue.value == null || dbMaxOpenConnsValue.value <= 0) return null
  return Math.min(100, Math.max(0, (dbConnOpenValue.value / dbMaxOpenConnsValue.value) * 100))
REDACTED)

const dbMiddleLabel = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'FAIL'
  if (dbUsagePercent.value != null) return `${dbUsagePercent.value.toFixed(0)REDACTED%`
  if (systemMetrics.value?.db_ok === true) return t('admin.ops.ok')
  return t('admin.ops.noData')
REDACTED)

const dbMiddleClass = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'text-rose-600 dark:text-rose-400'
  if (dbUsagePercent.value != null) {
    if (dbUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (dbUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  REDACTED
  if (systemMetrics.value?.db_ok === true) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
REDACTED)

const redisConnTotalValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_total
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const redisConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const redisConnActiveValue = computed<number | null>(() => {
  if (redisConnTotalValue.value == null || redisConnIdleValue.value == null) return null
  return Math.max(redisConnTotalValue.value - redisConnIdleValue.value, 0)
REDACTED)

const redisPoolSizeValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_pool_size
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const redisUsagePercent = computed<number | null>(() => {
  if (redisConnTotalValue.value == null || redisPoolSizeValue.value == null || redisPoolSizeValue.value <= 0) return null
  return Math.min(100, Math.max(0, (redisConnTotalValue.value / redisPoolSizeValue.value) * 100))
REDACTED)

const redisMiddleLabel = computed(() => {
  if (systemMetrics.value?.redis_ok === false) return 'FAIL'
  if (redisUsagePercent.value != null) return `${redisUsagePercent.value.toFixed(0)REDACTED%`
  if (systemMetrics.value?.redis_ok === true) return t('admin.ops.ok')
  return t('admin.ops.noData')
REDACTED)

const redisMiddleClass = computed(() => {
  if (systemMetrics.value?.redis_ok === false) return 'text-rose-600 dark:text-rose-400'
  if (redisUsagePercent.value != null) {
    if (redisUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (redisUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  REDACTED
  if (systemMetrics.value?.redis_ok === true) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
REDACTED)

const goroutineCountValue = computed<number | null>(() => {
  const v = systemMetrics.value?.goroutine_count
  return typeof v === 'number' && Number.isFinite(v) ? v : null
REDACTED)

const goroutinesWarnThreshold = 8_000
const goroutinesCriticalThreshold = 15_000

const goroutineStatus = computed<'ok' | 'warning' | 'critical' | 'unknown'>(() => {
  const n = goroutineCountValue.value
  if (n == null) return 'unknown'
  if (n >= goroutinesCriticalThreshold) return 'critical'
  if (n >= goroutinesWarnThreshold) return 'warning'
  return 'ok'
REDACTED)

const goroutineStatusLabel = computed(() => {
  switch (goroutineStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warning':
      return t('common.warning')
    case 'critical':
      return t('common.critical')
    default:
      return t('admin.ops.noData')
  REDACTED
REDACTED)

const goroutineStatusClass = computed(() => {
  switch (goroutineStatus.value) {
    case 'ok':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warning':
      return 'text-yellow-600 dark:text-yellow-400'
    case 'critical':
      return 'text-rose-600 dark:text-rose-400'
    default:
      return 'text-gray-900 dark:text-white'
  REDACTED
REDACTED)

const jobHeartbeats = computed(() => overview.value?.job_heartbeats ?? [])

const jobsStatus = computed<'ok' | 'warn' | 'unknown'>(() => {
  const list = jobHeartbeats.value
  if (!list.length) return 'unknown'
  for (const hb of list) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) return 'warn'
  REDACTED
  return 'ok'
REDACTED)

const jobsWarnCount = computed(() => {
  let warn = 0
  for (const hb of jobHeartbeats.value) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) warn++
  REDACTED
  return warn
REDACTED)

const jobsStatusLabel = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warn':
      return t('common.warning')
    default:
      return t('admin.ops.noData')
  REDACTED
REDACTED)

const jobsStatusClass = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warn':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-gray-900 dark:text-white'
  REDACTED
REDACTED)

const showJobsDetails = ref(false)

function openJobsDetails() {
  showJobsDetails.value = true
REDACTED
</script>

<template>
  <div class="flex flex-col gap-4 rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <!-- Top Toolbar -->
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
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="props.loading ? 'bg-gray-400' : 'bg-green-500'"></span>
            </span>
            {{ props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready') REDACTEDREDACTED
          </span>

          <span>·</span>
          <span>{{ t('common.refresh') REDACTEDREDACTED: {{ updatedAtLabel REDACTEDREDACTED</span>
          <span>·</span>

          <span class="flex items-center gap-1.5">
            <span class="relative flex h-2 w-2">
              <span
                v-if="wsStatus === 'connected'"
                class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75"
              ></span>
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
          class="w-full sm:w-[160px]"
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

    <div v-if="overview" class="grid grid-cols-1 gap-6 lg:grid-cols-12">
      <!-- Left: Health + Realtime -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900 lg:col-span-5">
        <div class="grid grid-cols-1 gap-6 md:grid-cols-[200px_1fr] md:items-center">
          <!-- 1) Health Score -->
          <div
            class="group relative flex cursor-pointer flex-col items-center justify-center rounded-xl py-2 transition-all hover:bg-white/60 dark:hover:bg-dark-800/60 md:border-r md:border-gray-200 md:pr-6 dark:md:border-dark-700"
          >
            <!-- Diagnosis Popover (hover) -->
            <div
              class="pointer-events-none absolute left-1/2 top-full z-50 mt-2 w-72 -translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100 md:left-full md:top-0 md:ml-2 md:mt-0 md:translate-x-0"
            >
              <div class="rounded-xl bg-white p-4 shadow-xl ring-1 ring-black/5 dark:bg-gray-800 dark:ring-white/10">
                <h4 class="mb-3 border-b border-gray-100 pb-2 text-sm font-bold text-gray-900 dark:border-gray-700 dark:text-white">
                  🧠 {{ t('admin.ops.diagnosis.title') REDACTEDREDACTED
                </h4>

                <div class="space-y-3">
                  <div v-for="(item, idx) in diagnosisReport" :key="idx" class="flex gap-3">
                    <div class="mt-0.5 shrink-0">
                      <svg v-if="item.type === 'critical'" class="h-4 w-4 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                        <path
                          fill-rule="evenodd"
                          d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                          clip-rule="evenodd"
                        />
                      </svg>
                      <svg v-else-if="item.type === 'warning'" class="h-4 w-4 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
                        <path
                          fill-rule="evenodd"
                          d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                          clip-rule="evenodd"
                        />
                      </svg>
                      <svg v-else class="h-4 w-4 text-blue-500" fill="currentColor" viewBox="0 0 20 20">
                        <path
                          fill-rule="evenodd"
                          d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 100 2 1 1 0 000-2zm-1 3a1 1 0 012 0v4a1 1 0 11-2 0v-4z"
                          clip-rule="evenodd"
                        />
                      </svg>
                    </div>
                    <div>
                      <div class="text-xs font-semibold text-gray-900 dark:text-white">{{ item.message REDACTEDREDACTED</div>
                      <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">{{ item.impact REDACTEDREDACTED</div>
                    </div>
                  </div>
                </div>

                <div class="mt-3 border-t border-gray-100 pt-2 text-[10px] text-gray-400 dark:border-gray-700">
                  {{ t('admin.ops.diagnosis.footer') REDACTEDREDACTED
                </div>
              </div>
            </div>

            <div class="relative flex items-center justify-center">
              <svg :width="circleSize" :height="circleSize" class="-rotate-90 transform">
                <circle
                  :cx="circleSize / 2"
                  :cy="circleSize / 2"
                  :r="radius"
                  :stroke-width="strokeWidth"
                  fill="transparent"
                  class="text-gray-200 dark:text-dark-700"
                  stroke="currentColor"
                />
                <circle
                  :cx="circleSize / 2"
                  :cy="circleSize / 2"
                  :r="radius"
                  :stroke-width="strokeWidth"
                  fill="transparent"
                  :stroke="healthScoreColor"
                  stroke-linecap="round"
                  :stroke-dasharray="circumference"
                  :stroke-dashoffset="dashOffset"
                  class="transition-all duration-1000 ease-out"
                />
              </svg>

              <div class="absolute flex flex-col items-center">
                <span class="text-3xl font-black" :class="healthScoreClass">
                  {{ isSystemIdle ? t('admin.ops.idleStatus') : (overview.health_score ?? '--') REDACTEDREDACTED
                </span>
                <span class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.health') REDACTEDREDACTED</span>
              </div>
            </div>

            <div class="mt-4 text-center">
              <div class="flex items-center justify-center gap-1 text-xs font-medium text-gray-500">
                {{ t('admin.ops.healthCondition') REDACTEDREDACTED
                <HelpTooltip :content="t('admin.ops.healthHelp')" />
              </div>
              <div class="mt-1 text-xs font-bold" :class="healthScoreClass">
                {{
                  isSystemIdle
                    ? t('admin.ops.idleStatus')
                    : typeof overview.health_score === 'number' && overview.health_score >= 90
                      ? t('admin.ops.healthyStatus')
                      : t('admin.ops.riskyStatus')
                REDACTEDREDACTED
              </div>
            </div>
          </div>

          <!-- 2) Realtime Traffic -->
          <div class="flex flex-col justify-center py-2">
            <div class="mb-3 flex items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <div class="relative flex h-3 w-3">
                  <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75"></span>
                  <span class="relative inline-flex h-3 w-3 rounded-full bg-blue-500"></span>
                </div>
                <h3 class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.realtime.title') REDACTEDREDACTED</h3>
                <HelpTooltip :content="t('admin.ops.tooltips.qps')" />
              </div>

              <!-- Time Window Selector -->
              <div class="flex gap-1">
                <button
                  v-for="window in (['1min', '5min', '30min', '1h'] as RealtimeWindow[])"
                  :key="window"
                  type="button"
                  class="rounded px-2 py-0.5 text-[10px] font-bold transition-colors"
                  :class="realtimeWindow === window
                    ? 'bg-blue-500 text-white'
                    : 'bg-gray-200 text-gray-600 hover:bg-gray-300 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'"
                  @click="realtimeWindow = window"
                >
                  {{ window REDACTEDREDACTED
                </button>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <!-- Current QPS/TPS -->
              <div>
                <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.current') REDACTEDREDACTED</div>
                <div class="mt-1 flex items-baseline gap-2">
                  <span class="text-2xl font-black text-gray-900 dark:text-white">{{ displayRealTimeQps.toFixed(1) REDACTEDREDACTED</span>
                  <span class="text-xs font-bold text-gray-500">QPS</span>
                </div>
                <div class="mt-0.5 text-xs font-medium text-gray-500">
                  TPS: {{ displayRealTimeTps.toFixed(1) REDACTEDREDACTED
                </div>
              </div>

              <!-- Peak QPS/TPS -->
              <div>
                <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.peak') REDACTEDREDACTED</div>
                <div class="mt-1 flex items-baseline gap-2">
                  <span class="text-2xl font-black text-gray-900 dark:text-white">{{ qpsPeakLabel REDACTEDREDACTED</span>
                  <span class="text-xs font-bold text-gray-500">QPS</span>
                </div>
                <div class="mt-0.5 text-xs font-medium text-gray-500">
                  TPS: {{ tpsPeakLabel REDACTEDREDACTED
                </div>
              </div>

              <!-- Average QPS/TPS with Animated Pulse Line -->
              <div class="col-span-2">
                <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.average') REDACTEDREDACTED</div>
                <div class="mt-1 flex items-center gap-4 text-sm font-medium text-gray-600 dark:text-gray-400">
                  <span>QPS: <span class="font-bold">{{ qpsAvgLabel REDACTEDREDACTED</span></span>
                  <span class="h-1 w-1 rounded-full bg-gray-300 dark:bg-gray-600"></span>
                  <span>TPS: <span class="font-bold">{{ tpsAvgLabel REDACTEDREDACTED</span></span>
                </div>

                <!-- Animated Pulse Line (Heart Beat Animation) -->
                <div class="mt-2 h-8 w-full overflow-hidden opacity-50">
                  <svg class="h-full w-full" viewBox="0 0 280 32" preserveAspectRatio="none">
                    <path
                      d="M0 16 Q 20 16, 40 16 T 80 16 T 120 10 T 160 22 T 200 16 T 240 16 T 280 16"
                      fill="none"
                      stroke="#3b82f6"
                      stroke-width="2"
                      vector-effect="non-scaling-stroke"
                    >
                      <animate
                        attributeName="d"
                        dur="2s"
                        repeatCount="indefinite"
                        values="M0 16 Q 20 16, 40 16 T 80 16 T 120 10 T 160 22 T 200 16 T 240 16 T 280 16;
                                M0 16 Q 20 16, 40 16 T 80 16 T 120 16 T 160 16 T 200 10 T 240 22 T 280 16;
                                M0 16 Q 20 16, 40 16 T 80 16 T 120 16 T 160 16 T 200 16 T 240 16 T 280 16"
                        keyTimes="0;0.5;1"
                      />
                    </path>
                  </svg>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Right: 6 cards (3 cols x 2 rows) -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:col-span-7 lg:grid-cols-3">
        <!-- Card 1: Total Requests -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.totalRequests') REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.ops.tooltips.totalRequests')" />
            </div>
            <button
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.requestDetails.title') REDACTED)"
            >
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 space-y-2 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.requests') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ totalRequestsLabel REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.tokens') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ totalTokensLabel REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgQps') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ qpsAvgLabel REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgTps') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ tpsAvgLabel REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgLatency') REDACTEDREDACTED:</span>
              <span class="font-bold" :class="getLatencyColor(durationAvgMs)">{{ durationAvgMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgTtft') REDACTEDREDACTED:</span>
              <span class="font-bold" :class="getLatencyColor(ttftAvgMs)">{{ ttftAvgMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
          </div>
        </div>

        <!-- Card 2: SLA -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-[10px] font-bold uppercase text-gray-400">SLA</span>
              <HelpTooltip :content="t('admin.ops.tooltips.sla')" />
              <span class="h-1.5 w-1.5 rounded-full" :class="(slaPercent ?? 0) >= 99.5 ? 'bg-green-500' : 'bg-yellow-500'"></span>
            </div>
            <button
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.requestDetails.title') REDACTED)"
            >
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 text-3xl font-black text-gray-900 dark:text-white">
            {{ slaPercent == null ? '-' : `${slaPercent.toFixed(3)REDACTED%` REDACTEDREDACTED
          </div>
          <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full bg-green-500 transition-all" :style="{ width: `${Math.max((slaPercent ?? 0) - 90, 0) * 10REDACTED%` REDACTED"></div>
          </div>
          <div class="mt-3 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.exceptions') REDACTEDREDACTED:</span>
              <span class="font-bold text-red-600 dark:text-red-400">{{ formatNumber((overview.request_count_sla ?? 0) - (overview.success_count ?? 0)) REDACTEDREDACTED</span>
            </div>
          </div>
        </div>

        <!-- Card 3: Latency (Duration) -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.latencyDuration') REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.ops.tooltips.latency')" />
            </div>
            <button
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.latencyDuration'), sort: 'duration_desc', min_duration_ms: Math.max(Number(durationP99Ms ?? 0), 0) REDACTED)"
            >
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 flex items-baseline gap-2">
            <div class="text-3xl font-black" :class="getLatencyColor(durationP99Ms)">
              {{ durationP99Ms ?? '-' REDACTEDREDACTED
            </div>
            <span class="text-xs font-bold text-gray-400">ms (P99)</span>
          </div>
          <div class="mt-3 grid grid-cols-3 gap-x-3 gap-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">P95:</span>
              <span class="font-bold" :class="getLatencyColor(durationP95Ms)">{{ durationP95Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">P90:</span>
              <span class="font-bold" :class="getLatencyColor(durationP90Ms)">{{ durationP90Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">P50:</span>
              <span class="font-bold" :class="getLatencyColor(durationP50Ms)">{{ durationP50Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">Avg:</span>
              <span class="font-bold" :class="getLatencyColor(durationAvgMs)">{{ durationAvgMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">Max:</span>
              <span class="font-bold" :class="getLatencyColor(durationMaxMs)">{{ durationMaxMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
          </div>
        </div>

        <!-- Card 4: TTFT -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">TTFT</span>
              <HelpTooltip :content="t('admin.ops.tooltips.ttft')" />
            </div>
            <button
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: 'TTFT' REDACTED)"
            >
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 flex items-baseline gap-2">
            <div class="text-3xl font-black" :class="getLatencyColor(ttftP99Ms)">
              {{ ttftP99Ms ?? '-' REDACTEDREDACTED
            </div>
            <span class="text-xs font-bold text-gray-400">ms (P99)</span>
          </div>
          <div class="mt-3 grid grid-cols-3 gap-x-3 gap-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">P95:</span>
              <span class="font-bold" :class="getLatencyColor(ttftP95Ms)">{{ ttftP95Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">P90:</span>
              <span class="font-bold" :class="getLatencyColor(ttftP90Ms)">{{ ttftP90Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">P50:</span>
              <span class="font-bold" :class="getLatencyColor(ttftP50Ms)">{{ ttftP50Ms ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">Avg:</span>
              <span class="font-bold" :class="getLatencyColor(ttftAvgMs)">{{ ttftAvgMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">Max:</span>
              <span class="font-bold" :class="getLatencyColor(ttftMaxMs)">{{ ttftMaxMs ?? '-' REDACTEDREDACTEDms</span>
            </div>
          </div>
        </div>

        <!-- Card 5: Request Errors -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.requestErrors') REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.ops.tooltips.errors')" />
            </div>
            <button class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('request')">
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 text-3xl font-black" :class="(errorRatePercent ?? 0) > 5 ? 'text-red-500' : 'text-gray-900 dark:text-white'">
            {{ errorRatePercent == null ? '-' : `${errorRatePercent.toFixed(2)REDACTED%` REDACTEDREDACTED
          </div>
          <div class="mt-3 space-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.errorCount') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.error_count_sla ?? 0) REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.businessLimited') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.business_limited_count ?? 0) REDACTEDREDACTED</span>
            </div>
          </div>
        </div>

        <!-- Card 6: Upstream Errors -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.upstreamErrors') REDACTEDREDACTED</span>
              <HelpTooltip :content="t('admin.ops.tooltips.upstreamErrors')" />
            </div>
            <button class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('upstream')">
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>
          <div class="mt-2 text-3xl font-black" :class="(upstreamErrorRatePercent ?? 0) > 5 ? 'text-red-500' : 'text-gray-900 dark:text-white'">
            {{ upstreamErrorRatePercent == null ? '-' : `${upstreamErrorRatePercent.toFixed(2)REDACTED%` REDACTEDREDACTED
          </div>
          <div class="mt-3 space-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.errorCountExcl429529') REDACTEDREDACTED:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.upstream_error_count_excl_429_529 ?? 0) REDACTEDREDACTED</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">429/529:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber((overview.upstream_429_count ?? 0) + (overview.upstream_529_count ?? 0)) REDACTEDREDACTED</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Integrated: System health (cards) -->
    <div v-if="overview" class="mt-2 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
        <span v-if="systemMetrics">
          {{ t('admin.ops.collectedAt') REDACTEDREDACTED {{ formatTimeShort(systemMetrics.created_at) REDACTEDREDACTED
          ({{ t('admin.ops.window') REDACTEDREDACTED {{ systemMetrics.window_minutes REDACTEDREDACTEDm)
        </span>
        <span v-else>{{ t('admin.ops.noSystemMetrics') REDACTEDREDACTED</span>
      </div>

      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <!-- CPU -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">CPU</div>
            <HelpTooltip :content="t('admin.ops.tooltips.cpu')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="cpuPercentClass">
            {{ cpuPercentValue == null ? '-' : `${cpuPercentValue.toFixed(1)REDACTED%` REDACTEDREDACTED
          </div>
          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('common.warning') REDACTEDREDACTED 80% · {{ t('common.critical') REDACTEDREDACTED 95%
          </div>
        </div>

        <!-- MEM -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">MEM</div>
            <HelpTooltip :content="t('admin.ops.tooltips.memory')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="memPercentClass">
            {{ memPercentValue == null ? '-' : `${memPercentValue.toFixed(1)REDACTED%` REDACTEDREDACTED
          </div>
          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{
              systemMetrics?.memory_used_mb == null || systemMetrics?.memory_total_mb == null
                ? '-'
                : `${formatNumber(systemMetrics.memory_used_mb)REDACTED / ${formatNumber(systemMetrics.memory_total_mb)REDACTED MB`
            REDACTEDREDACTED
          </div>
        </div>

        <!-- DB -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">DB</div>
            <HelpTooltip :content="t('admin.ops.tooltips.db')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="dbMiddleClass">
            {{ dbMiddleLabel REDACTEDREDACTED
          </div>
          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.conns') REDACTEDREDACTED {{ dbConnOpenValue ?? '-' REDACTEDREDACTED / {{ dbMaxOpenConnsValue ?? '-' REDACTEDREDACTED
            · {{ t('admin.ops.active') REDACTEDREDACTED {{ dbConnActiveValue ?? '-' REDACTEDREDACTED
            · {{ t('admin.ops.idle') REDACTEDREDACTED {{ dbConnIdleValue ?? '-' REDACTEDREDACTED
            <span v-if="dbConnWaitingValue != null"> · {{ t('admin.ops.waiting') REDACTEDREDACTED {{ dbConnWaitingValue REDACTEDREDACTED </span>
          </div>
        </div>

        <!-- Redis -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">Redis</div>
            <HelpTooltip :content="t('admin.ops.tooltips.redis')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="redisMiddleClass">
            {{ redisMiddleLabel REDACTEDREDACTED
          </div>
          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.conns') REDACTEDREDACTED {{ redisConnTotalValue ?? '-' REDACTEDREDACTED / {{ redisPoolSizeValue ?? '-' REDACTEDREDACTED
            <span v-if="redisConnActiveValue != null"> · {{ t('admin.ops.active') REDACTEDREDACTED {{ redisConnActiveValue REDACTEDREDACTED </span>
            <span v-if="redisConnIdleValue != null"> · {{ t('admin.ops.idle') REDACTEDREDACTED {{ redisConnIdleValue REDACTEDREDACTED </span>
          </div>
        </div>

        <!-- Goroutines -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.goroutines') REDACTEDREDACTED</div>
            <HelpTooltip :content="t('admin.ops.tooltips.goroutines')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="goroutineStatusClass">
            {{ goroutineStatusLabel REDACTEDREDACTED
          </div>
          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.current') REDACTEDREDACTED <span class="font-mono">{{ goroutineCountValue ?? '-' REDACTEDREDACTED</span>
            · {{ t('common.warning') REDACTEDREDACTED <span class="font-mono">{{ goroutinesWarnThreshold REDACTEDREDACTED</span>
            · {{ t('common.critical') REDACTEDREDACTED <span class="font-mono">{{ goroutinesCriticalThreshold REDACTEDREDACTED</span>
            <span v-if="systemMetrics?.concurrency_queue_depth != null">
              · {{ t('admin.ops.queue') REDACTEDREDACTED <span class="font-mono">{{ systemMetrics.concurrency_queue_depth REDACTEDREDACTED</span>
            </span>
          </div>
        </div>

        <!-- Jobs -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1">
              <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.jobs') REDACTEDREDACTED</div>
              <HelpTooltip :content="t('admin.ops.tooltips.jobs')" />
            </div>
            <button class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openJobsDetails">
              {{ t('admin.ops.requestDetails.details') REDACTEDREDACTED
            </button>
          </div>

          <div class="mt-1 text-lg font-black" :class="jobsStatusClass">
            {{ jobsStatusLabel REDACTEDREDACTED
          </div>

          <div class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('common.total') REDACTEDREDACTED <span class="font-mono">{{ jobHeartbeats.length REDACTEDREDACTED</span>
            · {{ t('common.warning') REDACTEDREDACTED <span class="font-mono">{{ jobsWarnCount REDACTEDREDACTED</span>
          </div>
        </div>
      </div>
    </div>

    <BaseDialog :show="showJobsDetails" :title="t('admin.ops.jobs')" width="wide" @close="showJobsDetails = false">
      <div v-if="!jobHeartbeats.length" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.noData') REDACTEDREDACTED
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="hb in jobHeartbeats"
          :key="hb.job_name"
          class="rounded-xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ hb.job_name REDACTEDREDACTED</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ formatTimeShort(hb.updated_at) REDACTEDREDACTED</div>
          </div>

          <div class="mt-2 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
            <div>
              {{ t('admin.ops.lastSuccess') REDACTEDREDACTED <span class="font-mono">{{ formatTimeShort(hb.last_success_at) REDACTEDREDACTED</span>
            </div>
            <div>
              {{ t('admin.ops.lastError') REDACTEDREDACTED <span class="font-mono">{{ formatTimeShort(hb.last_error_at) REDACTEDREDACTED</span>
            </div>
          </div>

          <div
            v-if="hb.last_error"
            class="mt-3 rounded-lg bg-rose-50 p-2 text-xs text-rose-700 dark:bg-rose-900/20 dark:text-rose-300"
          >
            {{ hb.last_error REDACTEDREDACTED
          </div>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>
