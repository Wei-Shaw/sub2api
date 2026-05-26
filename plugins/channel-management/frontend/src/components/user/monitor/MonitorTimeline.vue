<template>
  <div class="mt-4 pt-3 border-t border-gray-100 dark:border-dark-700/60">
    <div
      class="flex justify-between text-[10px] font-semibold uppercase tracking-widest text-gray-400 mb-2"
    >
      <span>{{ t('monitorCommon.history60pts', { n: length }) }}</span>
      <span class="tabular-nums">{{ t('monitorCommon.nextUpdateIn', { n: countdownSeconds }) }}</span>
    </div>

    <div
      v-if="maintenance"
      class="flex h-5 w-full items-center justify-center rounded border border-dashed border-gray-300 dark:border-dark-600 text-[10px] uppercase tracking-widest text-gray-400"
    >
      {{ t('monitorCommon.maintenancePaused') }}
    </div>
    <div v-else class="flex items-end gap-[2px] h-5 w-full">
      <div
        v-for="(bar, idx) in displayBars"
        :key="idx"
        class="flex-1 min-w-[3px] rounded-sm"
        :class="bar.colorClass"
        :style="{ height: bar.heightPct + '%' }"
        :title="bar.title"
      ></div>
    </div>

    <div
      class="mt-1 flex justify-between text-[9px] uppercase tracking-widest text-gray-400"
    >
      <span>{{ t('monitorCommon.past') }}</span>
      <span>{{ t('monitorCommon.now') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '../../../api/user/channelMonitor'
import { useChannelMonitorFormat } from '../../../composables/useChannelMonitorFormat'
import {
  STATUS_OPERATIONAL,
  STATUS_DEGRADED,
  STATUS_FAILED,
  STATUS_ERROR,
} from '../../../utils/channelMonitorConstants'

// T19 Fix C — local placeholder key for "未测试/未填充" bars (灰色).
// Not a MonitorStatus value; only used as a key in STATUS_HEIGHT/STATUS_COLOR.
const STATUS_EMPTY = 'empty' as const

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

// 4 级高度 + 颜色双重编码：高=好+绿，短=坏+红，灰=未测试。
// 长绿(正常) > 中黄(降级) > 短红(失败/系统错误) > 很短灰(未测试)。
// T19 Fix C — keys 引用 channelMonitorConstants 中的 STATUS_* 常量, 避免
// 裸字面量与 MonitorStatus 类型漂移.
const STATUS_HEIGHT: Record<string, number> = {
  [STATUS_OPERATIONAL]: 100,
  [STATUS_DEGRADED]: 65,
  [STATUS_FAILED]: 35,
  [STATUS_ERROR]: 35,
  [STATUS_EMPTY]: 15,
}

const STATUS_COLOR: Record<string, string> = {
  [STATUS_OPERATIONAL]: 'bg-emerald-500',
  [STATUS_DEGRADED]: 'bg-amber-500',
  [STATUS_FAILED]: 'bg-red-500',
  [STATUS_ERROR]: 'bg-red-500',
  [STATUS_EMPTY]: 'bg-gray-300 dark:bg-dark-600',
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
      colorClass: STATUS_COLOR[STATUS_EMPTY],
      heightPct: STATUS_HEIGHT[STATUS_EMPTY],
      title: '',
    })
  }

  for (const point of real) {
    const status = point.status as keyof typeof STATUS_HEIGHT
    const colorClass = STATUS_COLOR[status] ?? STATUS_COLOR[STATUS_EMPTY]
    const heightPct = STATUS_HEIGHT[status] ?? STATUS_HEIGHT[STATUS_EMPTY]
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
