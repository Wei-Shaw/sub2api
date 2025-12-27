<template>
  <div>
    <!-- Window stats row (above progress bar, left-right aligned with progress bar) -->
    <div
      v-if="windowStats"
      class="mb-0.5 flex items-center justify-between"
      :title="t('admin.accounts.usageWindow.statsTitle')"
    >
      <div
        class="flex cursor-help items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatRequests }} req
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatTokens }}
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"> ${{ formatCost }} </span>
      </div>
    </div>

    <!-- Progress bar row -->
    <div class="flex items-center gap-1">
      <!-- Label badge (fixed width for alignment) -->
      <span
        :class="['w-[32px] shrink-0 rounded px-1 text-center text-[10px] font-medium', labelClass]"
      >
        {{ label }}
      </span>

      <!-- Progress bar container -->
      <div class="h-1.5 w-8 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div
          :class="['h-full transition-all duration-300', barClass]"
          :style="{ width: barWidth }"
        ></div>
      </div>

      <!-- Percentage -->
      <span :class="['w-[32px] shrink-0 text-right text-[10px] font-medium', textClass]">
        {{ displayPercent }}
      </span>

      <!-- Reset time -->
      <span v-if="resetsAt" class="shrink-0 text-[10px] text-gray-400">
        {{ formatResetTime }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'

const props = defineProps<{
  label: string
  utilization: number // Percentage (0-100+)
  resetsAt?: string | null
  color: 'indigo' | 'emerald' | 'purple'
  windowStats?: WindowStats | null
}>()

const { t } = useI18n()

// Label background colors
const labelClass = computed(() => {
  const colors = {
    indigo: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
    emerald: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
    purple: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
  }
  return colors[props.color]
})

// Progress bar color based on utilization
const barClass = computed(() => {
  if (props.utilization >= 100) {
    return 'bg-red-500'
  } else if (props.utilization >= 80) {
    return 'bg-amber-500'
  } else {
    return 'bg-green-500'
  }
})

// Text color based on utilization
const textClass = computed(() => {
  if (props.utilization >= 100) {
    return 'text-red-600 dark:text-red-400'
  } else if (props.utilization >= 80) {
    return 'text-amber-600 dark:text-amber-400'
  } else {
    return 'text-gray-600 dark:text-gray-400'
  }
})

// Bar width (capped at 100%)
const barWidth = computed(() => {
  return `${Math.min(props.utilization, 100)}%`
})

// Display percentage (cap at 999% for readability)
const displayPercent = computed(() => {
  const percent = Math.round(props.utilization)
  return percent > 999 ? '>999%' : `${percent}%`
})

// Format reset time
const formatResetTime = computed(() => {
  if (!props.resetsAt) return 'N/A'
  const date = new Date(props.resetsAt)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()

  if (diffMs <= 0) return 'Now'

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${days}d ${diffHours % 24}h`
  } else if (diffHours > 0) {
    return `${diffHours}h ${diffMins}m`
  } else {
    return `${diffMins}m`
  }
})

// Format window stats
const formatRequests = computed(() => {
  if (!props.windowStats) return ''
  const r = props.windowStats.requests
  if (r >= 1000000) return `${(r / 1000000).toFixed(1)}M`
  if (r >= 1000) return `${(r / 1000).toFixed(1)}K`
  return r.toString()
})

const formatTokens = computed(() => {
  if (!props.windowStats) return ''
  const t = props.windowStats.tokens
  if (t >= 1000000000) return `${(t / 1000000000).toFixed(1)}B`
  if (t >= 1000000) return `${(t / 1000000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
})

const formatCost = computed(() => {
  if (!props.windowStats) return '0.00'
  return props.windowStats.cost.toFixed(2)
})
</script>
