<template>
  <span
    class="inline-flex items-baseline justify-end gap-1 font-mono tabular-nums"
    :class="toneClass"
  >
    <span v-if="isBlank" class="text-ink-disabled" aria-label="no value">–</span>
    <template v-else>
      <span :title="title">{{ formatted }}</span>
      <!-- The unit sits a step down so it never competes with the number. -->
      <span v-if="unit" class="text-2xs font-normal text-ink-tertiary">{{ unit }}</span>
    </template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { TONE_TEXT, type Tone } from './primitives'

/**
 * A numeric table cell.
 *
 * This is the single biggest legibility win available in this app. Only ten
 * files opted into `tabular-nums` before, which means every numeric column in
 * the product was misaligned: proportional digits give `1` a narrower advance
 * than `8`, so a column of counts wanders left and right and the eye cannot
 * scan it. Mono + tabular figures makes the column a column.
 *
 * Slashed zero (enabled alongside `tabular-nums`) matters because this app puts
 * IDs, hashes and API key fragments next to quantities — O vs 0 has to be
 * unambiguous.
 *
 * Blank renders an en dash in disabled ink rather than `0`. A missing
 * measurement and a measurement of zero are different facts, and collapsing
 * them is how a dashboard lies.
 */
const props = withDefaults(
  defineProps<{
    value: number | string | null | undefined
    /** Rendered smaller and dimmer after the number. */
    unit?: string
    /** Fixed decimal places. Omit to let Intl decide. */
    precision?: number
    /** Only for values that have crossed a declared threshold. */
    tone?: Tone
    /** Compact notation for large counts, e.g. 1.2M. */
    compact?: boolean
  }>(),
  { tone: 'neutral' }
)

const { locale } = useI18n()

const numeric = computed(() => {
  if (props.value === null || props.value === undefined || props.value === '') return null
  const n = typeof props.value === 'number' ? props.value : Number(props.value)
  return Number.isFinite(n) ? n : null
})

const isBlank = computed(() => numeric.value === null)

const formatted = computed(() => {
  if (numeric.value === null) return ''
  return new Intl.NumberFormat(locale.value, {
    minimumFractionDigits: props.precision,
    maximumFractionDigits: props.precision,
    notation: props.compact ? 'compact' : 'standard',
  }).format(numeric.value)
})

/**
 * The unrounded value stays reachable on hover. Rounding is a display choice;
 * throwing away the real number is not.
 */
const title = computed(() =>
  numeric.value !== null && String(numeric.value) !== formatted.value
    ? String(numeric.value)
    : undefined
)

const toneClass = computed(() => (props.tone === 'neutral' ? 'text-ink' : TONE_TEXT[props.tone]))
</script>
