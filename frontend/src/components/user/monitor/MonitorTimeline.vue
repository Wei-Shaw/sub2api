<template>
  <div class="mt-3 border-t border-line-subtle pt-2.5">
    <div class="mb-1.5 flex justify-between gap-2 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
      <span class="truncate">{{ t('monitorCommon.history60pts', { n: length }) }}</span>
      <span class="shrink-0 font-mono tabular-nums">{{ t('monitorCommon.nextUpdateIn', { n: countdownSeconds }) }}</span>
    </div>

    <div
      v-if="maintenance"
      class="flex h-5 w-full items-center justify-center border border-dashed border-line text-2xs uppercase tracking-[0.04em] text-ink-tertiary"
    >
      {{ t('monitorCommon.maintenancePaused') }}
    </div>
    <div v-else class="flex h-5 w-full items-end gap-px">
      <div
        v-for="(bar, idx) in displayBars"
        :key="idx"
        class="min-w-0 flex-1"
        :class="bar.colorClass"
        :style="{ height: bar.heightPct + '%' }"
        :title="bar.title"
      ></div>
    </div>

    <div class="mt-1 flex justify-between text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
      <span>{{ t('monitorCommon.past') }}</span>
      <span>{{ t('monitorCommon.now') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const props = withDefaults(defineProps<{
  buckets?: MonitorTimelinePoint[]
  countdownSeconds: number
  length?: number
  maintenance?: boolean
}>(), {
  buckets: () => [],
  length: 60,
  maintenance: false,
})

const { t } = useI18n()
const { statusLabel, formatLatency, formatRelativeTime } = useChannelMonitorFormat()

interface Bar {
  colorClass: string
  heightPct: number
  title: string
}

// Height is the primary encoding and it is redundant with the colour: tall =
// good, short = bad, shortest = never tested. Every bar also carries a `title`
// naming its status, so the strip reads without colour at all.
const STATUS_HEIGHT: Record<string, number> = {
  operational: 100,
  degraded: 65,
  failed: 35,
  error: 35,
  empty: 15,
}

/**
 * Operational is a neutral hairline grey, not green.
 *
 * On a healthy channel this strip is sixty bars of "fine". Painting all sixty
 * emerald spends the entire colour budget on the state that needs no attention,
 * and the one amber bar in the middle has nothing to stand out against. Only
 * the exceptions are tinted.
 */
const STATUS_COLOR: Record<string, string> = {
  operational: 'bg-line-strong',
  degraded: 'bg-warn',
  failed: 'bg-danger',
  error: 'bg-danger',
  empty: 'bg-line-subtle',
}

const displayBars = computed<Bar[]>(() => {
  // Real points come newest-first; convert to oldest-first so the rightmost
  // bar represents "now". Pad the left with empty placeholders to keep the
  // bar count stable at `length`.
  const real = [...(props.buckets ?? [])]
    .slice(0, props.length)
    .reverse()

  const padCount = Math.max(0, props.length - real.length)
  const bars: Bar[] = []

  for (let i = 0; i < padCount; i += 1) {
    bars.push({
      colorClass: STATUS_COLOR.empty,
      heightPct: STATUS_HEIGHT.empty,
      title: '',
    })
  }

  for (const point of real) {
    const status = point.status as keyof typeof STATUS_HEIGHT
    const colorClass = STATUS_COLOR[status] ?? STATUS_COLOR.empty
    const heightPct = STATUS_HEIGHT[status] ?? STATUS_HEIGHT.empty
    const latency = formatLatency(point.latency_ms)
    const relative = formatRelativeTime(point.checked_at)
    const label = statusLabel(point.status)
    bars.push({
      colorClass,
      heightPct,
      title: `${relative} · ${label} · ${latency}ms`,
    })
  }

  return bars
})
</script>
