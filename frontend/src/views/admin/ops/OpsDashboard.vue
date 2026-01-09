<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <div
        v-if="errorMessage"
        class="rounded-2xl bg-red-50 p-4 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400"
      >
        {{ errorMessage REDACTEDREDACTED
      </div>

      <OpsDashboardSkeleton v-if="loading && !hasLoadedOnce" />

      <OpsDashboardHeader
        v-else-if="opsEnabled"
        :overview="overview"
        :ws-status="wsStatus"
        :ws-reconnect-in-ms="wsReconnectInMs"
        :ws-has-data="wsHasData"
        :real-time-qps="realTimeQPS"
        :real-time-tps="realTimeTPS"
        :platform="platform"
        :group-id="groupId"
        :time-range="timeRange"
        :query-mode="queryMode"
        :loading="loading"
        :last-updated="lastUpdated"
        @update:time-range="onTimeRangeChange"
        @update:platform="onPlatformChange"
        @update:group="onGroupChange"
        @update:query-mode="onQueryModeChange"
        @refresh="fetchData"
        @open-request-details="handleOpenRequestDetails"
        @open-error-details="openErrorDetails"
      />

      <!-- Row: Concurrency + Throughput -->
      <div v-if="opsEnabled && !(loading && !hasLoadedOnce)" class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div class="lg:col-span-1 min-h-[360px]">
          <OpsConcurrencyCard :platform-filter="platform" :group-id-filter="groupId" />
        </div>
        <div class="lg:col-span-2 min-h-[360px]">
          <OpsThroughputTrendChart
            :points="throughputTrend?.points ?? []"
            :by-platform="throughputTrend?.by_platform ?? []"
            :top-groups="throughputTrend?.top_groups ?? []"
            :loading="loadingTrend"
            :time-range="timeRange"
            @select-platform="handleThroughputSelectPlatform"
            @select-group="handleThroughputSelectGroup"
            @open-details="handleOpenRequestDetails"
          />
        </div>
      </div>

      <!-- Row: Visual Analysis (baseline 3-up grid) -->
      <div v-if="opsEnabled && !(loading && !hasLoadedOnce)" class="grid grid-cols-1 gap-6 md:grid-cols-3">
        <OpsLatencyChart :latency-data="latencyHistogram" :loading="loadingLatency" />
        <OpsErrorDistributionChart
          :data="errorDistribution"
          :loading="loadingErrorDistribution"
          @open-details="openErrorDetails('request')"
        />
        <OpsErrorTrendChart
          :points="errorTrend?.points ?? []"
          :loading="loadingErrorTrend"
          :time-range="timeRange"
          @open-request-errors="openErrorDetails('request')"
          @open-upstream-errors="openErrorDetails('upstream')"
        />
      </div>

      <!-- Alert Events -->
      <OpsAlertEventsCard v-if="opsEnabled && !(loading && !hasLoadedOnce)" />

      <OpsErrorDetailsModal
        :show="showErrorDetails"
        :time-range="timeRange"
        :platform="platform"
        :group-id="groupId"
        :error-type="errorDetailsType"
        @update:show="showErrorDetails = $event"
        @openErrorDetail="openError"
      />

      <OpsErrorDetailModal v-model:show="showErrorModal" :error-id="selectedErrorId" />

      <OpsRequestDetailsModal
        v-model="showRequestDetails"
        :time-range="timeRange"
        :preset="requestDetailsPreset"
        :platform="platform"
        :group-id="groupId"
        @openErrorDetail="openError"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch REDACTED from 'vue'
import { useDebounceFn REDACTED from '@vueuse/core'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute, useRouter REDACTED from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import {
  opsAPI,
  OPS_WS_CLOSE_CODES,
  type OpsWSStatus,
  type OpsDashboardOverview,
  type OpsErrorDistributionResponse,
  type OpsErrorTrendResponse,
  type OpsLatencyHistogramResponse,
  type OpsThroughputTrendResponse
REDACTED from '@/api/admin/ops'
import { useAdminSettingsStore, useAppStore REDACTED from '@/stores'
import OpsDashboardHeader from './components/OpsDashboardHeader.vue'
import OpsDashboardSkeleton from './components/OpsDashboardSkeleton.vue'
import OpsConcurrencyCard from './components/OpsConcurrencyCard.vue'
import OpsErrorDetailModal from './components/OpsErrorDetailModal.vue'
import OpsErrorDistributionChart from './components/OpsErrorDistributionChart.vue'
import OpsErrorDetailsModal from './components/OpsErrorDetailsModal.vue'
import OpsErrorTrendChart from './components/OpsErrorTrendChart.vue'
import OpsLatencyChart from './components/OpsLatencyChart.vue'
import OpsThroughputTrendChart from './components/OpsThroughputTrendChart.vue'
import OpsAlertEventsCard from './components/OpsAlertEventsCard.vue'
import OpsRequestDetailsModal, { type OpsRequestDetailsPreset REDACTED from './components/OpsRequestDetailsModal.vue'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const adminSettingsStore = useAdminSettingsStore()
const { t REDACTED = useI18n()

const opsEnabled = computed(() => adminSettingsStore.opsMonitoringEnabled)

type TimeRange = '5m' | '30m' | '1h' | '6h' | '24h'
const allowedTimeRanges = new Set<TimeRange>(['5m', '30m', '1h', '6h', '24h'])

type QueryMode = 'auto' | 'raw' | 'preagg'
const allowedQueryModes = new Set<QueryMode>(['auto', 'raw', 'preagg'])

const loading = ref(true)
const hasLoadedOnce = ref(false)
const errorMessage = ref('')
const lastUpdated = ref<Date | null>(new Date())

const timeRange = ref<TimeRange>('1h')
const platform = ref<string>('')
const groupId = ref<number | null>(null)
const queryMode = ref<QueryMode>('auto')

const QUERY_KEYS = {
  timeRange: 'tr',
  platform: 'platform',
  groupId: 'group_id',
  queryMode: 'mode'
REDACTED as const

const isApplyingRouteQuery = ref(false)
const isSyncingRouteQuery = ref(false)

// WebSocket for realtime QPS/TPS
const realTimeQPS = ref(0)
const realTimeTPS = ref(0)
const wsStatus = ref<OpsWSStatus>('closed')
const wsReconnectInMs = ref<number | null>(null)
const wsHasData = ref(false)
let unsubscribeQPS: (() => void) | null = null

let dashboardFetchController: AbortController | null = null
let dashboardFetchSeq = 0

function isCanceledRequest(err: unknown): boolean {
  return (
    !!err &&
    typeof err === 'object' &&
    'code' in err &&
    (err as Record<string, unknown>).code === 'ERR_CANCELED'
  )
REDACTED

function abortDashboardFetch() {
  if (dashboardFetchController) {
    dashboardFetchController.abort()
    dashboardFetchController = null
  REDACTED
REDACTED

function stopQPSSubscription(options?: { resetMetrics?: boolean REDACTED) {
  wsStatus.value = 'closed'
  wsReconnectInMs.value = null
  if (unsubscribeQPS) unsubscribeQPS()
  unsubscribeQPS = null

  if (options?.resetMetrics) {
    realTimeQPS.value = 0
    realTimeTPS.value = 0
    wsHasData.value = false
  REDACTED
REDACTED

function startQPSSubscription() {
  stopQPSSubscription()
  unsubscribeQPS = opsAPI.subscribeQPS(
    (payload) => {
      if (payload && typeof payload === 'object' && payload.type === 'qps_update' && payload.data) {
        realTimeQPS.value = payload.data.qps || 0
        realTimeTPS.value = payload.data.tps || 0
        wsHasData.value = true
      REDACTED
    REDACTED,
    {
      onStatusChange: (status) => {
        wsStatus.value = status
        if (status === 'connected') wsReconnectInMs.value = null
      REDACTED,
      onReconnectScheduled: ({ delayMs REDACTED) => {
        wsReconnectInMs.value = delayMs
      REDACTED,
      onFatalClose: (event) => {
        // Server-side feature flag says realtime is disabled; keep UI consistent and avoid reconnect loops.
        if (event && event.code === OPS_WS_CLOSE_CODES.REALTIME_DISABLED) {
          adminSettingsStore.setOpsRealtimeMonitoringEnabledLocal(false)
          stopQPSSubscription({ resetMetrics: true REDACTED)
        REDACTED
      REDACTED,
      // QPS updates may be sparse in idle periods; keep the timeout conservative.
      staleTimeoutMs: 180_000
    REDACTED
  )
REDACTED

const readQueryString = (key: string): string => {
  const value = route.query[key]
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
REDACTED

const readQueryNumber = (key: string): number | null => {
  const raw = readQueryString(key)
  if (!raw) return null
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) ? n : null
REDACTED

const applyRouteQueryToState = () => {
  const nextTimeRange = readQueryString(QUERY_KEYS.timeRange)
  if (nextTimeRange && allowedTimeRanges.has(nextTimeRange as TimeRange)) {
    timeRange.value = nextTimeRange as TimeRange
  REDACTED

  platform.value = readQueryString(QUERY_KEYS.platform) || ''

  const groupIdRaw = readQueryNumber(QUERY_KEYS.groupId)
  groupId.value = typeof groupIdRaw === 'number' && groupIdRaw > 0 ? groupIdRaw : null

  const nextMode = readQueryString(QUERY_KEYS.queryMode)
  if (nextMode && allowedQueryModes.has(nextMode as QueryMode)) {
    queryMode.value = nextMode as QueryMode
  REDACTED else {
    const fallback = adminSettingsStore.opsQueryModeDefault || 'auto'
    queryMode.value = allowedQueryModes.has(fallback as QueryMode) ? (fallback as QueryMode) : 'auto'
  REDACTED
REDACTED

applyRouteQueryToState()

const buildQueryFromState = () => {
  const next: Record<string, any> = { ...route.query REDACTED

  Object.values(QUERY_KEYS).forEach((k) => {
    delete next[k]
  REDACTED)

  if (timeRange.value !== '1h') next[QUERY_KEYS.timeRange] = timeRange.value
  if (platform.value) next[QUERY_KEYS.platform] = platform.value
  if (typeof groupId.value === 'number' && groupId.value > 0) next[QUERY_KEYS.groupId] = String(groupId.value)
  if (queryMode.value !== 'auto') next[QUERY_KEYS.queryMode] = queryMode.value

  return next
REDACTED

const syncQueryToRoute = useDebounceFn(async () => {
  if (isApplyingRouteQuery.value) return
  const nextQuery = buildQueryFromState()

  const curr = route.query as Record<string, any>
  const nextKeys = Object.keys(nextQuery)
  const currKeys = Object.keys(curr)
  const sameLength = nextKeys.length === currKeys.length
  const sameValues = sameLength && nextKeys.every((k) => String(curr[k] ?? '') === String(nextQuery[k] ?? ''))
  if (sameValues) return

  try {
    isSyncingRouteQuery.value = true
    await router.replace({ query: nextQuery REDACTED)
  REDACTED finally {
    isSyncingRouteQuery.value = false
  REDACTED
REDACTED, 250)

const overview = ref<OpsDashboardOverview | null>(null)

const throughputTrend = ref<OpsThroughputTrendResponse | null>(null)
const loadingTrend = ref(false)

const latencyHistogram = ref<OpsLatencyHistogramResponse | null>(null)
const loadingLatency = ref(false)

const errorTrend = ref<OpsErrorTrendResponse | null>(null)
const loadingErrorTrend = ref(false)

const errorDistribution = ref<OpsErrorDistributionResponse | null>(null)
const loadingErrorDistribution = ref(false)

const selectedErrorId = ref<number | null>(null)
const showErrorModal = ref(false)

const showErrorDetails = ref(false)
const errorDetailsType = ref<'request' | 'upstream'>('request')

const showRequestDetails = ref(false)
const requestDetailsPreset = ref<OpsRequestDetailsPreset>({
  title: '',
  kind: 'all',
  sort: 'created_at_desc'
REDACTED)

function handleThroughputSelectPlatform(nextPlatform: string) {
  platform.value = nextPlatform || ''
  groupId.value = null
REDACTED

function handleThroughputSelectGroup(nextGroupId: number) {
  const id = Number.isFinite(nextGroupId) && nextGroupId > 0 ? nextGroupId : null
  groupId.value = id
REDACTED

function handleOpenRequestDetails(preset?: OpsRequestDetailsPreset) {
  const basePreset: OpsRequestDetailsPreset = {
    title: t('admin.ops.requestDetails.title'),
    kind: 'all',
    sort: 'created_at_desc'
  REDACTED

  requestDetailsPreset.value = { ...basePreset, ...(preset ?? {REDACTED) REDACTED
  if (!requestDetailsPreset.value.title) requestDetailsPreset.value.title = basePreset.title
  showRequestDetails.value = true
REDACTED

function openErrorDetails(kind: 'request' | 'upstream') {
  errorDetailsType.value = kind
  showErrorDetails.value = true
REDACTED

function onTimeRangeChange(v: string | number | boolean | null) {
  if (typeof v !== 'string') return
  if (!allowedTimeRanges.has(v as TimeRange)) return
  timeRange.value = v as TimeRange
REDACTED

function onPlatformChange(v: string | number | boolean | null) {
  platform.value = typeof v === 'string' ? v : ''
REDACTED

function onGroupChange(v: string | number | boolean | null) {
  if (v === null) {
    groupId.value = null
    return
  REDACTED
  if (typeof v === 'number') {
    groupId.value = v > 0 ? v : null
    return
  REDACTED
  if (typeof v === 'string') {
    const n = Number.parseInt(v, 10)
    groupId.value = Number.isFinite(n) && n > 0 ? n : null
  REDACTED
REDACTED

function onQueryModeChange(v: string | number | boolean | null) {
  if (typeof v !== 'string') return
  if (!allowedQueryModes.has(v as QueryMode)) return
  queryMode.value = v as QueryMode
REDACTED

function openError(id: number) {
  selectedErrorId.value = id
  showErrorModal.value = true
REDACTED

async function refreshOverviewWithCancel(fetchSeq: number, signal: AbortSignal) {
  if (!opsEnabled.value) return
  try {
    const data = await opsAPI.getDashboardOverview(
      {
        time_range: timeRange.value,
        platform: platform.value || undefined,
        group_id: groupId.value ?? undefined,
        mode: queryMode.value
      REDACTED,
      { signal REDACTED
    )
    if (fetchSeq !== dashboardFetchSeq) return
    overview.value = data
  REDACTED catch (err: any) {
    if (fetchSeq !== dashboardFetchSeq || isCanceledRequest(err)) return
    overview.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadOverview'))
  REDACTED
REDACTED

async function refreshThroughputTrendWithCancel(fetchSeq: number, signal: AbortSignal) {
  if (!opsEnabled.value) return
  loadingTrend.value = true
  try {
    const data = await opsAPI.getThroughputTrend(
      {
        time_range: timeRange.value,
        platform: platform.value || undefined,
        group_id: groupId.value ?? undefined,
        mode: queryMode.value
      REDACTED,
      { signal REDACTED
    )
    if (fetchSeq !== dashboardFetchSeq) return
    throughputTrend.value = data
  REDACTED catch (err: any) {
    if (fetchSeq !== dashboardFetchSeq || isCanceledRequest(err)) return
    throughputTrend.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadThroughputTrend'))
  REDACTED finally {
    if (fetchSeq === dashboardFetchSeq) {
      loadingTrend.value = false
    REDACTED
  REDACTED
REDACTED

async function refreshLatencyHistogramWithCancel(fetchSeq: number, signal: AbortSignal) {
  if (!opsEnabled.value) return
  loadingLatency.value = true
  try {
    const data = await opsAPI.getLatencyHistogram(
      {
        time_range: timeRange.value,
        platform: platform.value || undefined,
        group_id: groupId.value ?? undefined,
        mode: queryMode.value
      REDACTED,
      { signal REDACTED
    )
    if (fetchSeq !== dashboardFetchSeq) return
    latencyHistogram.value = data
  REDACTED catch (err: any) {
    if (fetchSeq !== dashboardFetchSeq || isCanceledRequest(err)) return
    latencyHistogram.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadLatencyHistogram'))
  REDACTED finally {
    if (fetchSeq === dashboardFetchSeq) {
      loadingLatency.value = false
    REDACTED
  REDACTED
REDACTED

async function refreshErrorTrendWithCancel(fetchSeq: number, signal: AbortSignal) {
  if (!opsEnabled.value) return
  loadingErrorTrend.value = true
  try {
    const data = await opsAPI.getErrorTrend(
      {
        time_range: timeRange.value,
        platform: platform.value || undefined,
        group_id: groupId.value ?? undefined,
        mode: queryMode.value
      REDACTED,
      { signal REDACTED
    )
    if (fetchSeq !== dashboardFetchSeq) return
    errorTrend.value = data
  REDACTED catch (err: any) {
    if (fetchSeq !== dashboardFetchSeq || isCanceledRequest(err)) return
    errorTrend.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorTrend'))
  REDACTED finally {
    if (fetchSeq === dashboardFetchSeq) {
      loadingErrorTrend.value = false
    REDACTED
  REDACTED
REDACTED

async function refreshErrorDistributionWithCancel(fetchSeq: number, signal: AbortSignal) {
  if (!opsEnabled.value) return
  loadingErrorDistribution.value = true
  try {
    const data = await opsAPI.getErrorDistribution(
      {
        time_range: timeRange.value,
        platform: platform.value || undefined,
        group_id: groupId.value ?? undefined,
        mode: queryMode.value
      REDACTED,
      { signal REDACTED
    )
    if (fetchSeq !== dashboardFetchSeq) return
    errorDistribution.value = data
  REDACTED catch (err: any) {
    if (fetchSeq !== dashboardFetchSeq || isCanceledRequest(err)) return
    errorDistribution.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorDistribution'))
  REDACTED finally {
    if (fetchSeq === dashboardFetchSeq) {
      loadingErrorDistribution.value = false
    REDACTED
  REDACTED
REDACTED

function isOpsDisabledError(err: unknown): boolean {
  return (
    !!err &&
    typeof err === 'object' &&
    'code' in err &&
    typeof (err as Record<string, unknown>).code === 'string' &&
    (err as Record<string, unknown>).code === 'OPS_DISABLED'
  )
REDACTED

async function fetchData() {
  if (!opsEnabled.value) return

  abortDashboardFetch()
  dashboardFetchSeq += 1
  const fetchSeq = dashboardFetchSeq
  dashboardFetchController = new AbortController()

  loading.value = true
  errorMessage.value = ''
  try {
    await Promise.all([
      refreshOverviewWithCancel(fetchSeq, dashboardFetchController.signal),
      refreshThroughputTrendWithCancel(fetchSeq, dashboardFetchController.signal),
      refreshLatencyHistogramWithCancel(fetchSeq, dashboardFetchController.signal),
      refreshErrorTrendWithCancel(fetchSeq, dashboardFetchController.signal),
      refreshErrorDistributionWithCancel(fetchSeq, dashboardFetchController.signal)
    ])
    if (fetchSeq !== dashboardFetchSeq) return
    lastUpdated.value = new Date()
  REDACTED catch (err) {
    if (!isOpsDisabledError(err)) {
      console.error('[ops] failed to fetch dashboard data', err)
      errorMessage.value = t('admin.ops.failedToLoadData')
    REDACTED
  REDACTED finally {
    if (fetchSeq === dashboardFetchSeq) {
      loading.value = false
      hasLoadedOnce.value = true
    REDACTED
  REDACTED
REDACTED

watch(
  () => [timeRange.value, platform.value, groupId.value, queryMode.value] as const,
  () => {
    if (isApplyingRouteQuery.value) return
    if (opsEnabled.value) {
      fetchData()
    REDACTED
    syncQueryToRoute()
  REDACTED
)

watch(
  () => route.query,
  () => {
    if (isSyncingRouteQuery.value) return

    const prevTimeRange = timeRange.value
    const prevPlatform = platform.value
    const prevGroupId = groupId.value

    isApplyingRouteQuery.value = true
    applyRouteQueryToState()
    isApplyingRouteQuery.value = false

    const changed =
      prevTimeRange !== timeRange.value || prevPlatform !== platform.value || prevGroupId !== groupId.value
    if (changed) {
      if (opsEnabled.value) {
        fetchData()
      REDACTED
    REDACTED
  REDACTED
)

onMounted(async () => {
  await adminSettingsStore.fetch()
  if (!adminSettingsStore.opsMonitoringEnabled) {
    await router.replace('/admin/settings')
    return
  REDACTED

  if (adminSettingsStore.opsRealtimeMonitoringEnabled) {
    startQPSSubscription()
  REDACTED else {
    stopQPSSubscription({ resetMetrics: true REDACTED)
  REDACTED

  if (opsEnabled.value) {
    await fetchData()
  REDACTED
REDACTED)

onUnmounted(() => {
  stopQPSSubscription()
  abortDashboardFetch()
REDACTED)

watch(
  () => adminSettingsStore.opsRealtimeMonitoringEnabled,
  (enabled) => {
    if (!opsEnabled.value) return
    if (enabled) {
      startQPSSubscription()
    REDACTED else {
      stopQPSSubscription({ resetMetrics: true REDACTED)
    REDACTED
  REDACTED
)
</script>
