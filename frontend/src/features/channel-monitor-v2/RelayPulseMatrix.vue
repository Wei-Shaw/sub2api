<template>
  <section class="flex min-h-[320px] min-w-0 flex-col rounded border border-line bg-surface">
    <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 border-b border-line px-4 py-3">
      <div class="min-w-0">
        <h2 class="truncate text-sm font-semibold text-ink">
          {{ t('channelMonitorV2.matrix.title') }}
        </h2>
        <p class="mt-0.5 text-xs text-ink-tertiary">
          {{ t('channelMonitorV2.matrix.description') }}
        </p>
      </div>
      <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
        <Badge class="shrink-0">{{ bucketLabel }}</Badge>
        <span class="hidden shrink-0 text-2xs text-ink-tertiary sm:inline">
          {{ t('channelMonitorV2.matrix.wheelZoomX') }}
        </span>
        <Button size="xs" class="shrink-0" :disabled="!zoomed" @click="resetMatrixZoom">
          {{ t('channelMonitorV2.matrix.resetZoom') }}
        </Button>
      </div>
    </header>

    <div class="min-h-0 flex-1">
      <div
        v-if="rows.length"
        ref="scrollRef"
        class="matrix-scroll max-h-[min(42vh,420px)] max-w-full overflow-auto"
        @wheel="onMatrixWheel"
      >
        <div class="matrix-table w-full" :style="tableStyle">
          <div
            class="matrix-header matrix-row sticky top-0 z-[3] border-b border-line-strong bg-surface-sunken text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
            :class="showThroughput ? 'matrix-row--with-tps' : ''"
          >
            <span class="truncate">{{ t('channelMonitorV2.matrix.dimension') }}</span>
            <span class="text-right">{{ t('channelMonitorV2.metrics.successRate') }}</span>
            <span class="text-right">{{ t('channelMonitorV2.metrics.ttft') }}</span>
            <span v-if="showThroughput" class="text-right">{{ t('channelMonitorV2.metrics.tps') }}</span>
            <span class="text-right">{{ t('channelMonitorV2.metrics.cacheRate') }}</span>
            <span class="pulse-axis flex justify-between gap-3 normal-case tracking-normal">
              <i class="not-italic">{{ axisStart }}</i>
              <i class="not-italic">{{ axisEnd }}</i>
            </span>
          </div>
          <div
            v-for="entry in alignedRows"
            :key="rowKey(entry.row)"
            class="matrix-row border-b border-line-subtle"
            :class="showThroughput ? 'matrix-row--with-tps' : ''"
          >
            <div class="dimension-cell flex min-w-0 items-center" :title="rowLabel(entry.row)">
              <span class="truncate text-xs text-ink">{{ rowLabel(entry.row) }}</span>
            </div>
            <!--
              Every quantity goes through NumCell: locale grouping, tabular
              figures, the unit a step down, the unrounded value on hover, and
              — the one that matters on a monitor — an en dash for "not
              measured" so it can never be read as a measured zero.

              Tone appears only when a row has crossed a declared threshold.
              A matrix where every healthy row is green has no signal left for
              the rows that are not.
            -->
            <NumCell
              :value="successPercent(entry.row.metrics)"
              :precision="1"
              unit="%"
              :tone="rowTone(entry.row)"
            />
            <NumCell
              :value="hasTraffic(entry.row.metrics) ? entry.row.metrics.ttft.p50_ms : null"
              :precision="0"
              unit="ms"
              :title="latencyPrivacy(entry.row.metrics.ttft)"
            />
            <NumCell
              v-if="showThroughput"
              :value="throughputValue(entry.row.metrics)"
              :precision="1"
            />
            <NumCell
              :value="hasTraffic(entry.row.metrics) ? entry.row.metrics.cache_rate * 100 : null"
              :precision="1"
              unit="%"
            />
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
    </div>

    <!--
      Legend. The old version was a `linear-gradient` strip, which claims the
      scale is continuous when the bands are discrete. Eleven swatches say what
      the matrix actually draws, and every named band carries a text label so
      the ramp survives grayscale.
    -->
    <footer
      class="flex flex-col gap-2 border-t border-line px-4 py-3"
      :aria-label="t('channelMonitorV2.matrix.legendAria')"
    >
      <div class="flex items-center gap-2 text-2xs text-ink-tertiary">
        <span class="shrink-0">{{ t('channelMonitorV2.matrix.bad') }}</span>
        <span class="score-legend flex flex-1 gap-px" aria-hidden="true">
          <i v-for="band in legendBands" :key="band" class="h-2 flex-1" :class="band"></i>
        </span>
        <span class="shrink-0">{{ t('channelMonitorV2.matrix.good') }}</span>
      </div>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-2xs text-ink-tertiary">
        <span
          v-for="item in legendEntries"
          :key="item.label"
          class="inline-flex items-center gap-1.5"
        >
          <i class="h-2 w-2 shrink-0" :class="item.band" aria-hidden="true"></i>{{ item.label }}
        </span>
      </div>
    </footer>

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
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import NumCell from '@/components/common/NumCell.vue'
import type { Tone } from '@/components/common/primitives'
import {
  formatLatencyPrivacy,
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

/** Worst → best, matching the `.health-score*` ramp declared below. */
const legendBands = [
  'health-score0',
  'health-score1',
  'health-score2',
  'health-score3',
  'health-score4',
  'health-score5',
  'health-score6',
  'health-score7',
  'health-score8',
  'health-score9',
  'health-score10',
] as const

const legendEntries = computed(() => [
  { band: 'health-score10', label: t('channelMonitorV2.matrix.healthyLegend') },
  { band: 'health-score6', label: t('channelMonitorV2.matrix.warningLegend') },
  { band: 'health-score2', label: t('channelMonitorV2.matrix.criticalLegend') },
  { band: 'health-unknown', label: t('channelMonitorV2.matrix.unknownLegend') },
])

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
  bucketStarts.value.length ? formatAxisTime(bucketStarts.value[0]) : ''
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

/**
 * Row-level tone for the success column. Neutral until the row's health score
 * drops below the watch band — colour marks the exception, never the norm.
 */
function rowTone(row: MonitorMatrixRow): Tone {
  if (!hasTraffic(row.metrics)) return 'neutral'
  const score = healthModeScore(row.health, props.healthMode)
  if (score == null) return 'neutral'
  if (score < 50) return 'danger'
  if (score < 80) return 'warn'
  return 'neutral'
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

/**
 * Empty traffic: no request count and no throughput signal. When throughput is
 * hidden for privacy the count alone cannot prove absence, so the row is still
 * treated as measured and success is derived from error_rate.
 */
function hasTraffic(metrics: MonitorMetric): boolean {
  if (metrics.request_count > 0) return true
  if ((metrics.rpm || 0) > 0 || (metrics.tpm || 0) > 0) return true
  return !props.showThroughput
}

/** Success as a percentage, or null when nothing was measured. */
function successPercent(metrics: MonitorMetric): number | null {
  if (!hasTraffic(metrics)) return null
  return (1 - (metrics.error_rate || 0)) * 100
}

function throughputValue(metrics: MonitorMetric): number | null {
  if (!hasTraffic(metrics)) return null
  return tokensPerSecondFromTpm(metrics.tpm)
}

function successRate(metrics: MonitorMetric): string {
  if (!hasTraffic(metrics)) return '–'
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
  gap: 0 clamp(0.25rem, 0.8vw, 0.625rem);
  padding: 0 1rem;
  min-height: var(--ds-row-h);
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
.matrix-header {
  min-height: var(--ds-header-h);
}
/* Hover moves the ground and nothing else. No zebra: a striped table spends
   its background channel on decoration, leaving none for state. */
.matrix-row:not(.matrix-header):hover {
  background-color: rgb(var(--ds-surface-hover));
}
.matrix-table,
.pulse-track {
  min-width: 0;
}

/*
 * HEALTH BANDS — the single definition in the app.
 *
 * `ChannelStatusV2View.vue` carried a near-identical 16-declaration copy of
 * this ramp, so fixing a band meant editing two files and noticing. The view
 * now renders row state through the `StatusDot` primitive and owns no band CSS
 * at all; the component that owns the matrix owns the scale.
 *
 * The ramp is NOT green→yellow→red. A healthy monitor is almost entirely
 * healthy, and a wall of green spends the whole colour budget on the cells
 * that need no attention — by the time something breaks the eye has nothing
 * left to catch. Good is a neutral hairline grey that steps in weight; colour
 * starts only where the score enters the watch band. Every cell also carries
 * its score in `title`/`aria-label`, so the ramp is never the only channel.
 *
 * The three groupings track the thresholds the legend states (healthy ≥80,
 * watch 50–79, critical <50) as closely as `scoreToBand()` allows:
 * `round(score / 10)` makes score8 span 75–84 and score5 span 45–54, so each
 * boundary lands within five points of its label and the two errors are
 * symmetric. Retuning further would mean tinting genuinely-healthy cells.
 */
.health-score10 { background: rgb(var(--ds-line)); }
.health-score9  { background: rgb(var(--ds-line-strong)); }
.health-score8  { background: rgb(var(--ds-line-emphasis)); }
.health-score7  { background: rgb(var(--ds-warn) / 0.45); }
.health-score6  { background: rgb(var(--ds-warn) / 0.7); }
.health-score5  { background: rgb(var(--ds-warn)); }
.health-score4  { background: rgb(var(--ds-danger) / 0.45); }
.health-score3  { background: rgb(var(--ds-danger) / 0.58); }
.health-score2  { background: rgb(var(--ds-danger) / 0.72); }
.health-score1  { background: rgb(var(--ds-danger) / 0.86); }
.health-score0  { background: rgb(var(--ds-danger)); }
/* Coarse fallbacks (older payloads without score) */
.health-healthy  { background: rgb(var(--ds-line-strong)); }
.health-warning  { background: rgb(var(--ds-warn)); }
.health-critical { background: rgb(var(--ds-danger)); }
.health-unknown  { background: rgb(var(--ds-surface-sunken)); box-shadow: inset 0 0 0 1px rgb(var(--ds-line-subtle)); }

.pulse-cell {
  position: relative;
  min-width: 0;
}
.pulse-cell.has-data {
  cursor: help;
}
.pulse-cell.is-empty {
  cursor: default;
}
/*
 * Hover affordance only. Focus is the global `outline` in style.css, which
 * already uses `--ds-focus`; this rule used to read
 * `rgb(var(--color-primary-500, 99 102 241) / 0.55)` and `--color-primary-500`
 * is defined nowhere in the repo (the token family is `--ds-*`), so the
 * fallback indigo always won and the outline never followed the accent.
 */
.pulse-cell.has-data:hover {
  outline: 1px solid rgb(var(--ds-ink));
  outline-offset: 1px;
  z-index: 5;
}

/*
 * In-cell tooltip text. Never painted — the visible tooltip is the body
 * Teleport below, so the matrix viewport cannot clip it — but the markup stays
 * so the cell keeps its full textual content. All of its former chrome (two
 * radii, a shadow, and eight hand-written `.dark` colour branches) is gone
 * with it.
 */
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
  border-radius: var(--ds-radius);
  border: 1px solid rgb(var(--ds-line));
  background: rgb(var(--ds-surface-raised));
  padding: var(--ds-space-4) var(--ds-space-5);
  box-shadow: var(--ds-shadow-popover);
  white-space: nowrap;
}
.matrix-floating-tooltip-line {
  display: block;
  font-family: var(--ds-font-mono);
  font-size: var(--ds-text-2xs);
  line-height: var(--ds-lh-2xs);
  color: rgb(var(--ds-ink-secondary));
}
.matrix-floating-tooltip-title {
  margin-bottom: var(--ds-space-1);
  font-weight: 600;
  color: rgb(var(--ds-ink));
}

@media (max-width: 1023px) {
  .matrix-row {
    min-height: var(--ds-row-h-touch);
  }
}

/*
 * The old override dropped to four column tracks while the row still rendered
 * five or six cells, so the cache column and the pulse track wrapped onto an
 * implicit second row. Same track count, tighter minimums.
 */
@media (max-width: 640px) {
  .matrix-row {
    grid-template-columns:
      minmax(84px, 1fr)
      minmax(44px, 0.3fr)
      minmax(48px, 0.32fr)
      minmax(44px, 0.3fr)
      minmax(84px, 2.2fr);
    gap: 0 0.35rem;
    padding: 0 0.75rem;
  }

  .matrix-row--with-tps {
    grid-template-columns:
      minmax(78px, 1fr)
      minmax(42px, 0.28fr)
      minmax(46px, 0.3fr)
      minmax(46px, 0.3fr)
      minmax(42px, 0.28fr)
      minmax(80px, 2fr);
  }
}
</style>
