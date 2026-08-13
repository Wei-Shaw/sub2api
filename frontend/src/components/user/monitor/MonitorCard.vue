<template>
  <button
    type="button"
    class="flex w-full flex-col rounded border border-line bg-surface p-4 text-left transition-colors duration-fast ease-out hover:bg-surface-hover"
    @click="emit('click')"
  >
    <!-- Header: mark + name/model + status -->
    <div class="flex min-w-0 items-start gap-3">
      <span
        class="grid h-7 w-7 shrink-0 place-items-center rounded-sm border border-line bg-surface-sunken text-ink-secondary"
      >
        <ProviderIcon :provider="item.provider" :size="16" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="truncate text-sm font-medium text-ink">{{ item.name }}</div>
        <div class="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span class="shrink-0 text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
            {{ providerLabel(item.provider) }}
          </span>
          <span class="min-w-0 truncate font-mono text-xs text-ink-secondary">
            {{ item.primary_model }}
          </span>
          <Badge v-if="item.group_name" class="shrink-0">{{ item.group_name }}</Badge>
        </div>
      </div>
    </div>

    <!--
      Status: a dot with its word, in the one place on the card that carries
      state. What it replaces: a `rounded-full` chip filled
      `bg-emerald-100 text-emerald-700` (plus a `dark:` twin), which put a
      coloured pill on every healthy channel and, being colour-plus-fill, still
      encoded the state in hue.
    -->
    <div class="mt-3">
      <StatusDot :tone="statusTone" :label="statusLabel(item.primary_status)" />
    </div>

    <MonitorMetricPair
      primary-icon="bolt"
      :primary-label="t('monitorCommon.dialogLatency')"
      :primary-value="item.primary_latency_ms ?? null"
      primary-unit="ms"
      secondary-icon="globe"
      :secondary-label="t('monitorCommon.endpointPing')"
      :secondary-value="item.primary_ping_latency_ms ?? null"
      secondary-unit="ms"
    />

    <MonitorAvailabilityRow
      :window-label="availabilityLabel"
      :value="availabilityValue"
      :samples-label="extraModelsCountLabel"
    />

    <MonitorTimeline
      :buckets="item.timeline"
      :countdown-seconds="countdownSeconds"
    />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import Badge from '@/components/common/Badge.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import {
  STATUS_OPERATIONAL,
  STATUS_DEGRADED,
  STATUS_FAILED,
} from '@/constants/channelMonitor'
import ProviderIcon from './ProviderIcon.vue'
import MonitorMetricPair from './MonitorMetricPair.vue'
import MonitorAvailabilityRow from './MonitorAvailabilityRow.vue'
import MonitorTimeline from './MonitorTimeline.vue'

const props = defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  countdownSeconds: number
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

const { t } = useI18n()
const { statusLabel, providerLabel } = useChannelMonitorFormat()

/**
 * Status → tone, resolved here rather than by `statusBadgeClass()`.
 *
 * That helper still returns `bg-emerald-100 … dark:bg-emerald-500/15` pairs for
 * the unmigrated admin surfaces, and it also paints the operational case. This
 * card leaves operational neutral: on a status page most channels are up, and a
 * grid of green chips has no room left to shout about the one that is down.
 */
const statusTone = computed<Tone>(() => {
  if (props.item.primary_status === STATUS_DEGRADED) return 'warn'
  if (props.item.primary_status === STATUS_FAILED) return 'danger'
  if (props.item.primary_status === STATUS_OPERATIONAL) return 'neutral'
  // `error` and the empty string are both "we do not know".
  return 'neutral'
})

const availabilityLabel = computed(() => {
  const win = t(`channelStatus.windowTab.${props.window}`)
  return `${t('monitorCommon.availabilityPrefix')} · ${win}`
})

const extraModelsCountLabel = computed(() => {
  const count = props.item.extra_models?.length ?? 0
  if (count === 0) return undefined
  return t('monitorCommon.extraModelsCount', { n: count })
})
</script>
