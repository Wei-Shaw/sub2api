<template>
  <div class="flex flex-col gap-1">
    <div v-if="label || showValue" class="flex items-baseline justify-between gap-2">
      <span v-if="label" class="text-xs text-ink-secondary">{{ label }}</span>
      <!--
        The numeric label is the PRIMARY channel; the bar is the redundant one.
        A quota bar alone tells you roughly-how-full, which is not a number
        anyone can act on.
      -->
      <span v-if="showValue" class="font-mono text-xs tabular-nums" :class="valueClass">
        {{ valueText }}
      </span>
    </div>

    <div
      class="relative h-1 w-full overflow-hidden bg-surface-sunken"
      role="meter"
      :aria-valuenow="value"
      :aria-valuemin="0"
      :aria-valuemax="max"
      :aria-valuetext="valueText"
      :aria-label="label"
    >
      <div
        class="h-full transition-[width] duration-slow ease-out"
        :class="fillClass"
        :style="{ width: `${percent}%` }"
      />
      <!--
        Thresholds are 1px ticks, not a gradient. A gradient implies the value
        gets continuously worse; a tick says "this is the line".
      -->
      <span
        v-for="mark in marks"
        :key="mark"
        class="absolute top-0 h-full w-px bg-line-strong"
        :style="{ left: `${mark}%` }"
        aria-hidden="true"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * A quota / capacity bar.
 *
 * 4px tall, zero radius, flat fill. What it is not: a rounded pill with a
 * `bg-gradient-to-r` running through it, which is what the old `.progress-bar`
 * was. A gradient on a quota bar is actively misleading — it reads as a value
 * ramp, when the only thing that varies is width.
 *
 * The fill stays neutral ink until the value crosses `warnAt`, then `danger`
 * past `dangerAt`. Colour appears only when there is something to say. This is
 * the signal-budget rule: on a page of twenty quota bars, the two that are
 * nearly full should be the only coloured things on screen.
 */
const props = withDefaults(
  defineProps<{
    value: number
    max?: number
    label?: string
    /** Fraction of max at which the fill turns amber. */
    warnAt?: number
    /** Fraction of max at which the fill turns red. */
    dangerAt?: number
    showValue?: boolean
    /** Render as "used / total" rather than a percentage. */
    format?: 'percent' | 'ratio'
  }>(),
  { max: 100, warnAt: 0.8, dangerAt: 0.95, showValue: true, format: 'percent' }
)

const { locale } = useI18n()

const fraction = computed(() => {
  if (!props.max || !Number.isFinite(props.max)) return 0
  return Math.min(Math.max(props.value / props.max, 0), 1)
})

const percent = computed(() => fraction.value * 100)

const level = computed<'ok' | 'warn' | 'danger'>(() => {
  if (fraction.value >= props.dangerAt) return 'danger'
  if (fraction.value >= props.warnAt) return 'warn'
  return 'ok'
})

const fillClass = computed(
  () => ({ ok: 'bg-ink-secondary', warn: 'bg-warn', danger: 'bg-danger' })[level.value]
)

const valueClass = computed(
  () => ({ ok: 'text-ink', warn: 'text-warn', danger: 'text-danger' })[level.value]
)

const nf = computed(() => new Intl.NumberFormat(locale.value))

const valueText = computed(() => {
  if (props.format === 'ratio') {
    return `${nf.value.format(props.value)} / ${nf.value.format(props.max)}`
  }
  return new Intl.NumberFormat(locale.value, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(fraction.value)
})

/** Threshold positions, deduplicated and clamped inside the track. */
const marks = computed(() =>
  Array.from(
    new Set(
      [props.warnAt, props.dangerAt]
        .filter((t) => t > 0 && t < 1)
        .map((t) => Math.round(t * 1000) / 10)
    )
  )
)
</script>
