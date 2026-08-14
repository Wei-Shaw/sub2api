<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import { TONE_SOLID, TONE_TINT, type Tone } from './primitives'

/**
 * Squared, bordered, tinted.
 *
 * The pill shape is gone. `rounded-full` on a text badge was one of the most
 * repeated slop signals in the old tree (559 sites of `rounded-full`, most of
 * them badges), and a pill reads as a tag you can dismiss rather than a label
 * describing the row.
 *
 * The 1px border is load-bearing, not decoration: it is the non-colour channel
 * that keeps the badge legible in grayscale and for colour-blind readers. See
 * `TONE_TINT` in primitives.ts.
 */
const props = withDefaults(
  defineProps<{
    tone?: Tone
    /** `solid` for the rare case a badge must dominate, e.g. a hard failure. */
    variant?: 'tint' | 'solid'
    /** Uppercase + tracking. For short category labels, not for sentences. */
    caps?: boolean
    mono?: boolean
  }>(),
  { tone: 'neutral', variant: 'tint' }
)

const classes = computed(() => [
  'inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-2xs font-medium',
  props.variant === 'solid' ? TONE_SOLID[props.tone] : TONE_TINT[props.tone],
  props.caps ? 'uppercase tracking-[0.04em]' : '',
  props.mono ? 'font-mono tabular-nums' : '',
])
</script>
