<template>
  <div
    class="stat-card !min-h-[6.5rem] !rounded-3xl !border-0 !p-4 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700"
    :title="title || undefined"
  >
    <div
      v-if="resolvedState"
      class="mt-1 h-2 w-2 shrink-0 rounded-full"
      :class="dotClass"
      aria-hidden="true"
    ></div>
    <div class="min-w-0 flex-1">
      <span class="stat-label text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ label }}</span>
      <strong
        class="stat-value mt-1 block overflow-visible text-xl tabular-nums leading-tight !text-clip !whitespace-normal"
        :class="stateClass"
      >{{ value }}</strong>
      <div
        v-if="detailParts.length > 1"
        class="mt-1.5 flex flex-wrap gap-x-2 gap-y-0.5 text-[11px] leading-snug text-gray-400 dark:text-dark-400"
      >
        <span
          v-for="(part, index) in detailParts"
          :key="`${index}:${part}`"
          class="whitespace-nowrap tabular-nums"
        >{{ part }}</span>
      </div>
      <small
        v-else-if="detail"
        class="mt-1.5 block text-[11px] leading-snug text-gray-400 dark:text-dark-400"
      >{{ detail }}</small>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { HealthState } from '@/api/channelMonitorV2'

const props = defineProps<{
  label: string
  value: string
  detail: string
  state?: HealthState
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

const missingValue = computed(() => {
  const value = (props.value || '').trim()
  return value === '' || value === '-' || value === '—'
})

const resolvedState = computed(() => (missingValue.value ? undefined : props.state))

const stateClass = computed(() => {
  if (!resolvedState.value) return missingValue.value ? 'text-gray-500 dark:text-dark-400' : 'text-gray-900 dark:text-white'
  if (resolvedState.value === 'healthy') return 'text-emerald-600 dark:text-emerald-400'
  if (resolvedState.value === 'warning') return 'text-amber-600 dark:text-amber-400'
  if (resolvedState.value === 'critical') return 'text-red-600 dark:text-red-400'
  return 'text-gray-500 dark:text-dark-400'
})

const dotClass = computed(() => {
  if (resolvedState.value === 'healthy') return 'bg-emerald-500'
  if (resolvedState.value === 'warning') return 'bg-amber-500'
  if (resolvedState.value === 'critical') return 'bg-red-500'
  return 'bg-gray-300 dark:bg-dark-600'
})
</script>
