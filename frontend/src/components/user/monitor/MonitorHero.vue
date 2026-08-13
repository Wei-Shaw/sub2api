<template>
  <section class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-line pb-3">
    <!--
      The page's one status readout: a dot with a word next to it. What it
      replaces: an uppercase pill in `bg-emerald-100 text-emerald-700` whose dot
      carried `animate-pulse` — a permanent animation on a permanent state, and
      a green that filled the whole chip whether or not anything was wrong.
    -->
    <StatusDot :tone="overallTone" :label="overallLabel" />

    <div class="flex flex-wrap items-center justify-end gap-2">
      <div
        class="inline-flex -space-x-px"
        role="group"
        :aria-label="t('monitorCommon.availabilityPrefix')"
      >
        <button
          v-for="opt in windowOptions"
          :key="opt.value"
          type="button"
          :aria-pressed="window === opt.value"
          :class="[SEGMENT, window === opt.value ? SEGMENT_ON : SEGMENT_OFF]"
          @click="emit('update:window', opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>

      <Button
        size="sm"
        class="shrink-0"
        :disabled="loading"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="emit('refresh')"
      >
        <template #icon>
          <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
        </template>
      </Button>

      <AutoRefreshButton
        v-if="autoRefresh"
        :enabled="autoRefresh.enabled.value"
        :interval-seconds="autoRefresh.intervalSeconds.value"
        :countdown="autoRefresh.countdown.value"
        :intervals="autoRefresh.intervals"
        @update:enabled="autoRefresh.setEnabled"
        @update:interval="autoRefresh.setInterval"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import Button from '@/components/common/Button.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'

export type MonitorWindow = '7d' | '15d' | '30d'
export type OverallStatus = 'operational' | 'degraded'

/** Hairlines collapse via `-space-x-px` so the group reads as one object. */
const SEGMENT =
  'h-7 shrink-0 border px-2.5 text-xs font-medium transition-colors duration-fast first:rounded-l last:rounded-r'
const SEGMENT_ON = 'relative z-10 border-accent-solid bg-accent-solid text-accent-on'
const SEGMENT_OFF = 'border-line bg-surface text-ink-secondary hover:bg-surface-hover hover:text-ink'

const props = defineProps<{
  overallStatus: OverallStatus
  intervalSeconds: number
  window: MonitorWindow
  loading: boolean
  autoRefresh?: {
    enabled: { value: boolean }
    intervalSeconds: { value: number }
    countdown: { value: number }
    intervals: readonly number[]
    setEnabled: (v: boolean) => void
    setInterval: (v: number) => void
  }
}>()

const emit = defineEmits<{
  (e: 'update:window', value: MonitorWindow): void
  (e: 'refresh'): void
}>()

const { t } = useI18n()

const windowOptions = computed<{ value: MonitorWindow; label: string }[]>(() => [
  { value: '7d', label: t('channelStatus.windowTab.7d') },
  { value: '15d', label: t('channelStatus.windowTab.15d') },
  { value: '30d', label: t('channelStatus.windowTab.30d') },
])

const overallLabel = computed(() => t(`channelStatus.overall.${props.overallStatus}`))

/**
 * "Everything is fine" earns no colour. Only the degraded state is tinted, so
 * on a page of healthy channels the one signal on screen means something.
 */
const overallTone = computed<Tone>(() =>
  props.overallStatus === 'degraded' ? 'warn' : 'neutral'
)
</script>
