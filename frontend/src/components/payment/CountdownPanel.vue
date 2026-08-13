<template>
  <!--
    The "expires in / waiting for payment" block, which five payment surfaces
    each hand-rolled with slightly different type sizes and a `text-2xl
    font-bold` clock.
  -->
  <section class="rounded border border-line bg-surface-sunken px-4 py-3">
    <p v-if="label" class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
      {{ label }}
    </p>
    <!--
      `role="timer"` with `aria-live="off"` is deliberate and is the whole reason
      this is a component rather than three lines inline.

      A countdown ticks once a second. Any ancestor with `aria-live="polite"`
      turns it into a screen reader reading the clock forever, which drowns out
      the one announcement that matters — the payment settling. The live region
      therefore lives on the STATUS header in the parent, and the clock
      explicitly opts out here.
    -->
    <p
      role="timer"
      aria-live="off"
      class="font-mono text-xl font-medium tabular-nums slashed-zero text-ink"
      :class="label ? 'mt-0.5' : ''"
    >
      {{ value }}
    </p>
    <p v-if="caption" class="mt-0.5 text-xs text-ink-tertiary">{{ caption }}</p>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  /** e.g. `t('payment.qr.expiresIn')`. Omit when the clock needs no heading. */
  label?: string
  /** Pre-formatted `mm:ss`. This component does not own the timer. */
  value: string
  caption?: string
}>()
</script>
