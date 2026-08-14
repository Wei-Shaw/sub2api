<template>
  <div class="flex min-w-0 flex-col gap-1 px-4 py-3" :title="title || undefined">
    <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        {{ label }}
      </span>
      <!--
        State carries a dot AND a word. A bare coloured dot encodes the state in
        hue alone, which is unreadable for a colour-vision-deficient reader, in
        a grayscale printout, and in a screenshot pasted into a ticket —
        `StatusDot` makes the label impossible to forget by requiring it. The
        healthy state resolves to `neutral`: a KPI strip where every tile is
        green has spent its whole colour budget before anything is wrong.
      -->
      <StatusDot v-if="state && stateLabel" :tone="tone" :label="stateLabel" muted />
    </div>

    <strong class="truncate font-mono text-2xl font-semibold tabular-nums" :class="valueClass">
      {{ value }}
    </strong>

    <div
      v-if="detailParts.length > 1"
      class="flex flex-wrap gap-x-2 gap-y-0.5 text-2xs text-ink-tertiary"
    >
      <span
        v-for="(part, index) in detailParts"
        :key="`${index}:${part}`"
        class="whitespace-nowrap font-mono tabular-nums"
      >{{ part }}</span>
    </div>
    <small v-else-if="detail" class="block text-2xs text-ink-tertiary">{{ detail }}</small>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import StatusDot from '@/components/common/StatusDot.vue'
import type { Tone } from '@/components/common/primitives'
import type { HealthState } from '@/api/channelMonitorV2'

/**
 * One headline number in the KPI strip.
 *
 * Deliberately not a card: no border, no shadow, no ground of its own. A row of
 * these lives inside ONE bordered panel, separated by hairlines — boxing each
 * number individually is the nested-card look this rewrite removes.
 *
 * `value` arrives pre-formatted because the monitor formatters own units that
 * `Intl.NumberFormat` cannot express on its own (`1.2s` vs `840ms`, `%`, `K/M`
 * compaction). It is still rendered in mono tabular figures so a row of tiles
 * aligns.
 */
const props = defineProps<{
  label: string
  value: string
  detail: string
  state?: HealthState
  /** Required alongside `state` — the text half of the status channel. */
  stateLabel?: string
  /** Exact numeric tooltip (e.g. uncompacted RPM/TPM). */
  title?: string
}>()

/** Split "AVG 475ms · P90 800ms" into chips so nothing is ellipsized. */
const detailParts = computed(() => {
  const raw = (props.detail || '').trim()
  if (!raw || raw === '-') return []
  return raw
    .split(/\s*[·|]\s*/)
    .map((part) => part.trim())
    .filter(Boolean)
})

const tone = computed<Tone>(() => {
  if (props.state === 'warning') return 'warn'
  if (props.state === 'critical') return 'danger'
  // `healthy` and `unknown` are both unremarkable — neither earns colour.
  return 'neutral'
})

/**
 * Only a crossed threshold tints the number itself. Healthy stays ink, so on a
 * five-tile strip the one that is failing is the only coloured thing on screen.
 */
const valueClass = computed(() => {
  if (props.state === 'warning') return 'text-warn'
  if (props.state === 'critical') return 'text-danger'
  return 'text-ink'
})
</script>
