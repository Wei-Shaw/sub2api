<template>
  <div>
    <!-- Window stats row (above progress bar) -->
    <div
      v-if="windowStats && (windowStats.requests > 0 || windowStats.tokens > 0)"
      class="mb-0.5 flex items-center"
    >
      <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatRequests }} req
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatTokens }}
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
          A ${{ formatAccountCost }}
        </span>
        <span
          v-if="windowStats?.user_cost != null"
          class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
          :title="t('usage.userBilled')"
        >
          U ${{ formatUserCost }}
        </span>
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
          :title="barTitle"
        ></div>
      </div>

      <!-- Percentage -->
      <span :class="['w-[36px] shrink-0 text-right text-[10px] font-medium', textClass]" :title="barTitle">
        {{ displayPercent }}
      </span>

      <!-- Reset time -->
      <span v-if="shouldShowResetTime" class="shrink-0 text-[10px] text-gray-400">
        {{ formatResetTime }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'

const props = defineProps<{
  label: string
  utilization: number // Used percentage (0-100+)
  resetsAt?: string | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber'
  windowStats?: WindowStats | null
  showNowWhenIdle?: boolean
  displayMode?: 'used' | 'remaining' | 'remaining-from-used'
}>()

const { t } = useI18n()

// Reactive clock for countdown — only runs when a reset time is shown,
// to avoid creating many idle timers across large account lists.
const now = ref(new Date())
const { pause: pauseClock, resume: resumeClock } = useIntervalFn(
  () => {
    now.value = new Date()
  },
  60_000,
  { immediate: false },
)
if (props.resetsAt) resumeClock()
watch(
  () => props.resetsAt,
  (val) => {
    if (val) {
      now.value = new Date()
      resumeClock()
    } else {
      pauseClock()
    }
  },
)

const usagePercent = computed(() => Math.max(0, props.utilization || 0))
const remainingPercent = computed(() => Math.max(0, 100 - Math.min(usagePercent.value, 100)))
const explicitRemainingPercent = computed(() => Math.max(0, Math.min(usagePercent.value, 100)))
const shownRemainingPercent = computed(() => props.displayMode === 'remaining' ? explicitRemainingPercent.value : remainingPercent.value)
const showsRemaining = computed(() => props.displayMode === 'remaining' || props.displayMode === 'remaining-from-used')
const displayValue = computed(() => showsRemaining.value ? shownRemainingPercent.value : usagePercent.value)

// Label background colors
const labelClass = computed(() => {
  const colors = {
    indigo: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
    emerald: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
    purple: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
    amber: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
  }
  return colors[props.color]
})

const remainingHealthClass = computed(() => {
  if (shownRemainingPercent.value <= 10) return 'critical'
  if (shownRemainingPercent.value <= 25) return 'warning'
  return 'healthy'
})

// Progress bar color. Remaining mode uses health semantics: green when plenty remains, red when low.
const barClass = computed(() => {
  if (showsRemaining.value) {
    if (remainingHealthClass.value === 'critical') return 'bg-red-500'
    if (remainingHealthClass.value === 'warning') return 'bg-amber-500'
    return 'bg-green-500'
  }
  if (usagePercent.value >= 100) return 'bg-red-500'
  if (usagePercent.value >= 80) return 'bg-amber-500'
  return 'bg-green-500'
})

// Text color follows the same semantics as the bar.
const textClass = computed(() => {
  if (showsRemaining.value) {
    if (remainingHealthClass.value === 'critical') return 'text-red-600 dark:text-red-400'
    if (remainingHealthClass.value === 'warning') return 'text-amber-600 dark:text-amber-400'
    return 'text-green-600 dark:text-green-400'
  }
  if (usagePercent.value >= 100) return 'text-red-600 dark:text-red-400'
  if (usagePercent.value >= 80) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-600 dark:text-gray-400'
})

// Bar width (capped at 100%)
const barWidth = computed(() => `${Math.min(displayValue.value, 100)}%`)

// Display percentage (cap at 999% for readability)
const displayPercent = computed(() => {
  const percent = Math.round(displayValue.value)
  const value = percent > 999 ? '>999%' : `${percent}%`
  return showsRemaining.value ? `余${value}` : value
})

const barTitle = computed(() => {
  if (showsRemaining.value) {
    const remaining = Math.round(shownRemainingPercent.value)
    const used = props.displayMode === 'remaining' ? Math.max(0, 100 - remaining) : Math.round(usagePercent.value)
    return `剩余 ${remaining}%，已用 ${used}%`
  }
  return `已用 ${Math.round(usagePercent.value)}%`
})

const shouldShowResetTime = computed(() => {
  if (props.resetsAt) return true
  return Boolean(props.showNowWhenIdle && usagePercent.value <= 0)
})

// Format reset time
const formatResetTime = computed(() => {
  // For rolling windows, when utilization is 0%, treat as immediately available.
  if (props.showNowWhenIdle && usagePercent.value <= 0) {
    return '现在'
  }

  if (!props.resetsAt) return '-'

  const date = new Date(props.resetsAt)
  const diffMs = date.getTime() - now.value.getTime()

  if (diffMs <= 0) return '现在'

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

// Window stats formatters
const formatRequests = computed(() => {
  if (!props.windowStats) return ''
  return formatCompactNumber(props.windowStats.requests, { allowBillions: false })
})

const formatTokens = computed(() => {
  if (!props.windowStats) return ''
  return formatCompactNumber(props.windowStats.tokens)
})

const formatAccountCost = computed(() => {
  if (!props.windowStats) return '0.00'
  return props.windowStats.cost.toFixed(2)
})

const formatUserCost = computed(() => {
  if (!props.windowStats || props.windowStats.user_cost == null) return '0.00'
  return props.windowStats.user_cost.toFixed(2)
})

</script>
