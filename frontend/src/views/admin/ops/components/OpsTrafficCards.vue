<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { OpsTrafficDirection, OpsTrafficLink, OpsTrafficSummary } from '@/api/admin/ops'
import { formatTrafficBandwidth, formatTrafficBytes, formatTrafficBytesWithBandwidth } from '@/utils/traffic'

const props = withDefaults(defineProps<{
  traffic?: OpsTrafficSummary | null
  startTime?: string | null
  endTime?: string | null
  loading?: boolean
}>(), { traffic: null, startTime: null, endTime: null, loading: false })

const { t } = useI18n()

// Use the returned overview window, not the toolbar selection or refresh cadence.
const durationSeconds = computed(() => {
  if (!props.startTime || !props.endTime) return null
  return (Date.parse(props.endTime) - Date.parse(props.startTime)) / 1000
})

// A card shows the request body on top and the response body below. The headline
// value is that direction's actual (encoded) size, its equivalent rate follows
// in the grey label, and the compression rate sits beside the value.
interface TrafficRow {
  key: 'request' | 'response' | 'combined'
  labelKey: string
  actualBytes: number | null | undefined
  originalBytes: number | null | undefined
  originalSampleCount: number | undefined
  compressionPercent: number | null | undefined
  combined: boolean
}

function placeholderRow(key: 'request' | 'response'): TrafficRow {
  return {
    key,
    labelKey: `admin.ops.traffic.${key}`,
    actualBytes: null,
    originalBytes: null,
    originalSampleCount: undefined,
    compressionPercent: null,
    combined: false
  }
}

function directionalRow(key: 'request' | 'response', direction: OpsTrafficDirection): TrafficRow {
  return {
    key,
    labelKey: `admin.ops.traffic.${key}`,
    actualBytes: direction.actual_bytes,
    originalBytes: direction.original_bytes,
    originalSampleCount: direction.original_sample_count,
    compressionPercent: direction.compression_percent,
    combined: false
  }
}

// Windows that predate the directional split keep side totals but report no
// request/response breakdown. Fall back to the side totals with their side-level
// ratio, explicitly marked as combined, instead of inventing a split.
function rowsFor(summary: OpsTrafficLink | undefined): TrafficRow[] {
  const request = summary?.request
  const response = summary?.response
  if (request && response) {
    return [directionalRow('request', request), directionalRow('response', response)]
  }
  return [{
    key: 'combined',
    labelKey: 'admin.ops.traffic.actual',
    actualBytes: summary?.actual_bytes,
    originalBytes: summary?.original_bytes,
    originalSampleCount: summary?.original_sample_count,
    compressionPercent: summary?.compression_percent,
    combined: true
  }]
}

const cards = computed(() => [
  { key: 'upstream', summary: props.traffic?.upstream, order: 8, rows: rowsFor(props.traffic?.upstream) },
  { key: 'downstream', summary: props.traffic?.downstream, order: 9, rows: rowsFor(props.traffic?.downstream) }
])

// Loading keeps the canonical two-row shape without showing stale totals.
function displayRows(card: { rows: TrafficRow[] }): TrafficRow[] {
  return props.loading ? [placeholderRow('request'), placeholderRow('response')] : card.rows
}

function validCount(value: number | null | undefined): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
}

const ingestion = computed(() => props.traffic?.ingestion?.scope === 'instance_lifetime' ? props.traffic.ingestion : null)
// These counters are not filtered traffic totals. Only show known problems;
// absent health must never produce a healthy/complete badge.
function positiveCount(value: number | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}
const ingestionFailed = computed(() => positiveCount(ingestion.value?.failed))
const ingestionPending = computed(() => positiveCount(ingestion.value?.pending))

// Traffic has its own persisted HTTP request ledger, including failures without
// usage. overview.request_count_total has a different population.
const totalRequests = computed(() => {
  const count = props.traffic?.request_count
  return validCount(count) ? count : null
})

// Side totals cover two bodies per request; directional details cover one.
const totalBodies = computed(() => {
  const count = totalRequests.value
  return count != null && Number.isSafeInteger(count * 2) ? count * 2 : null
})

function coverageText(count: number | undefined, paired = false): string {
  const total = totalBodies.value
  if (total == null || !validCount(count) || count > total) {
    return t('admin.ops.traffic.coverageUnknown')
  }
  return t(paired ? 'admin.ops.traffic.pairedCoverage' : 'admin.ops.traffic.coverage', { count, total })
}

function directionCoverageText(count: number | undefined): string {
  const total = totalRequests.value
  if (total == null || !validCount(count) || count > total) {
    return t('admin.ops.traffic.coverageUnknown')
  }
  return t('admin.ops.traffic.directionCoverage', { count, total })
}

// A combined row covers both bodies of every request; a directional row one.
function originalCoverageText(row: TrafficRow): string {
  return row.combined ? coverageText(row.originalSampleCount) : directionCoverageText(row.originalSampleCount)
}

function coverageState(summary: OpsTrafficLink | undefined): 'partial' | 'unavailable' | null {
  const original = summary?.original_sample_count
  const actual = summary?.actual_sample_count
  if ((!validCount(original) || original === 0) && (!validCount(actual) || actual === 0)) {
    return 'unavailable'
  }
  const total = totalBodies.value
  if (total == null || !validCount(original) || !validCount(actual) || original < total || actual < total) {
    return 'partial'
  }
  return null
}

// Each ratio arrives from the server, computed over that row's paired samples.
// Never derive it here from independently covered original/actual totals.
function compressionLabel(percent: number | null | undefined): string {
  if (typeof percent !== 'number' || !Number.isFinite(percent)) return '—'
  return `${Number(percent.toFixed(1)).toFixed(1)}%`
}
</script>

<template>
  <span v-if="loading" class="sr-only" role="status">{{ t('admin.ops.loadingText') }}</span>

  <article
    v-for="card in cards"
    :key="card.key"
    class="min-h-[184px] min-w-0 rounded-2xl bg-gray-50 p-4 dark:bg-dark-900 2xl:min-h-44"
    :style="{ order: card.order }"
    :data-testid="`traffic-${card.key}-card`"
    :aria-label="t(`admin.ops.traffic.${card.key}`)"
    :aria-busy="loading"
  >
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex min-w-0 items-center gap-1">
        <h2 class="text-[10px] font-bold uppercase text-gray-400">{{ t(`admin.ops.traffic.${card.key}`) }}</h2>
        <HelpTooltip
          v-if="!loading"
          width-class="w-80 max-w-[calc(100vw-2rem)]"
          :aria-label="t('admin.ops.traffic.compositionHelp')"
          :data-testid="`traffic-${card.key}-coverage-tooltip`"
        >
          <div class="space-y-2">
            <p :data-testid="`traffic-${card.key}-original-coverage`"><span class="font-semibold">{{ t('admin.ops.traffic.original') }}:</span> {{ coverageText(card.summary?.original_sample_count) }}</p>
            <p :data-testid="`traffic-${card.key}-actual-coverage`"><span class="font-semibold">{{ t('admin.ops.traffic.actual') }}:</span> {{ coverageText(card.summary?.actual_sample_count) }}</p>
            <p :data-testid="`traffic-${card.key}-paired-coverage`"><span class="font-semibold">{{ t('admin.ops.traffic.compression') }}:</span> {{ coverageText(card.summary?.paired_sample_count, true) }}</p>
            <p :data-testid="`traffic-${card.key}-compression-hint`">{{ t('admin.ops.traffic.compressionHint') }}</p>
            <p :data-testid="`traffic-${card.key}-scope`">{{ t('admin.ops.traffic.scope') }}</p>
          </div>
        </HelpTooltip>
      </div>

      <div class="flex flex-wrap items-center justify-end gap-1">
        <span
          v-if="!loading && coverageState(card.summary)"
          class="shrink-0 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-950/40 dark:text-amber-400"
          :data-testid="`traffic-${card.key}-coverage-state`"
        >{{ t(`admin.ops.traffic.${coverageState(card.summary)}`) }}</span>

        <!-- One explicitly instance-labelled badge; it is not downstream-only health. -->
        <HelpTooltip
          v-if="card.key === 'downstream' && !loading && (ingestionFailed > 0 || ingestionPending > 0)"
          width-class="w-80 max-w-[calc(100vw-2rem)]"
          class="!ml-0 max-w-full rounded-full px-2 py-0.5 text-[10px]"
          :class="ingestionFailed > 0 ? 'bg-red-50 text-red-800 dark:bg-red-950/40 dark:text-red-300' : 'bg-amber-50 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300'"
          :role="ingestionFailed > 0 ? 'alert' : 'status'"
          :aria-label="t('admin.ops.traffic.ingestionHelp')"
          data-testid="traffic-ingestion-warning"
        >
          <template #trigger>
            <span class="inline-flex max-w-full flex-wrap items-center gap-1 font-semibold">
              <svg class="h-3.5 w-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3m0 4h.01M10.29 3.86l-8.2 14.2A2 2 0 0 0 3.82 21h16.36a2 2 0 0 0 1.73-2.94l-8.2-14.2a2 2 0 0 0-3.42 0z" />
              </svg>
              <span data-testid="traffic-ingestion-instance">{{ t('admin.ops.traffic.instance') }}</span>
              <span aria-hidden="true">·</span>
              <span v-if="ingestionFailed > 0" data-testid="traffic-ingestion-failed-short">{{ t('admin.ops.traffic.ingestionFailedShort', { count: ingestionFailed }) }}</span>
              <span v-if="ingestionPending > 0" data-testid="traffic-ingestion-pending-short">{{ t('admin.ops.traffic.ingestionPendingShort', { count: ingestionPending }) }}</span>
            </span>
          </template>
          <div class="space-y-2">
            <p v-if="ingestionFailed > 0" data-testid="traffic-ingestion-failed">{{ t('admin.ops.traffic.ingestionFailed', { count: ingestionFailed }) }}</p>
            <p v-if="ingestionPending > 0" data-testid="traffic-ingestion-pending">{{ t('admin.ops.traffic.ingestionPending', { count: ingestionPending }) }}</p>
            <p data-testid="traffic-ingestion-scope">{{ t('admin.ops.traffic.ingestionScope') }}</p>
            <p data-testid="traffic-ingestion-coverage">{{ t('admin.ops.traffic.ingestionCoverage') }}</p>
          </div>
        </HelpTooltip>
      </div>
    </div>

    <dl class="mt-2 space-y-2">
      <div v-for="row in displayRows(card)" :key="row.key">
        <dt class="text-xs text-gray-500 dark:text-gray-400" :data-testid="`traffic-${card.key}-${row.key}-label`">
          <span>{{ t(row.labelKey) }}</span>
          <template v-if="!loading">
            <span aria-hidden="true">, </span>
            <HelpTooltip
              class="!ml-0 min-w-0 cursor-help rounded !align-baseline outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              tabindex="0"
              width-class="w-80 max-w-[calc(100vw-2rem)]"
              :aria-label="`${t('admin.ops.traffic.bandwidth')} ${formatTrafficBandwidth(row.actualBytes, durationSeconds)}`"
              :data-testid="`traffic-${card.key}-${row.key}-bandwidth-tooltip`"
            >
              <template #trigger>
                <span
                  class="whitespace-nowrap tabular-nums"
                  :data-testid="`traffic-${card.key}-${row.key}-bandwidth`"
                >{{ formatTrafficBandwidth(row.actualBytes, durationSeconds) }}</span>
              </template>
              <p :data-testid="`traffic-${card.key}-${row.key}-bandwidth-hint`">{{ t('admin.ops.traffic.bandwidthHint') }}</p>
            </HelpTooltip>
          </template>
        </dt>
        <dd class="mt-1" :data-testid="`traffic-${card.key}-${row.key}-row`">
          <div v-if="loading" class="h-9 w-28 animate-pulse rounded bg-gray-200 dark:bg-dark-700" aria-hidden="true"></div>
          <div v-else class="flex min-w-0 flex-wrap items-baseline gap-x-1 gap-y-1">
            <HelpTooltip
              class="!ml-0 min-w-0"
              width-class="w-72 max-w-[calc(100vw-2rem)]"
              :aria-label="t('admin.ops.traffic.originalHelp')"
              :data-testid="`traffic-${card.key}-${row.key}-original-tooltip`"
            >
              <template #trigger>
                <span
                  class="min-w-0 cursor-help text-xl font-black tabular-nums text-gray-900 [overflow-wrap:anywhere] dark:text-white xl:text-2xl 2xl:text-3xl"
                  :data-testid="`traffic-${card.key}-${row.key}`"
                >{{ formatTrafficBytes(row.actualBytes) }}</span>
              </template>
              <div class="space-y-2">
                <p>
                  <span class="font-semibold">{{ t('admin.ops.traffic.original') }}:</span>
                  <span
                    class="whitespace-nowrap tabular-nums"
                    :data-testid="`traffic-${card.key}-${row.key}-original`"
                  >{{ formatTrafficBytesWithBandwidth(row.originalBytes, durationSeconds) }}</span>
                </p>
                <p
                  class="text-gray-300"
                  :data-testid="`traffic-${card.key}-${row.key}-original-coverage`"
                >{{ originalCoverageText(row) }}</p>
              </div>
            </HelpTooltip>
            <span
              class="whitespace-nowrap text-xs font-bold text-gray-400"
              :aria-label="`${t('admin.ops.traffic.compression')} ${compressionLabel(row.compressionPercent)}`"
              :data-testid="`traffic-${card.key}-${row.key}-compression`"
            >{{ compressionLabel(row.compressionPercent) }}</span>
            <span
              v-if="row.combined"
              class="shrink-0 rounded-full bg-gray-200 px-2 py-0.5 text-[10px] font-semibold text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              :data-testid="`traffic-${card.key}-${row.key}-combined-badge`"
            >{{ t('admin.ops.traffic.noDirectionalSplit') }}</span>
          </div>
        </dd>
      </div>
    </dl>
  </article>
</template>
