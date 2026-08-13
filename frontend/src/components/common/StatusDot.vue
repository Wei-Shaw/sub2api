<template>
  <span class="inline-flex items-center gap-1.5 whitespace-nowrap">
    <!--
      The ONLY element in this system allowed `border-radius: full`, at 6px.
      Everything else — badges included — is 2px. That exclusivity is what makes
      a round thing read as "state" at a glance.
    -->
    <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="TONE_FILL[tone]" aria-hidden="true" />
    <span :class="labelClass">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import { TONE_FILL, TONE_TEXT, type Tone } from './primitives'

/**
 * A colored dot with a mandatory text label.
 *
 * `label` is REQUIRED, and that is the whole design of this component. A bare
 * colored dot encodes state in hue alone, which fails for readers with colour
 * vision deficiency, in a grayscale printout, and in a screenshot pasted into
 * a ticket. Making the prop required means the redundant channel cannot be
 * forgotten — there is no way to render this component wrong.
 *
 * Use for ROW-level state, in a dedicated narrow leading column. Do not tint
 * the row itself: a table where every row carries a background colour has no
 * signal left for the rows that matter.
 */
const props = withDefaults(
  defineProps<{
    tone?: Tone
    label: string
    /** Dim the label when the state is unremarkable, e.g. a long list of "ok". */
    muted?: boolean
  }>(),
  { tone: 'neutral' }
)

const labelClass = computed(() => [
  'text-xs',
  props.muted ? 'text-ink-tertiary' : TONE_TEXT[props.tone],
])
</script>
