<template>
  <div class="flex flex-col gap-1">
    <div class="flex items-center gap-1.5">
      <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        {{ label }}
      </span>
      <slot name="label-adornment" />
    </div>

    <div class="flex items-baseline gap-1.5">
      <span class="truncate font-mono text-2xl font-semibold tabular-nums text-ink">
        <slot>{{ displayValue }}</slot>
      </span>
      <span v-if="unit" class="text-xs text-ink-tertiary">{{ unit }}</span>
    </div>

    <!--
      Delta carries a glyph as well as a colour, for the same reason StatusDot
      requires a label: green and red alone do not survive grayscale or
      deuteranopia.
    -->
    <div v-if="delta !== undefined && delta !== null" class="flex items-center gap-1 text-xs">
      <span :class="deltaClass">{{ deltaGlyph }} {{ deltaText }}</span>
      <span v-if="deltaCaption" class="text-ink-tertiary">{{ deltaCaption }}</span>
    </div>
    <div v-else-if="caption" class="text-xs text-ink-tertiary">{{ caption }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * A single headline number.
 *
 * This replaces the icon-led stat card. The old one led with a 48px pastel
 * rounded-square holding an icon, which is the most recognisable AI-dashboard
 * cliché there is — and it spent the most visually prominent element in the
 * card on decoration rather than on the number.
 *
 * Type-led instead: a small uppercase label, then the value in mono tabular
 * figures at the largest size on the card, then the delta. Nothing else. In a
 * system with no colour decoration, the type scale IS the hierarchy.
 *
 * Deliberately not a card: no border, no padding, no background. Metrics are
 * usually laid out in a row inside one panel, and giving each its own box
 * produces the nested-card look this rewrite is removing. Wrap a row of these
 * in a single bordered panel instead.
 */
const props = withDefaults(
  defineProps<{
    label: string
    value?: number | string | null
    unit?: string
    /** Fractional change, e.g. 0.124 for +12.4%. */
    delta?: number | null
    /** e.g. "vs last week". Shown next to the delta. */
    deltaCaption?: string
    /** For a metric where a rise is bad — error rate, latency, cost. */
    invertDelta?: boolean
    /** Shown when there is no delta. */
    caption?: string
    precision?: number
  }>(),
  { precision: 0 }
)

const { locale } = useI18n()

const displayValue = computed(() => {
  if (props.value === null || props.value === undefined || props.value === '') return '–'
  const n = typeof props.value === 'number' ? props.value : Number(props.value)
  if (!Number.isFinite(n)) return String(props.value)
  return new Intl.NumberFormat(locale.value, {
    minimumFractionDigits: props.precision,
    maximumFractionDigits: props.precision,
  }).format(n)
})

const deltaGlyph = computed(() => {
  if (!props.delta) return '='
  return props.delta > 0 ? '▲' : '▼'
})

const deltaText = computed(() => {
  if (props.delta === undefined || props.delta === null) return ''
  return new Intl.NumberFormat(locale.value, {
    style: 'percent',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
    signDisplay: 'never',
  }).format(Math.abs(props.delta))
})

const deltaClass = computed(() => {
  if (!props.delta) return 'text-ink-tertiary'
  const good = props.invertDelta ? props.delta < 0 : props.delta > 0
  return good ? 'text-success' : 'text-danger'
})
</script>
