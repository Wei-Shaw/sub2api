<template>
  <div class="mt-3 flex items-end justify-between">
    <div class="text-[11px] uppercase tracking-widest text-gray-400">
      {{ windowLabel REDACTEDREDACTED
    </div>
    <div class="flex items-baseline gap-0.5">
      <span
        class="text-3xl font-bold tabular-nums leading-none"
        :style="colorStyle"
      >
        {{ displayValue REDACTEDREDACTED
      </span>
      <span
        class="text-base font-semibold leading-none"
        :style="colorStyle"
      >%</span>
    </div>
  </div>
  <div
    v-if="samplesLabel"
    class="mt-1 text-[11px] text-gray-400 text-right"
  >
    {{ samplesLabel REDACTEDREDACTED
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { hslForPct REDACTED from '@/composables/useChannelMonitorFormat'

const props = defineProps<{
  windowLabel: string
  value: number | null
  samplesLabel?: string
REDACTED>()

const { t REDACTED = useI18n()

const displayValue = computed(() => {
  if (props.value === null || Number.isNaN(props.value)) return t('monitorCommon.latencyEmpty')
  return props.value.toFixed(2)
REDACTED)

const colorStyle = computed(() => {
  const colour = hslForPct(props.value)
  return colour ? { color: colour REDACTED : { color: 'rgb(156 163 175)' REDACTED
REDACTED)
</script>
