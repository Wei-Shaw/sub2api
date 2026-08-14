<template>
  <span class="inline-flex items-baseline gap-0.5 whitespace-nowrap" :title="titleText">
    <template v-if="rankNum != null && rankNum > 0">
      <!--
        Position is a number, so it is typeset as one: mono, tabular, aligned
        with every other figure in the column.

        What used to be here: a 20px multi-path trophy SVG driven by three
        hardcoded gold/silver/bronze palettes, plus amber/slate text tints with
        `dark:` counterparts. A medal is an ornament — it spends the most
        prominent element in the cell on decoration, cannot be scanned in a
        column, and encoded the top three in hue. Weight carries the top three
        now, which survives grayscale.
      -->
      <span class="font-mono text-2xs text-ink-tertiary" aria-hidden="true">#</span>
      <NumCell :value="rankNum" :class="isPodium ? 'font-semibold' : ''" />
      <span class="sr-only">{{ ariaLabel }}</span>
    </template>
    <template v-else>
      <span class="font-mono text-xs text-ink-disabled" :aria-label="ariaLabel">–</span>
    </template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import NumCell from '@/components/common/NumCell.vue'

const { t } = useI18n()

const props = defineProps<{
  rank: number | null | undefined
}>()

const rankNum = computed(() => {
  const n = Number(props.rank)
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : null
})

const isPodium = computed(() => rankNum.value != null && rankNum.value <= 3)

const ariaLabel = computed(() => {
  // Rank 0 = present but unranked (no traffic in window / outside top list).
  if (rankNum.value == null || rankNum.value <= 0) return t('channelMonitorV2.rank.unranked')
  if (rankNum.value === 1) return t('channelMonitorV2.rank.gold')
  if (rankNum.value === 2) return t('channelMonitorV2.rank.silver')
  if (rankNum.value === 3) return t('channelMonitorV2.rank.bronze')
  return t('channelMonitorV2.rank.place', { n: rankNum.value })
})

const titleText = computed(() => ariaLabel.value)
</script>
