<template>
  <div class="mt-3">
    <div class="flex items-baseline justify-between gap-3">
      <span class="truncate text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        {{ windowLabel }}
      </span>
      <!--
        The number is the primary channel; the meter is the redundant one.
        What this replaces: a 3xl figure whose colour came from
        `hsl(pct * 1.2 …)` — a continuous rainbow, so 97% and 99% were two
        different greens saying nothing, and the hue was the only signal.
        Colour appears here only below the declared thresholds.
      -->
      <NumCell :value="value" :precision="2" unit="%" :tone="tone" />
    </div>
    <!--
      Bar only, and hidden from assistive tech: the NumCell above is the
      announced channel, so announcing the same fraction twice is noise.
      `warnAt`/`dangerAt` are pinned to 1 because on availability HIGH is good —
      the primitive's default 80/95 thresholds would paint a perfect channel red.
    -->
    <div aria-hidden="true">
      <Meter class="mt-1" :value="value ?? 0" :max="100" :show-value="false" :warn-at="1" :danger-at="1" />
    </div>
    <p v-if="samplesLabel" class="mt-1 text-right text-2xs text-ink-tertiary">
      {{ samplesLabel }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import Meter from '@/components/common/Meter.vue'
import NumCell from '@/components/common/NumCell.vue'
import type { Tone } from '@/components/common/primitives'

const props = defineProps<{
  windowLabel: string
  value: number | null
  samplesLabel?: string
}>()

/** Signal budget: a healthy availability gets no colour at all. */
const tone = computed<Tone>(() => {
  if (props.value === null || Number.isNaN(props.value)) return 'neutral'
  if (props.value < 95) return 'danger'
  if (props.value < 99) return 'warn'
  return 'neutral'
})
</script>
