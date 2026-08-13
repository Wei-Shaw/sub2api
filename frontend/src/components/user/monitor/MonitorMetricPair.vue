<template>
  <!--
    Two measurements, separated by a hairline rather than boxed in two tinted
    `rounded-xl` wells. A well inside a card inside a grid is three nested boxes
    for two numbers.
  -->
  <dl class="mt-3 grid grid-cols-2 gap-px border-y border-line-subtle bg-line-subtle">
    <div class="bg-surface py-2 pr-3">
      <dt class="flex items-center gap-1.5 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        <Icon :name="primaryIcon" size="xs" />
        <span class="truncate">{{ primaryLabel }}</span>
      </dt>
      <dd class="mt-0.5 flex justify-start">
        <NumCell :value="primaryValue" :unit="primaryUnit" :precision="0" />
      </dd>
    </div>
    <div class="bg-surface py-2 pl-3">
      <dt class="flex items-center gap-1.5 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        <Icon :name="secondaryIcon" size="xs" />
        <span class="truncate">{{ secondaryLabel }}</span>
      </dt>
      <dd class="mt-0.5 flex justify-start">
        <NumCell :value="secondaryValue" :unit="secondaryUnit" :precision="0" />
      </dd>
    </div>
  </dl>
</template>

<script setup lang="ts">
import NumCell from '@/components/common/NumCell.vue'
import Icon from '@/components/icons/Icon.vue'

/**
 * `primaryValue` / `secondaryValue` are `number | null`, not pre-formatted
 * strings. `NumCell` needs the number to group it by locale, keep the unit a
 * step down, expose the unrounded value on hover, and — the one that matters on
 * a monitor — render an en dash when there is no measurement at all, which a
 * formatted `"0"` would have hidden.
 */
defineProps<{
  primaryLabel: string
  primaryValue: number | null
  primaryUnit: string
  primaryIcon: 'bolt' | 'globe' | 'clock' | 'link'
  secondaryLabel: string
  secondaryValue: number | null
  secondaryUnit: string
  secondaryIcon: 'bolt' | 'globe' | 'clock' | 'link'
}>()
</script>
