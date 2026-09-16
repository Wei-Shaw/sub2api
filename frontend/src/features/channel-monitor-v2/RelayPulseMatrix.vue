<template>
  <section
    class="card flex min-h-[360px] flex-col overflow-visible !rounded-3xl !border-0 !p-6 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700"
  >
    <div class="card-header mb-4 flex shrink-0 flex-wrap items-start justify-between gap-3 !border-0 !p-0">
      <div class="min-w-0">
        <h2 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
          <span class="inline-flex h-4 w-4 text-emerald-500" aria-hidden="true">
            <Icon name="grid" size="sm" />
          </span>
          {{ t('channelMonitorV2.matrix.title') }}
        </h2>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ t('channelMonitorV2.matrix.description') }}
        </p>
      </div>
      <div class="flex w-full min-w-0 flex-wrap items-center justify-end gap-2 text-xs text-gray-500 dark:text-gray-400 sm:w-auto">
        <span class="badge badge-gray shrink-0">{{ bucketLabel }}</span>
        <span class="hidden text-[11px] text-gray-400 dark:text-dark-400 sm:inline">{{ t('channelMonitorV2.matrix.wheelZoomX') }}</span>
        <button
          type="button"
          class="inline-flex shrink-0 items-center rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-800"
          :disabled="!zoomed"
          @click="resetMatrixZoom"
        >
          {{ t('channelMonitorV2.matrix.resetZoom') }}
        </button>
      </div>
    </div>

    <div class="card-body min-h-0 flex-1 !p-0">
      <div
        v-if="rows.length"
        ref="scrollRef"
        class="matrix-scroll max-h-[min(42vh,420px)] max-w-full overflow-auto rounded-2xl bg-gray-50/60 p-2 dark:bg-dark-900/30"
        @wheel="onMatrixWheel"
      >
        <div class="matrix-table w-full" :style="tableStyle">
          <div
            class="matrix-header matrix-row sticky top-0 z-[3] bg-gray-50 text-[10px] font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900 dark:text-gray-400"
            :class="showThroughput ? 'matrix-row--with-tps' : ''"
          >
            <span>{{ t('channelMonitorV2.matrix.dimension') }}</span>
            <span>{{ t('channelMonitorV2.metrics.successRate') }}</span>
            <span>{{ t('channelMonitorV2.metrics.ttft') }}</span>
            <span v-if="showThroughput">{{ t('channelMonitorV2.metrics.tps') }}</span>
            <span>{{ t('channelMonitorV2.metrics.cacheRate') }}</span>
            <span class="pulse-axis flex justify-between gap-3">
              <i class="not-italic">{{ axisStart }}</i>
              <i class="not-italic">{{ axisEnd }}</i>
            </span>
          </div>
          <div
            v-for="entry in alignedRows"
            :key="rowKey(entry.row)"
            class="matrix-row border-b border-gray-100/80 dark:border-dark-700/60"
            :class="showThroughput ? 'matrix-row--with-tps' : ''"
          >
            <div class="dimension-cell flex min-w-0 items-center gap-2 bg-white dark:bg-dark-800" :title="rowLabel(entry.row)">
              <span :class="['status-dot', cellClass(entry.row.health, entry.row.metrics.request_count)]"></span>
              <strong class="truncate text-xs font-semibold text-gray-800 dark:text-gray-100">{{ rowLabel(entry.row) }}</strong>
            </div>
            <strong class="summary-value bg-white text-xs font-medium tabular-nums text-gray-600 dark:bg-dark-800 dark:text-gray-300">
              {{ successRate(entry.row.metrics) }}
            </strong>
            <strong
              class="summary-value bg-white text-xs font-medium tabular-nums text-gray-600 dark:bg-dark-800 dark:text-gray-300"
              :title="latencyPrivacy(entry.row.metrics.ttft)"
            >
              {{ formatMs(entry.row.metrics.ttft.p50_ms) }}
            </strong>
            <strong
              v-if="showThroughput"
              class="summary-value bg-white text-xs font-medium tabular-nums text-gray-600 dark:bg-dark-800 dark:text-gray-300"
              :title="exactTps(entry.row.metrics.tpm)"
            >
              {{ formatTps(entry.row.metrics.tpm) }}
            </strong>
            <strong
              class="summary-value bg-white text-xs font-medium tabular-nums text-gray-600 dark:bg-dark-800 dark:text-gray-300"
            >
              {{ formatPercent(entry.row.metrics.cache_rate) }}
            </strong>
            <div class="pulse-track grid items-stretch" :style="pulseStyle">
              <span
                v-for="slot in entry.slots"
                :key="slot.start"
                class="pulse-cell relative rounded-sm border-0 p-0 outline-offset-1"
                :class="[
                  slot.bucket ? cellClass(slot.bucket.health, slot.bucket.metrics.request_count) : 'health-unknown',
                  slot.bucket ? 'has-data' : 'is-empty',
                ]"
                tabindex="0"
                role="img"
                :title="slot.bucket ? bucketTooltip(slot.bucket) : t('channelMonitorV2.matrix.noTrafficAt', { time: formatBucketRange(slot.start) })"
                :aria-label="slot.bucket ? bucketTooltip(slot.bucket) : t('channelMonitorV2.matrix.noTrafficAt', { time: formatBucketRange(slot.start) })"
                @mouseenter="showTooltip($event, slot)"
                @mousemove="moveTooltip($event)"
                @mouseleave="hideTooltip"
                @focus="showTooltip($event, slot)"
                @blur="hideTooltip"
              >
                <span class="pulse-tooltip" role="tooltip">
                  <template v-if="slot.bucket">
                    <span class="pulse-tooltip-line pulse-tooltip-title">{{ formatBucketRange(slot.start) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.matrix.scoreLine', { score: formatScore(slot.bucket.health) }) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.successRateValue', { value: successRate(slot.bucket.metrics) }) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.ttftValue', { value: latencyPrivacy(slot.bucket.metrics.ttft) }) }}</span>
                    <span v-if="showThroughput" class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.tpsValue', { value: formatTps(slot.bucket.metrics.tpm) }) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.cacheRateValue', { value: formatPercent(slot.bucket.metrics.cache_rate) }) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(slot.bucket.metrics.error_rate) }) }}</span>
                    <span v-if="showThroughput" class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.rpmValue', { value: formatRate(slot.bucket.metrics.rpm) }) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.metrics.durationValue', { value: latencyPrivacy(slot.bucket.metrics.duration) }) }}</span>
                  </template>
                  <template v-else>
                    <span class="pulse-tooltip-line pulse-tooltip-title">{{ formatBucketRange(slot.start) }}</span>
                    <span class="pulse-tooltip-line">{{ t('channelMonitorV2.matrix.noTraffic') }}</span>
                  </template>
                </span>
              </span>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="flex min-h-[200px] items-center justify-center py-8">
        <EmptyState
          :title="t('channelMonitorV2.matrix.emptyTitle')"
          :description="t('channelMonitorV2.empty.description')"
        />
      </div>

      <div class="mt-4 flex flex-col gap-2" :aria-label="t('channelMonitorV2.matrix.legendAria')">
        <div class="flex items-center gap-2 text-[11px] text-gray-500 dark:text-gray-400">
          <span class="shrink-0">{{ t('channelMonitorV2.matrix.bad') }}</span>
          <div class="score-legend h-2.5 flex-1 overflow-hidden rounded-full"></div>
          <span class="shrink-0">{{ t('channelMonitorV2.matrix.good') }}</span>
        </div>
        <div class="flex flex-wrap gap-4 text-[11px] text-gray-500 dark:text-gray-400">
          <span class="inline-flex items-center gap-1.5"><i class="status-dot health-score10"></i>{{ t('channelMonitorV2.matrix.healthyLegend') }}</span>
          <span class="inline-flex items-center gap-1.5"><i class="status-dot health-score6"></i>{{ t('channelMonitorV2.matrix.warningLegend') }}</span>
          <span class="inline-flex items-center gap-1.5"><i class="status-dot health-score2"></i>{{ t('channelMonitorV2.matrix.criticalLegend') }}</span>
          <span class="inline-flex items-center gap-1.5"><i class="status-dot health-unknown"></i>{{ t('channelMonitorV2.matrix.unknownLegend') }}</span>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="floatingTooltip.visible"
        class="matrix-floating-tooltip"
        :style="{ left: `${floatingTooltip.x}px`, top: `${floatingTooltip.y}px` }"
        role="tooltip"
      >
        <span
          v-for="(line, index) in floatingTooltip.lines"
          :key="`${index}:${line}`"
          class="matrix-floating-tooltip-line"
          :class="index === 0 ? 'matrix-floating-tooltip-title' : ''"
        >
          {{ line }}
        </span>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, reactive, ref, watch } from 'vue'
import type {
  LatencyMetric,
  MonitorCoverage,
  MonitorHealth,
  MonitorMatrixBucket,
  MonitorMatrixRow,
  MonitorMetric,
} from '@/api/channelMonitorV2'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  formatLatencyPrivacy,
  formatMonitorMs,
  formatMonitorPercent,
  formatMonitorSuccessRateFromError,
  formatMonitorThroughput,
  formatMonitorTokensPerSecond,
  tokensPerSecondFromTpm,
  healthModeScore,
  healthScoreClass,
} from '@/features/channel-monitor-v2/monitorFormat'
import {
  applyWheelZoom,
  clientXRatio,
  isZoomed,
  resetZoom,
  sliceByZoom,
  type ZoomState,
} from '@/features/channel-monitor-v2/monitorZoom'

type HealthMode = 'overall' | 'success' | 'ttft' | 'cache'
const { t, locale } = useI18n()

const props = withDefaults(
  defineProps<{
    rows: MonitorMatrixRow[]
    coverage: MonitorCoverage
    healthMode: HealthMode
    /** When false, RPM/TPM are omitted from tooltips (user scale privacy). */
    showThroughput?: boolean
  }>(),
  { showThroughput: true },
)

type AlignedSlot = { start: string; bucket?: MonitorMatrixBucket }

const floatingTooltip = reactive({
  visible: false,
  x: 0,
  y: 0,
  lines: [] as string[],
})

const scrollRef = ref<HTMLElement | null>(null)
const zoom = ref<ZoomState>(resetZoom())
const zoomed = computed(() => isZoomed(zoom.value))

const allBucketStarts = computed(() => {
  // X-axis always spans the UI-selected range [requested_start, requested_end).
  // Partial backfill leaves empty cells until coverage_start/data_through fill in.
  const step = Math.max(60, props.coverage.bucket_seconds) * 1000
  const requestedStart = new Date(props.coverage.requested_start).getTime()
  const requestedEndRaw = props.coverage.requested_end
    ? new Date(props.coverage.requested_end).getTime()
    : NaN
  // Fallback for older payloads without requested_end.
  const dataThrough = new Date(props.coverage.data_through).getTime()
  const end = Number.isFinite(requestedEndRaw) && requestedEndRaw > requestedStart
    ? requestedEndRaw
    : dataThrough
  if (![requestedStart, end].every(Number.isFinite) || requestedStart >= end) return []
  const starts: string[] = []
  for (let cursor = Math.floor(requestedStart / step) * step; cursor < end; cursor += step) {
    starts.push(new Date(cursor).toISOString())
  }
  return starts
})
/** Visible bucket window after X zoom (cursor-centered), not always the tail. */
const bucketStarts = computed(() => sliceByZoom(allBucketStarts.value, zoom.value))
const tableStyle = computed(() => ({
  '--bucket-count': String(Math.max(1, bucketStarts.value.length)),
  minWidth: zoomed.value ? `calc(260px + ${pulseMinWidth.value})` : '0',
}))
const pulseMinWidth = computed(() => {
  const count = Math.max(1, bucketStarts.value.length)
  if (!zoomed.value) return '0px'
  // Zoom in = fewer columns + wider min cell (span shrinks → intensity grows).
  const intensity = Math.min(12, Math.round((1 - zoom.value.span) / 0.08))
  const width = 6 + intensity * 4
  const gap = intensity >= 4 ? 3 : 2
  return `${count * width + Math.max(0, count - 1) * gap}px`
})
const pulseStyle = computed(() => {
  const count = Math.max(1, bucketStarts.value.length)
  const intensity = zoomed.value ? Math.min(12, Math.round((1 - zoom.value.span) / 0.08)) : 0
  const gapPx = !zoomed.value ? (count > 24 ? 1 : 2) : intensity >= 4 ? 3 : 2
  const heightPx = 16
  // Unzoomed: equal flex fractions. Zoomed: enforce growing min width so blocks lengthen.
  const minCell = !zoomed.value ? '0' : `${6 + intensity * 4}px`
  return {
    gridTemplateColumns: `repeat(${count}, minmax(${minCell}, 1fr))`,
    gap: `${gapPx}px`,
    height: `${heightPx}px`,
    minWidth: pulseMinWidth.value,
  }
})
const axisStart = computed(() =>
  bucketStarts.value.length ? formatAxisTime(bucketStarts.value[0]) : '时间脉冲'
)
const axisEnd = computed(() =>
  bucketStarts.value.length ? formatAxisTime(bucketStarts.value[bucketStarts.value.length - 1]) : ''
)
const bucketLabel = computed(() => {
  const minutes = props.coverage.bucket_seconds / 60
  if (minutes < 60) return t('channelMonitorV2.bucket.minutes', { count: minutes })
  const hours = minutes / 60
  if (hours < 24) return t('channelMonitorV2.bucket.hours', { count: hours })
  return t('channelMonitorV2.bucket.days', { count: hours / 24 })
})

/** Shared ISO start → column index for the visible window (rebuilt when zoom/coverage changes). */
const bucketStartIndex = computed(() => {
  const map = new Map<string, number>()
  bucketStarts.value.forEach((start, index) => map.set(start, index))
  return map
})

/** Pre-aligned sparse slots per row so wheel zoom does not rebuild Maps every paint. */
const alignedRows = computed(() => {
  const starts = bucketStarts.value
  const indexByStart = bucketStartIndex.value
  return props.rows.map((row) => {
    const slots: AlignedSlot[] = starts.map((start) => ({ start }))
    for (const bucket of row.buckets || []) {
      const key = new Date(bucket.bucket_start).toISOString()
      const index = indexByStart.get(key)
      if (index != null) slots[index] = { start: starts[index], bucket }
    }
    return { row, slots }
  })
})

function onMatrixWheel(event: WheelEvent) {
  const track = scrollRef.value
  const target = event.target as HTMLElement | null
  const pulse = target?.closest('.pulse-track') as HTMLElement | null
  const overMatrix = Boolean(target?.closest('.matrix-scroll'))
  // Plain vertical wheel over the matrix zooms X (narrower range → wider cells).
  // Shift+wheel or horizontal delta pans; leave non-matrix page scroll alone.
  const isPan = event.shiftKey || Math.abs(event.deltaX) > Math.abs(event.deltaY)
  if (!overMatrix && !pulse) return
  // When not zoomed and user scrolls vertically outside pulse, still zoom if over matrix body.
  if (!overMatrix && !isPan) return
  event.preventDefault()
  const ratioEl = pulse || track
  const ratio = clientXRatio(event.clientX, ratioEl)
  zoom.value = applyWheelZoom(zoom.value, event, ratio)
}

function resetMatrixZoom() {
  zoom.value = resetZoom()
}

watch(
  () => [
    props.coverage.requested_start,
    props.coverage.requested_end,
    props.coverage.coverage_start,
    props.coverage.data_through,
    props.coverage.bucket_seconds,
  ],
  () => {
    zoom.value = resetZoom()
  },
)

function cellClass(health: MonitorHealth, requestCount: number): string {
  return healthScoreClass(health, props.healthMode, requestCount)
}

function rowLabel(row: MonitorMatrixRow): string {
  const parts = [row.platform]
  if (row.group_name || row.group_id) parts.push(row.group_name || `#${row.group_id}`)
  if (row.model) parts.push(row.model === '__other__' ? t('channelMonitorV2.otherModels') : row.model)
  return parts.join(' / ')
}

function rowKey(row: MonitorMatrixRow): string {
  return [row.platform, row.group_id || 0, row.model || ''].join(':')
}

function successRate(metrics: MonitorMetric): string {
  // Empty traffic: no request count and no throughput signal.
  // When throughput is hidden for privacy, still show success from error_rate.
  const noCount = metrics.request_count <= 0
  const noTP = (metrics.rpm || 0) <= 0 && (metrics.tpm || 0) <= 0
  if (noCount && noTP && props.showThroughput) return '-'
  return formatMonitorSuccessRateFromError(metrics.error_rate)
}

function formatScore(health: MonitorHealth): string {
  const score = healthModeScore(health, props.healthMode)
  if (score == null) return '—'
  return `${Math.round(score)}`
}

function bucketTooltip(bucket: MonitorMatrixBucket): string {
  return bucketTooltipLines(bucket).join('\n')
}

function bucketTooltipLines(bucket: MonitorMatrixBucket): string[] {
  const metrics = bucket.metrics
  const lines = [
    formatBucketRange(bucket.bucket_start),
    t('channelMonitorV2.matrix.scoreLine', { score: formatScore(bucket.health) }),
    t('channelMonitorV2.metrics.successRateValue', { value: successRate(metrics) }),
    t('channelMonitorV2.metrics.ttftValue', { value: latencyPrivacy(metrics.ttft) }),
  ]
  if (props.showThroughput) {
    lines.push(t('channelMonitorV2.metrics.tpsValue', { value: formatTps(metrics.tpm) }))
  }
  lines.push(
    t('channelMonitorV2.metrics.cacheRateValue', { value: formatPercent(metrics.cache_rate) }),
    t('channelMonitorV2.metrics.errorRateValue', { value: formatPercent(metrics.error_rate) }),
  )
  if (props.showThroughput) {
    lines.push(t('channelMonitorV2.metrics.rpmValue', { value: formatRate(metrics.rpm) }))
  }
  lines.push(t('channelMonitorV2.metrics.durationValue', { value: latencyPrivacy(metrics.duration) }))
  return lines
}
function emptyTooltipLines(start: string): string[] {
  return [formatBucketRange(start), t('channelMonitorV2.matrix.noTraffic')]
}

function showTooltip(event: MouseEvent | FocusEvent, slot: AlignedSlot) {
  floatingTooltip.lines = slot.bucket ? bucketTooltipLines(slot.bucket) : emptyTooltipLines(slot.start)
  floatingTooltip.visible = true
  positionTooltip(event)
}

function moveTooltip(event: MouseEvent) {
  if (!floatingTooltip.visible) return
  positionTooltip(event)
}

function hideTooltip() {
  floatingTooltip.visible = false
}

function positionTooltip(event: MouseEvent | FocusEvent) {
  if ('clientX' in event) {
    floatingTooltip.x = Math.min(window.innerWidth - 12, Math.max(12, event.clientX))
    floatingTooltip.y = Math.min(window.innerHeight - 12, Math.max(12, event.clientY)) - 12
    return
  }
  const target = event.target as HTMLElement | null
  const rect = target?.getBoundingClientRect()
  if (!rect) return
  floatingTooltip.x = rect.left + rect.width / 2
  floatingTooltip.y = rect.top - 10
}

function latencyPrivacy(metric: LatencyMetric) {
  return formatLatencyPrivacy(metric.p50_ms, metric.p90_ms, metric.avg_ms, metric.p95_ms)
}

function formatPercent(value: number) {
  return formatMonitorPercent(value)
}

function formatRate(value: number) {
  return formatMonitorThroughput(value)
}

function formatTps(tpm: number | null | undefined) {
  return formatMonitorTokensPerSecond(tpm)
}

function exactTps(tpm: number | null | undefined) {
  const tps = tokensPerSecondFromTpm(tpm)
  return Intl.NumberFormat(locale.value || undefined, { maximumFractionDigits: 3 }).format(tps)
}

function formatMs(value: number | null) {
  return formatMonitorMs(value)
}

function formatAxisTime(value: string) {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function formatBucketRange(value: string) {
  const start = new Date(value)
  const end = new Date(start.getTime() + props.coverage.bucket_seconds * 1000)
  return `${formatAxisTime(start.toISOString())} - ${new Intl.DateTimeFormat(locale.value || undefined, { hour: '2-digit', minute: '2-digit' }).format(end)}`
}
</script>

<style scoped>
/* dimension | success | ttft | cache | pulse */
.matrix-row {
  display: grid;
  grid-template-columns:
    minmax(120px, 1.2fr)
    minmax(52px, 0.34fr)
    minmax(58px, 0.36fr)
    minmax(52px, 0.34fr)
    minmax(120px, 2.8fr);
  align-items: center;
  gap: 0.5rem clamp(0.25rem, 0.8vw, 0.625rem);
  min-height: 2.25rem;
}
/* + tokens/s column when throughput visible */
.matrix-row--with-tps {
  grid-template-columns:
    minmax(110px, 1.15fr)
    minmax(48px, 0.3fr)
    minmax(54px, 0.32fr)
    minmax(58px, 0.36fr)
    minmax(48px, 0.3fr)
    minmax(120px, 2.6fr);
}
.matrix-table,
.pulse-track {
  min-width: 0;
}
.status-dot {
  display: inline-block;
  height: 0.5rem;
  width: 0.5rem;
  flex: none;
  border-radius: 9999px;
}

/* Multi-stop green → yellow → red (score10 best … score0 worst) */
.health-score10 { background: #16a34a; }
.health-score9  { background: #22c55e; }
.health-score8  { background: #4ade80; }
.health-score7  { background: #a3e635; }
.health-score6  { background: #facc15; }
.health-score5  { background: #fbbf24; }
.health-score4  { background: #f59e0b; }
.health-score3  { background: #f97316; }
.health-score2  { background: #fb7185; }
.health-score1  { background: #f87171; }
.health-score0  { background: rgb(239, 67, 67); }
/* Coarse fallbacks (older payloads without score) */
.health-healthy  { background: #22c55e; }
.health-warning  { background: #f59e0b; }
.health-critical { background: #ef4444; }
.health-unknown  { background: #9ca3af; }

.score-legend {
  background: linear-gradient(
    90deg,
    rgb(239, 67, 67) 0%,
    #f87171 15%,
    #f97316 30%,
    #f59e0b 45%,
    #facc15 55%,
    #a3e635 70%,
    #22c55e 85%,
    #16a34a 100%
  );
}

.pulse-cell {
  position: relative;
  min-width: 0;
}
.pulse-cell.has-data {
  cursor: help;
}
.pulse-cell.is-empty {
  opacity: 0.55;
  cursor: default;
}
.pulse-cell.has-data:hover,
.pulse-cell.has-data:focus-visible {
  outline: 2px solid rgb(var(--color-primary-500, 99 102 241) / 0.55);
  outline-offset: 1px;
  z-index: 5;
}

/* CSS-only hover tooltip — no click modal, no absolute request counts.
   Native title is also provided so dense/scrolling layouts can always show the
   full content even when a browser clips transformed children. */
.pulse-tooltip {
  pointer-events: none;
  position: absolute;
  bottom: calc(100% + 8px);
  left: 50%;
  z-index: 40;
  min-width: 11.5rem;
  max-width: 16rem;
  transform: translateX(-50%) translateY(4px);
  border-radius: 0.75rem;
  border: 1px solid rgb(229 231 235);
  background: rgb(255 255 255);
  padding: 0.5rem 0.625rem;
  box-shadow: 0 10px 25px -5px rgb(0 0 0 / 0.15);
  opacity: 0;
  visibility: hidden;
  transition: opacity 0.12s ease, transform 0.12s ease, visibility 0.12s;
  white-space: nowrap;
}
:global(.dark) .pulse-tooltip {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39);
  color: rgb(229 231 235);
}
.pulse-tooltip-line {
  display: block;
  font-size: 11px;
  line-height: 1.45;
  color: rgb(75 85 99);
}
:global(.dark) .pulse-tooltip-line {
  color: rgb(209 213 219);
}
.pulse-tooltip-title {
  margin-bottom: 0.2rem;
  font-weight: 600;
  color: rgb(17 24 39);
}
:global(.dark) .pulse-tooltip-title {
  color: rgb(243 244 246);
}
.pulse-cell:hover .pulse-tooltip,
.pulse-cell:focus-visible .pulse-tooltip {
  opacity: 1;
  visibility: visible;
  transform: translateX(-50%) translateY(0);
}
/* Keep semantic/test text in-cell, but render the visible tooltip through body
   Teleport so it cannot be clipped by the matrix viewport. */
.pulse-tooltip {
  display: none;
}
.matrix-floating-tooltip {
  pointer-events: none;
  position: fixed;
  z-index: 9999;
  min-width: 11.5rem;
  max-width: min(18rem, calc(100vw - 1.5rem));
  transform: translate(-50%, -100%);
  border-radius: 0.75rem;
  border: 1px solid rgb(229 231 235);
  background: rgb(255 255 255);
  padding: 0.5rem 0.625rem;
  box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.28);
  white-space: nowrap;
}
:global(.dark) .matrix-floating-tooltip {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39);
  color: rgb(229 231 235);
}
.matrix-floating-tooltip-line {
  display: block;
  font-size: 11px;
  line-height: 1.45;
  color: rgb(75 85 99);
}
:global(.dark) .matrix-floating-tooltip-line {
  color: rgb(209 213 219);
}
.matrix-floating-tooltip-title {
  margin-bottom: 0.2rem;
  font-weight: 600;
  color: rgb(17 24 39);
}
:global(.dark) .matrix-floating-tooltip-title {
  color: rgb(243 244 246);
}

@media (max-width: 640px) {
  .matrix-row {
    grid-template-columns: minmax(88px, 1fr) minmax(48px, 0.45fr) minmax(54px, 0.5fr) minmax(96px, 2.6fr);
    gap: 0.35rem;
  }
  .matrix-row > :nth-child(2) {
    left: 0;
  }
  .matrix-row > :nth-child(3) {
    left: 0;
  }
}
</style>
