<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api'
import { opsAPI, type OpsDashboardOverview, type OpsMetricThresholds, type OpsRealtimeTrafficSummary } from '@/api/admin/ops'
import type { OpsRequestDetailsPreset } from './OpsRequestDetailsModal.vue'
import { useAdminSettingsStore } from '@/stores'
import { formatNumber } from '@/utils/format'

type RealtimeWindow = '1min' | '5min' | '30min' | '1h'

interface Props {
  overview?: OpsDashboardOverview | null
  platform: string
  groupId: number | null
  timeRange: string
  queryMode: string
  loading: boolean
  lastUpdated: Date | null
  thresholds?: OpsMetricThresholds | null // 阈值配置
  autoRefreshEnabled?: boolean
  autoRefreshCountdown?: number
  fullscreen?: boolean
  customStartTime?: string | null
  customEndTime?: string | null
}

interface Emits {
  (e: 'update:platform', value: string): void
  (e: 'update:group', value: number | null): void
  (e: 'update:timeRange', value: string): void
  (e: 'update:queryMode', value: string): void
  (e: 'update:customTimeRange', startTime: string, endTime: string): void
  (e: 'refresh'): void
  (e: 'openRequestDetails', preset?: OpsRequestDetailsPreset): void
  (e: 'openErrorDetails', kind: 'request' | 'upstream'): void
  (e: 'openSettings'): void
  (e: 'openAlertRules'): void
  (e: 'enterFullscreen'): void
  (e: 'exitFullscreen'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const adminSettingsStore = useAdminSettingsStore()

const realtimeWindow = ref<RealtimeWindow>('1min')

const overview = computed(() => props.overview ?? null)
const systemMetrics = computed(() => overview.value?.system_metrics ?? null)

const REALTIME_WINDOW_MINUTES: Record<RealtimeWindow, number> = {
  '1min': 1,
  '5min': 5,
  '30min': 30,
  '1h': 60
}

const TOOLBAR_RANGE_MINUTES: Record<string, number> = {
  '5m': 5,
  '30m': 30,
  '1h': 60,
  '6h': 6 * 60,
  '24h': 24 * 60
}

const availableRealtimeWindows = computed(() => {
  const toolbarMinutes = TOOLBAR_RANGE_MINUTES[props.timeRange] ?? 60
  return (['1min', '5min', '30min', '1h'] as const).filter((w) => REALTIME_WINDOW_MINUTES[w] <= toolbarMinutes)
})

watch(
  () => props.timeRange,
  () => {
    // The realtime window must be inside the toolbar window; reset to keep UX predictable.
    realtimeWindow.value = '1min'
    // Keep realtime traffic consistent with toolbar changes even when the window is already 1min.
    loadRealtimeTrafficSummary()
  }
)

// --- Filters ---

const showCustomTimeRangeDialog = ref(false)
const customStartTimeInput = ref('')
const customEndTimeInput = ref('')

function formatCustomTimeRangeLabel(startTime: string, endTime: string): string {
  const start = new Date(startTime)
  const end = new Date(endTime)
  const formatDate = (d: Date) => {
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const hour = String(d.getHours()).padStart(2, '0')
    const minute = String(d.getMinutes()).padStart(2, '0')
    return `${month}-${day} ${hour}:${minute}`
  }
  return `${formatDate(start)} ~ ${formatDate(end)}`
}

const groups = ref<Array<{ id: number; name: string; platform: string }>>([])

const platformOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'grok', label: 'Grok' }
])

const timeRangeOptions = computed(() => [
  { value: '5m', label: t('admin.ops.timeRange.5m') },
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  {
    value: 'custom',
    label: props.timeRange === 'custom' && props.customStartTime && props.customEndTime
      ? `${t('admin.ops.timeRange.custom')} (${formatCustomTimeRangeLabel(props.customStartTime, props.customEndTime)})`
      : t('admin.ops.timeRange.custom')
  }
])

const queryModeOptions = computed(() => [
  { value: 'auto', label: t('admin.ops.queryMode.auto') },
  { value: 'raw', label: t('admin.ops.queryMode.raw') },
  { value: 'preagg', label: t('admin.ops.queryMode.preagg') }
])

const groupOptions = computed(() => {
  const filtered = props.platform ? groups.value.filter((g) => g.platform === props.platform) : groups.value
  return [{ value: null, label: t('common.all') }, ...filtered.map((g) => ({ value: g.id, label: g.name }))]
})

watch(
  () => props.platform,
  (newPlatform) => {
    if (!newPlatform) return
    const currentGroup = groups.value.find((g) => g.id === props.groupId)
    if (currentGroup && currentGroup.platform !== newPlatform) {
      emit('update:group', null)
    }
  }
)

onMounted(async () => {
  try {
    const list = await adminAPI.groups.getAll()
    groups.value = list.map((g) => ({ id: g.id, name: g.name, platform: g.platform }))
  } catch (e) {
    console.error('[OpsDashboardHeader] Failed to load groups', e)
    groups.value = []
  }
})

function handlePlatformChange(val: string | number | boolean | null) {
  emit('update:platform', String(val || ''))
}

function handleGroupChange(val: string | number | boolean | null) {
  if (val === null || val === '' || typeof val === 'boolean') {
    emit('update:group', null)
    return
  }
  const id = typeof val === 'number' ? val : Number.parseInt(String(val), 10)
  emit('update:group', Number.isFinite(id) && id > 0 ? id : null)
}

function handleTimeRangeChange(val: string | number | boolean | null) {
  const newValue = String(val || '1h')
  if (newValue === 'custom') {
    // 初始化为最近1小时
    const now = new Date()
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
    customStartTimeInput.value = oneHourAgo.toISOString().slice(0, 16)
    customEndTimeInput.value = now.toISOString().slice(0, 16)
    showCustomTimeRangeDialog.value = true
  } else {
    emit('update:timeRange', newValue)
  }
}

function handleCustomTimeRangeConfirm() {
  if (!customStartTimeInput.value || !customEndTimeInput.value) return
  const startTime = new Date(customStartTimeInput.value).toISOString()
  const endTime = new Date(customEndTimeInput.value).toISOString()
  // Emit custom time range first so the parent can build correct API params
  // when it reacts to timeRange switching to "custom".
  emit('update:customTimeRange', startTime, endTime)
  emit('update:timeRange', 'custom')
  showCustomTimeRangeDialog.value = false
}

function handleCustomTimeRangeCancel() {
  showCustomTimeRangeDialog.value = false
  // 如果当前不是 custom，不需要做任何事
  // 如果当前是 custom，保持不变
}

function handleQueryModeChange(val: string | number | boolean | null) {
  emit('update:queryMode', String(val || 'auto'))
}

function openDetails(preset?: OpsRequestDetailsPreset) {
  emit('openRequestDetails', preset)
}

function openErrorDetails(kind: 'request' | 'upstream') {
  emit('openErrorDetails', kind)
}

// --- Threshold checking helpers ---
type ThresholdLevel = 'normal' | 'warning' | 'critical'

function getSLAThresholdLevel(slaPercent: number | null): ThresholdLevel {
  if (slaPercent == null) return 'normal'
  const threshold = props.thresholds?.sla_percent_min
  if (threshold == null) return 'normal'

  // SLA is "higher is better":
  // - below threshold => critical
  // - within +0.1% buffer => warning
  const warningBuffer = 0.1

  if (slaPercent < threshold) return 'critical'
  if (slaPercent < threshold + warningBuffer) return 'warning'
  return 'normal'
}

function getTTFTThresholdLevel(ttftMs: number | null): ThresholdLevel {
  if (ttftMs == null) return 'normal'
  const threshold = props.thresholds?.ttft_p99_ms_max
  if (threshold == null) return 'normal'
  if (ttftMs >= threshold) return 'critical'
  if (ttftMs >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getRequestErrorRateThresholdLevel(errorRatePercent: number | null): ThresholdLevel {
  if (errorRatePercent == null) return 'normal'
  const threshold = props.thresholds?.request_error_rate_percent_max
  if (threshold == null) return 'normal'
  if (errorRatePercent >= threshold) return 'critical'
  if (errorRatePercent >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent: number | null): ThresholdLevel {
  if (upstreamErrorRatePercent == null) return 'normal'
  const threshold = props.thresholds?.upstream_error_rate_percent_max
  if (threshold == null) return 'normal'
  if (upstreamErrorRatePercent >= threshold) return 'critical'
  if (upstreamErrorRatePercent >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getThresholdColorClass(level: ThresholdLevel): string {
  switch (level) {
    case 'critical':
      return 'text-red-600 dark:text-red-400'
    case 'warning':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-green-600 dark:text-green-400'
  }
}

// --- Realtime / Overview labels ---

const totalRequestsLabel = computed(() => formatNumber(overview.value?.request_count_total ?? 0))
const totalTokensLabel = computed(() => formatNumber(overview.value?.token_consumed ?? 0))

const realtimeTrafficSummary = ref<OpsRealtimeTrafficSummary | null>(null)
const realtimeTrafficLoading = ref(false)

function makeZeroRealtimeTrafficSummary(): OpsRealtimeTrafficSummary {
  const now = new Date().toISOString()
  return {
    window: realtimeWindow.value,
    start_time: now,
    end_time: now,
    platform: props.platform,
    group_id: props.groupId,
    qps: { current: 0, peak: 0, avg: 0 },
    tps: { current: 0, peak: 0, avg: 0 }
  }
}

async function loadRealtimeTrafficSummary() {
  if (realtimeTrafficLoading.value) return
  if (!adminSettingsStore.opsRealtimeMonitoringEnabled) {
    realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
    return
  }
  realtimeTrafficLoading.value = true
  try {
    const res = await opsAPI.getRealtimeTrafficSummary(realtimeWindow.value, props.platform, props.groupId)
    if (res && res.enabled === false) {
      adminSettingsStore.setOpsRealtimeMonitoringEnabledLocal(false)
    }
    realtimeTrafficSummary.value = res?.summary ?? null
  } catch (err) {
    console.error('[OpsDashboardHeader] Failed to load realtime traffic summary', err)
    realtimeTrafficSummary.value = null
  } finally {
    realtimeTrafficLoading.value = false
  }
}

watch(
  () => [realtimeWindow.value, props.platform, props.groupId] as const,
  () => {
    loadRealtimeTrafficSummary()
  },
  { immediate: true }
)

watch(
  () => adminSettingsStore.opsRealtimeMonitoringEnabled,
  (enabled) => {
    if (!enabled) {
      // Keep UI stable when realtime monitoring is turned off.
      realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
    } else {
      loadRealtimeTrafficSummary()
    }
  },
  { immediate: true }
)

// Realtime traffic refresh follows the parent (OpsDashboard) refresh cadence.
watch(
  () => [props.autoRefreshEnabled, props.autoRefreshCountdown, props.loading] as const,
  ([enabled, countdown, loading]) => {
    if (!enabled) return
    if (loading) return
    // Treat countdown reset (or reaching 0) as a refresh boundary.
    if (countdown === 0) {
      loadRealtimeTrafficSummary()
    }
  }
)

// no-op: parent controls refresh cadence

const displayRealTimeQps = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
})

const displayRealTimeTps = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
})

const realtimeQpsPeakLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.peak
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeTpsPeakLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.peak
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeQpsAvgLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.avg
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeTpsAvgLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.avg
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})

const qpsAvgLabel = computed(() => {
  const v = overview.value?.qps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
})

const tpsAvgLabel = computed(() => {
  const v = overview.value?.tps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
})

const slaPercent = computed(() => {
  const v = overview.value?.sla
  if (typeof v !== 'number') return null
  return v * 100
})

const errorRatePercent = computed(() => {
  const v = overview.value?.error_rate
  if (typeof v !== 'number') return null
  return v * 100
})

const upstreamErrorRatePercent = computed(() => {
  const v = overview.value?.upstream_error_rate
  if (typeof v !== 'number') return null
  return v * 100
})

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

// --- Health Score & Diagnosis (primary) ---

const isSystemIdle = computed(() => {
  const ov = overview.value
  if (!ov) return true
  const qps = ov.qps?.current
  const errorRate = ov.error_rate ?? 0
  return (qps ?? 0) === 0 && errorRate === 0
})

const healthScoreValue = computed<number | null>(() => {
  const v = overview.value?.health_score
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const healthScoreColor = computed(() => {
  if (isSystemIdle.value) return '#9ca3af' // gray-400
  const score = healthScoreValue.value
  if (score == null) return '#9ca3af'
  if (score >= 90) return '#10b981' // green
  if (score >= 60) return '#f59e0b' // yellow
  return '#ef4444' // red
})

const healthScoreClass = computed(() => {
  if (isSystemIdle.value) return 'text-gray-400'
  const score = healthScoreValue.value
  if (score == null) return 'text-gray-400'
  if (score >= 90) return 'text-green-500'
  if (score >= 60) return 'text-yellow-500'
  return 'text-red-500'
})

const circleSize = computed(() => props.fullscreen ? 140 : 100)
const strokeWidth = computed(() => props.fullscreen ? 10 : 8)
const radius = computed(() => (circleSize.value - strokeWidth.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const dashOffset = computed(() => {
  if (isSystemIdle.value) return 0
  if (healthScoreValue.value == null) return 0
  const score = Math.max(0, Math.min(100, healthScoreValue.value))
  return circumference.value - (score / 100) * circumference.value
})

interface DiagnosisItem {
  type: 'critical' | 'warning' | 'info'
  message: string
  impact: string
  action?: string
}

const diagnosisReport = computed<DiagnosisItem[]>(() => {
  const ov = overview.value
  if (!ov) return []

  const report: DiagnosisItem[] = []

  if (isSystemIdle.value) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.idle'),
      impact: t('admin.ops.diagnosis.idleImpact')
    })
    return report
  }

  // Resource diagnostics (highest priority)
  const sm = ov.system_metrics
  if (sm) {
    if (sm.db_ok === false) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.dbDown'),
        impact: t('admin.ops.diagnosis.dbDownImpact'),
        action: t('admin.ops.diagnosis.dbDownAction')
      })
    }
    if (sm.redis_ok === false) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.redisDown'),
        impact: t('admin.ops.diagnosis.redisDownImpact'),
        action: t('admin.ops.diagnosis.redisDownAction')
      })
    }

    const cpuPct = sm.cpu_usage_percent ?? 0
    if (cpuPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.cpuCritical', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuCriticalImpact'),
        action: t('admin.ops.diagnosis.cpuCriticalAction')
      })
    } else if (cpuPct > 80) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.cpuHigh', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuHighImpact'),
        action: t('admin.ops.diagnosis.cpuHighAction')
      })
    }

    const memPct = sm.memory_usage_percent ?? 0
    if (memPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.memoryCritical', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryCriticalImpact'),
        action: t('admin.ops.diagnosis.memoryCriticalAction')
      })
    } else if (memPct > 85) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.memoryHigh', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryHighImpact'),
        action: t('admin.ops.diagnosis.memoryHighAction')
      })
    }
  }

  const ttftP99 = ov.ttft?.p99_ms ?? 0
  if (ttftP99 > 500) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.ttftHigh', { ttft: ttftP99.toFixed(0) }),
      impact: t('admin.ops.diagnosis.ttftHighImpact'),
      action: t('admin.ops.diagnosis.ttftHighAction')
    })
  }

  // Error rate diagnostics (adjusted thresholds)
  const upstreamRatePct = (ov.upstream_error_rate ?? 0) * 100
  if (upstreamRatePct > 5) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.upstreamCritical', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamCriticalImpact'),
      action: t('admin.ops.diagnosis.upstreamCriticalAction')
    })
  } else if (upstreamRatePct > 2) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.upstreamHigh', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamHighImpact'),
      action: t('admin.ops.diagnosis.upstreamHighAction')
    })
  }

  const errorPct = (ov.error_rate ?? 0) * 100
  if (errorPct > 3) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.errorHigh', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorHighImpact'),
      action: t('admin.ops.diagnosis.errorHighAction')
    })
  } else if (errorPct > 0.5) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.errorElevated', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorElevatedImpact'),
      action: t('admin.ops.diagnosis.errorElevatedAction')
    })
  }

  // SLA diagnostics
  const slaPct = (ov.sla ?? 0) * 100
  if (slaPct < 90) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.slaCritical', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaCriticalImpact'),
      action: t('admin.ops.diagnosis.slaCriticalAction')
    })
  } else if (slaPct < 98) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.slaLow', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaLowImpact'),
      action: t('admin.ops.diagnosis.slaLowAction')
    })
  }

  // Health score diagnostics (lowest priority)
  if (healthScoreValue.value != null) {
    if (healthScoreValue.value < 60) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.healthCritical', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthCriticalImpact'),
        action: t('admin.ops.diagnosis.healthCriticalAction')
      })
    } else if (healthScoreValue.value < 90) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.healthLow', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthLowImpact'),
        action: t('admin.ops.diagnosis.healthLowAction')
      })
    }
  }

  if (report.length === 0) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.healthy'),
      impact: t('admin.ops.diagnosis.healthyImpact')
    })
  }

  return report
})

// --- System health (secondary) ---

function formatTimeShort(ts?: string | null): string {
  if (!ts) return '-'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleTimeString()
}

const cpuPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.cpu_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const cpuPercentClass = computed(() => {
  const v = cpuPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 80) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
})

const memPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.memory_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const memPercentClass = computed(() => {
  const v = memPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 85) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
})

const dbConnActiveValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_active
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnWaitingValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_waiting
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnOpenValue = computed<number | null>(() => {
  if (dbConnActiveValue.value == null || dbConnIdleValue.value == null) return null
  return dbConnActiveValue.value + dbConnIdleValue.value
})

const dbMaxOpenConnsValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_max_open_conns
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbUsagePercent = computed<number | null>(() => {
  if (dbConnOpenValue.value == null || dbMaxOpenConnsValue.value == null || dbMaxOpenConnsValue.value <= 0) return null
  return Math.min(100, Math.max(0, (dbConnOpenValue.value / dbMaxOpenConnsValue.value) * 100))
})

const dbMiddleLabel = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'FAIL'
  if (dbUsagePercent.value != null) return `${dbUsagePercent.value.toFixed(0)}%`
  if (systemMetrics.value?.db_ok === true) return t('admin.ops.ok')
  return t('admin.ops.noData')
})

const dbMiddleClass = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'text-rose-600 dark:text-rose-400'
  if (dbUsagePercent.value != null) {
    if (dbUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (dbUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  }
  if (systemMetrics.value?.db_ok === true) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
})

const redisConnTotalValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_total
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisConnActiveValue = computed<number | null>(() => {
  if (redisConnTotalValue.value == null || redisConnIdleValue.value == null) return null
  return Math.max(redisConnTotalValue.value - redisConnIdleValue.value, 0)
})

const redisPoolSizeValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_pool_size
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisUsagePercent = computed<number | null>(() => {
  if (redisConnTotalValue.value == null || redisPoolSizeValue.value == null || redisPoolSizeValue.value <= 0) return null
  return Math.min(100, Math.max(0, (redisConnTotalValue.value / redisPoolSizeValue.value) * 100))
})

const redisMiddleLabel = computed(() => {
  if (systemMetrics.value?.redis_ok === false) return 'FAIL'
  if (redisUsagePercent.value != null) return `${redisUsagePercent.value.toFixed(0)}%`
  if (systemMetrics.value?.redis_ok === true) return t('admin.ops.ok')
  return t('admin.ops.noData')
})

const redisMiddleClass = computed(() => {
  if (systemMetrics.value?.redis_ok === false) return 'text-rose-600 dark:text-rose-400'
  if (redisUsagePercent.value != null) {
    if (redisUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (redisUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  }
  if (systemMetrics.value?.redis_ok === true) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
})

const goroutineCountValue = computed<number | null>(() => {
  const v = systemMetrics.value?.goroutine_count
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const goroutinesWarnThreshold = 8_000
const goroutinesCriticalThreshold = 15_000

const goroutineStatus = computed<'ok' | 'warning' | 'critical' | 'unknown'>(() => {
  const n = goroutineCountValue.value
  if (n == null) return 'unknown'
  if (n >= goroutinesCriticalThreshold) return 'critical'
  if (n >= goroutinesWarnThreshold) return 'warning'
  return 'ok'
})

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
  }
})

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
  }
})

const jobHeartbeats = computed(() => overview.value?.job_heartbeats ?? [])

const jobsStatus = computed<'ok' | 'warn' | 'unknown'>(() => {
  const list = jobHeartbeats.value
  if (!list.length) return 'unknown'
  for (const hb of list) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) return 'warn'
  }
  return 'ok'
})

const jobsWarnCount = computed(() => {
  let warn = 0
  for (const hb of jobHeartbeats.value) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) warn++
  }
  return warn
})

const jobsStatusLabel = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warn':
      return t('common.warning')
    default:
      return t('admin.ops.noData')
  }
})

const jobsStatusClass = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warn':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-gray-900 dark:text-white'
  }
})

const showJobsDetails = ref(false)

function openJobsDetails() {
  showJobsDetails.value = true
}

function handleToolbarRefresh() {
  loadRealtimeTrafficSummary()
  emit('refresh')
}
</script>

<template>
  <div :class="['flex flex-col gap-4 rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700', props.fullscreen ? 'p-8' : 'p-6']">
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
          {{ t('admin.ops.title') }}
        </h1>

        <div v-if="!props.fullscreen" class="mt-1 flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
          <span class="flex items-center gap-1.5" :title="props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready')">
            <span class="relative flex h-2 w-2">
              <span class="relative inline-flex h-2 w-2 rounded-full" :class="props.loading ? 'bg-gray-400' : 'bg-green-500'"></span>
            </span>
            {{ props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready') }}
          </span>

          <span>·</span>
          <span>{{ t('common.refresh') }}: {{ props.lastUpdated ? props.lastUpdated.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).replace(/\//g, '-') : t('common.unknown') }}</span>

          <template v-if="props.autoRefreshEnabled && props.autoRefreshCountdown !== undefined">
            <span>·</span>
            <span>{{ t('admin.ops.autoRefreshRemaining', { seconds: props.autoRefreshCountdown }) }}</span>
          </template>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-3">
        <template v-if="!props.fullscreen">
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
        </template>

        <Select
          v-if="false"
          :model-value="queryMode"
          :options="queryModeOptions"
          class="relative w-full sm:w-[170px]"
          @update:model-value="handleQueryModeChange"
        />

        <button
          v-if="!props.fullscreen"
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-500 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="handleToolbarRefresh"
        >
          <svg class="h-4 w-4" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
            />
          </svg>
        </button>

        <div v-if="!props.fullscreen" class="mx-1 hidden h-4 w-[1px] bg-gray-200 dark:bg-dark-700 sm:block"></div>

        <!-- Alert Rules Button (hidden in fullscreen) -->
        <button
          v-if="!props.fullscreen"
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-lg bg-blue-100 px-3 text-xs font-bold text-blue-700 transition-colors hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:hover:bg-blue-900/50"
          :title="t('admin.ops.alertRules.title')"
          @click="emit('openAlertRules')"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
          <span class="hidden sm:inline">{{ t('admin.ops.alertRules.manage') }}</span>
        </button>

        <!-- Settings Button (hidden in fullscreen) -->
        <button
          v-if="!props.fullscreen"
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-lg bg-gray-100 px-3 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :title="t('admin.ops.settings.title')"
          @click="emit('openSettings')"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          <span class="hidden sm:inline">{{ t('common.settings') }}</span>
        </button>

        <!-- Enter Fullscreen Button (hidden in fullscreen mode) -->
        <button
          v-if="!props.fullscreen"
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :title="t('admin.ops.fullscreen.enter')"
          @click="emit('enterFullscreen')"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
          </svg>
        </button>
      </div>
    </div>

    <div v-if="overview" class="grid grid-cols-1 gap-6 lg:grid-cols-12">
      <!-- Left: Health + Realtime -->
      <div :class="['rounded-2xl bg-gray-50 dark:bg-dark-900 lg:col-span-5', props.fullscreen ? 'p-6' : 'p-4']">
        <div class="grid h-full grid-cols-1 gap-6 md:grid-cols-[200px_1fr] md:items-center">
          <!-- 1) Health Score -->
          <div
            class="group relative flex cursor-pointer flex-col items-center justify-center rounded-xl py-2 transition-all hover:bg-white/60 dark:hover:bg-dark-800/60 md:border-r md:border-gray-200 md:pr-6 dark:md:border-dark-700"
          >
            <!-- Diagnosis Popover (hover) -->
            <div
              class="pointer-events-none absolute left-1/2 top-full z-50 mt-2 w-72 -translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100 md:left-full md:top-0 md:ml-2 md:mt-0 md:translate-x-0"
            >
              <div class="rounded-xl bg-white p-4 shadow-xl ring-1 ring-black/5 dark:bg-gray-800 dark:ring-white/10">
                <h4 class="mb-3 border-b border-gray-100 pb-2 text-sm font-bold text-gray-900 dark:border-gray-700 dark:text-white flex items-center gap-2">
                  <Icon name="brain" size="sm" class="text-blue-500" />
                  {{ t('admin.ops.diagnosis.title') }}
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
                    <div class="flex-1">
                      <div class="text-xs font-semibold text-gray-900 dark:text-white">{{ item.message }}</div>
                      <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">{{ item.impact }}</div>
                      <div v-if="item.action" class="mt-1 text-[11px] text-blue-600 dark:text-blue-400 flex items-center gap-1">
                        <Icon name="lightbulb" size="xs" />
                        {{ item.action }}
                      </div>
                    </div>
                  </div>
                </div>

                <div class="mt-3 border-t border-gray-100 pt-2 text-[10px] text-gray-400 dark:border-gray-700">
                  {{ t('admin.ops.diagnosis.footer') }}
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
                <span :class="[props.fullscreen ? 'text-5xl' : 'text-3xl', 'font-black', healthScoreClass]">
                  {{ isSystemIdle ? t('admin.ops.idleStatus') : (overview.health_score ?? '--') }}
                </span>
                <span :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase tracking-wider text-gray-400']">{{ t('admin.ops.health') }}</span>
              </div>
            </div>

            <div class="mt-4 text-center" v-if="!props.fullscreen">
              <div class="flex items-center justify-center gap-1 text-xs font-medium text-gray-500">
                {{ t('admin.ops.healthCondition') }}
                <HelpTooltip :content="t('admin.ops.healthHelp')" />
              </div>
              <div class="mt-1 text-xs font-bold" :class="healthScoreClass">
                {{
                  isSystemIdle
                    ? t('admin.ops.idleStatus')
                    : typeof overview.health_score === 'number' && overview.health_score >= 90
                      ? t('admin.ops.healthyStatus')
                      : t('admin.ops.riskyStatus')
                }}
              </div>
            </div>
          </div>

          <!-- 2) Realtime Traffic -->
          <div class="flex h-full flex-col justify-center py-2">
            <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <div class="relative flex h-3 w-3 shrink-0">
                  <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75"></span>
                  <span class="relative inline-flex h-3 w-3 rounded-full bg-blue-500"></span>
                </div>
                <h3 class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.realtime.title') }}</h3>
                <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.qps')" />
              </div>

              <!-- Time Window Selector -->
              <div class="flex flex-wrap gap-1">
                <button
                  v-for="window in availableRealtimeWindows"
                  :key="window"
                  type="button"
                  class="rounded px-1.5 py-0.5 text-[9px] font-bold transition-colors sm:px-2 sm:text-[10px]"
                  :class="realtimeWindow === window
                    ? 'bg-blue-500 text-white'
                    : 'bg-gray-200 text-gray-600 hover:bg-gray-300 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'"
                  @click="realtimeWindow = window"
                >
                  {{ window }}
                </button>
              </div>
            </div>

            <div :class="props.fullscreen ? 'space-y-4' : 'space-y-3'">
              <!-- Row 1: Current -->
              <div>
                <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.current') }}</div>
                <div class="mt-1 flex flex-wrap items-baseline gap-x-4 gap-y-2">
                  <div class="flex items-baseline gap-1.5">
                    <span :class="[props.fullscreen ? 'text-4xl' : 'text-xl sm:text-2xl', 'font-black text-gray-900 dark:text-white']">{{ displayRealTimeQps.toFixed(1) }}</span>
                    <span :class="[props.fullscreen ? 'text-sm' : 'text-xs', 'font-bold text-gray-500']">QPS</span>
                  </div>
                  <div class="flex items-baseline gap-1.5">
                    <span :class="[props.fullscreen ? 'text-4xl' : 'text-xl sm:text-2xl', 'font-black text-gray-900 dark:text-white']">{{ displayRealTimeTps.toFixed(1) }}</span>
                    <span :class="[props.fullscreen ? 'text-sm' : 'text-xs', 'font-bold text-gray-500']">{{ t('admin.ops.tps') }}</span>
                  </div>
                </div>
              </div>

              <!-- Row 2: Peak + Average -->
              <div class="grid grid-cols-2 gap-3">
                <!-- Peak -->
                <div>
                  <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.peak') }}</div>
                  <div :class="[props.fullscreen ? 'text-base' : 'text-sm', 'mt-1 space-y-0.5 font-medium text-gray-600 dark:text-gray-400']">
                    <div class="flex items-baseline gap-1.5">
                      <span class="font-black text-gray-900 dark:text-white">{{ realtimeQpsPeakLabel }}</span>
                      <span class="text-xs">QPS</span>
                    </div>
                    <div class="flex items-baseline gap-1.5">
                      <span class="font-black text-gray-900 dark:text-white">{{ realtimeTpsPeakLabel }}</span>
                      <span class="text-xs">{{ t('admin.ops.tps') }}</span>
                    </div>
                  </div>
                </div>

                <!-- Average -->
                <div>
                  <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.average') }}</div>
                  <div :class="[props.fullscreen ? 'text-base' : 'text-sm', 'mt-1 space-y-0.5 font-medium text-gray-600 dark:text-gray-400']">
                    <div class="flex items-baseline gap-1.5">
                      <span class="font-black text-gray-900 dark:text-white">{{ realtimeQpsAvgLabel }}</span>
                      <span class="text-xs">QPS</span>
                    </div>
                    <div class="flex items-baseline gap-1.5">
                      <span class="font-black text-gray-900 dark:text-white">{{ realtimeTpsAvgLabel }}</span>
                      <span class="text-xs">{{ t('admin.ops.tps') }}</span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Animated Pulse Line (Heart Beat Animation) -->
              <div class="h-8 w-full overflow-hidden opacity-50">
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

      <!-- Right: 6 cards (3 cols x 2 rows) -->
      <div class="grid h-full grid-cols-1 content-center gap-4 sm:grid-cols-2 lg:col-span-7 lg:grid-cols-3">
        <!-- Card 1: Requests -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 1;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.requestsTitle') }}</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.totalRequests')" />
            </div>
            <button
              v-if="!props.fullscreen"
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.requestDetails.title') })"
            >
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 space-y-2 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.requests') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ totalRequestsLabel }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.tokens') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ totalTokensLabel }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgQps') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ qpsAvgLabel }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.avgTps') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ tpsAvgLabel }}</span>
            </div>
          </div>
        </div>

        <!-- Card 2: SLA -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 2;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.sla') }}</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.sla')" />
              <span class="h-1.5 w-1.5 rounded-full" :class="getSLAThresholdLevel(slaPercent) === 'critical' ? 'bg-red-500' : getSLAThresholdLevel(slaPercent) === 'warning' ? 'bg-yellow-500' : 'bg-green-500'"></span>
            </div>
            <button
              v-if="!props.fullscreen"
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.requestDetails.title'), kind: 'error' })"
            >
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getSLAThresholdLevel(slaPercent))">
            {{ slaPercent == null ? '-' : `${slaPercent.toFixed(3)}%` }}
          </div>
          <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full transition-all" :class="getSLAThresholdLevel(slaPercent) === 'critical' ? 'bg-red-500' : getSLAThresholdLevel(slaPercent) === 'warning' ? 'bg-yellow-500' : 'bg-green-500'" :style="{ width: `${Math.max((slaPercent ?? 0) - 90, 0) * 10}%` }"></div>
          </div>
          <div class="mt-3 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.exceptions') }}:</span>
              <span class="font-bold text-red-600 dark:text-red-400">{{ formatNumber((overview.request_count_sla ?? 0) - (overview.success_count ?? 0)) }}</span>
            </div>
          </div>
        </div>

        <!-- Card 4: Request Duration -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 4;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.latencyDuration') }}</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.latency')" />
            </div>
            <button
              v-if="!props.fullscreen"
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.latencyDuration'), sort: 'duration_desc' })"
            >
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 flex items-baseline gap-2">
            <div class="text-3xl font-black text-gray-900 dark:text-white">
              {{ durationP99Ms ?? '-' }}
            </div>
            <span class="text-xs font-bold text-gray-400">ms (P99)</span>
          </div>
          <div class="mt-3 grid grid-cols-1 gap-x-3 gap-y-1 text-xs 2xl:grid-cols-2">
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P95:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ durationP95Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P90:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ durationP90Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P50:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ durationP50Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">Avg:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ durationAvgMs ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">Max:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ durationMaxMs ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
          </div>
        </div>

        <!-- Card 5: TTFT -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 5;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">TTFT</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.ttft')" />
            </div>
            <button
              v-if="!props.fullscreen"
              class="text-[10px] font-bold text-blue-500 hover:underline"
              type="button"
              @click="openDetails({ title: t('admin.ops.ttftLabel'), sort: 'duration_desc' })"
            >
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 flex items-baseline gap-2">
            <div class="text-3xl font-black" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP99Ms))">
              {{ ttftP99Ms ?? '-' }}
            </div>
            <span class="text-xs font-bold text-gray-400">ms (P99)</span>
          </div>
          <div class="mt-3 grid grid-cols-1 gap-x-3 gap-y-1 text-xs 2xl:grid-cols-2">
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P95:</span>
              <span class="font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP95Ms))">{{ ttftP95Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P90:</span>
              <span class="font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP90Ms))">{{ ttftP90Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">P50:</span>
              <span class="font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP50Ms))">{{ ttftP50Ms ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">Avg:</span>
              <span class="font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftAvgMs))">{{ ttftAvgMs ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
            <div class="flex items-baseline gap-1 whitespace-nowrap">
              <span class="text-gray-500">Max:</span>
              <span class="font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftMaxMs))">{{ ttftMaxMs ?? '-' }}</span>
              <span class="text-gray-400">ms</span>
            </div>
          </div>
        </div>

        <!-- Card 3: Request Errors -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 3;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.requestErrors') }}</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.errors')" />
            </div>
            <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('request')">
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getRequestErrorRateThresholdLevel(errorRatePercent))">
            {{ errorRatePercent == null ? '-' : `${errorRatePercent.toFixed(2)}%` }}
          </div>
          <div class="mt-3 space-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.errorCount') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.error_count_sla ?? 0) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.businessLimited') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.business_limited_count ?? 0) }}</span>
            </div>
          </div>
        </div>

        <!-- Card 6: Upstream Errors -->
        <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 6;">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1">
              <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.upstreamErrors') }}</span>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.upstreamErrors')" />
            </div>
            <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('upstream')">
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>
          <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent))">
            {{ upstreamErrorRatePercent == null ? '-' : `${upstreamErrorRatePercent.toFixed(2)}%` }}
          </div>
          <div class="mt-3 space-y-1 text-xs">
            <div class="flex justify-between">
              <span class="text-gray-500">{{ t('admin.ops.errorCountExcl429529') }}:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber(overview.upstream_error_count_excl_429_529 ?? 0) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500">429/529:</span>
              <span class="font-bold text-gray-900 dark:text-white">{{ formatNumber((overview.upstream_429_count ?? 0) + (overview.upstream_529_count ?? 0)) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Integrated: System health (cards) -->
    <div v-if="overview" class="mt-2 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <!-- CPU -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">CPU</div>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.cpu')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="cpuPercentClass">
            {{ cpuPercentValue == null ? '-' : `${cpuPercentValue.toFixed(1)}%` }}
          </div>
          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('common.warning') }} 80% · {{ t('common.critical') }} 95%
          </div>
        </div>

        <!-- MEM -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.memory') }}</div>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.memory')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="memPercentClass">
            {{ memPercentValue == null ? '-' : `${memPercentValue.toFixed(1)}%` }}
          </div>
          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{
              systemMetrics?.memory_used_mb == null || systemMetrics?.memory_total_mb == null
                ? '-'
                : `${formatNumber(systemMetrics.memory_used_mb)} / ${formatNumber(systemMetrics.memory_total_mb)} MB`
            }}
          </div>
        </div>

        <!-- DB -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.db') }}</div>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.db')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="dbMiddleClass">
            {{ dbMiddleLabel }}
          </div>
          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.conns') }} {{ dbConnOpenValue ?? '-' }} / {{ dbMaxOpenConnsValue ?? '-' }}
            · {{ t('admin.ops.active') }} {{ dbConnActiveValue ?? '-' }}
            · {{ t('admin.ops.idle') }} {{ dbConnIdleValue ?? '-' }}
            <span v-if="dbConnWaitingValue != null"> · {{ t('admin.ops.waiting') }} {{ dbConnWaitingValue }} </span>
          </div>
        </div>

        <!-- Redis -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">Redis</div>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.redis')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="redisMiddleClass">
            {{ redisMiddleLabel }}
          </div>
          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.conns') }} {{ redisConnTotalValue ?? '-' }} / {{ redisPoolSizeValue ?? '-' }}
            <span v-if="redisConnActiveValue != null"> · {{ t('admin.ops.active') }} {{ redisConnActiveValue }} </span>
            <span v-if="redisConnIdleValue != null"> · {{ t('admin.ops.idle') }} {{ redisConnIdleValue }} </span>
          </div>
        </div>

        <!-- Goroutines -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center gap-1">
            <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.goroutines') }}</div>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.goroutines')" />
          </div>
          <div class="mt-1 text-lg font-black" :class="goroutineStatusClass">
            {{ goroutineStatusLabel }}
          </div>
          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.current') }} <span class="font-mono">{{ goroutineCountValue ?? '-' }}</span>
            · {{ t('common.warning') }} <span class="font-mono">{{ goroutinesWarnThreshold }}</span>
            · {{ t('common.critical') }} <span class="font-mono">{{ goroutinesCriticalThreshold }}</span>
            <span v-if="systemMetrics?.concurrency_queue_depth != null">
              · {{ t('admin.ops.queue') }} <span class="font-mono">{{ systemMetrics.concurrency_queue_depth }}</span>
            </span>
          </div>
        </div>

        <!-- Jobs -->
        <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-1">
              <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.jobs') }}</div>
              <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.jobs')" />
            </div>
            <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openJobsDetails">
              {{ t('admin.ops.requestDetails.details') }}
            </button>
          </div>

          <div class="mt-1 text-lg font-black" :class="jobsStatusClass">
            {{ jobsStatusLabel }}
          </div>

          <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
            {{ t('common.total') }} <span class="font-mono">{{ jobHeartbeats.length }}</span>
            · {{ t('common.warning') }} <span class="font-mono">{{ jobsWarnCount }}</span>
          </div>
        </div>
      </div>
    </div>

    <BaseDialog :show="showJobsDetails" :title="t('admin.ops.jobs')" width="wide" @close="showJobsDetails = false">
      <div v-if="!jobHeartbeats.length" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.noData') }}
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="hb in jobHeartbeats"
          :key="hb.job_name"
          class="rounded-xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ hb.job_name }}</div>
            <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
              <span v-if="hb.last_duration_ms != null" class="font-mono">{{ hb.last_duration_ms }}ms</span>
              <span>{{ formatTimeShort(hb.updated_at) }}</span>
            </div>
          </div>

          <div class="mt-2 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
            <div>
              {{ t('admin.ops.lastSuccess') }} <span class="font-mono">{{ formatTimeShort(hb.last_success_at) }}</span>
            </div>
            <div>
              {{ t('admin.ops.lastError') }} <span class="font-mono">{{ formatTimeShort(hb.last_error_at) }}</span>
            </div>
            <div>
              {{ t('admin.ops.result') }} <span class="font-mono">{{ hb.last_result || '-' }}</span>
            </div>
          </div>

          <div
            v-if="hb.last_error"
            class="mt-3 rounded-lg bg-rose-50 p-2 text-xs text-rose-700 dark:bg-rose-900/20 dark:text-rose-300"
          >
            {{ hb.last_error }}
          </div>
        </div>
      </div>
    </BaseDialog>

    <!-- Custom Time Range Dialog -->
    <BaseDialog :show="showCustomTimeRangeDialog" :title="t('admin.ops.timeRange.custom')" width="narrow" @close="handleCustomTimeRangeCancel">
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            {{ t('admin.ops.customTimeRange.startTime') }}
          </label>
          <input
            v-model="customStartTimeInput"
            type="datetime-local"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            {{ t('admin.ops.customTimeRange.endTime') }}
          </label>
          <input
            v-model="customEndTimeInput"
            type="datetime-local"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
          />
        </div>
        <div class="flex justify-end gap-3 pt-2">
          <button
            type="button"
            class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
            @click="handleCustomTimeRangeCancel"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="rounded-lg bg-blue-500 px-4 py-2 text-sm font-medium text-white hover:bg-blue-600"
            @click="handleCustomTimeRangeConfirm"
          >
            {{ t('common.confirm') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>
