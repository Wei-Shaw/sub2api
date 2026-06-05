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
      <span
        :class="['w-[32px] shrink-0 rounded px-1 text-center text-[10px] font-medium', labelClass]"
      >
        {{ label }}
      </span>
      <div class="h-1.5 w-8 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div
          :class="['h-full transition-all duration-300', barClass]"
          :style="{ width: barWidth }"
        ></div>
      </div>
      <span :class="['w-[32px] shrink-0 text-right text-[10px] font-medium', textClass]">
        {{ displayPercent }}
      </span>
      <span v-if="shouldShowResetTime" class="shrink-0 text-[10px] text-gray-400">
        {{ formatResetTime }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatCompactNumber } from '../../utils/formatCompact'

/**
 * Minimal WindowStats shape for usage display.
 * Mirrors the host's WindowStats type — only the fields needed for rendering.
 */
export interface WindowStats {
  requests: number
  tokens: number
  cost: number
  standard_cost?: number
  user_cost?: number
}

const props = defineProps<{
  label: string
  utilization: number
  resetsAt?: string | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber'
  windowStats?: WindowStats | null
  showNowWhenIdle?: boolean
}>()

const { t } = useI18n()

// Reactive clock for countdown — vanilla setInterval replacing @vueuse/core useIntervalFn
const now = ref(new Date())
let clockTimer: ReturnType<typeof setInterval> | null = null

const startClock = () => {
  if (clockTimer) return
  clockTimer = setInterval(() => { now.value = new Date() }, 60_000)
}
const stopClock = () => {
  if (!clockTimer) return
  clearInterval(clockTimer)
  clockTimer = null
}

if (props.resetsAt) startClock()
watch(
  () => props.resetsAt,
  (val) => {
    if (val) { now.value = new Date(); startClock() }
    else { stopClock() }
  },
)
onUnmounted(stopClock)

const labelClass = computed(() => {
  const colors = {
    indigo: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
    emerald: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
    purple: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
    amber: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  }
  return colors[props.color]
})

const barClass = computed(() => {
  if (props.utilization >= 100) return 'bg-red-500'
  if (props.utilization >= 80) return 'bg-amber-500'
  return 'bg-green-500'
})

const textClass = computed(() => {
  if (props.utilization >= 100) return 'text-red-600 dark:text-red-400'
  if (props.utilization >= 80) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-600 dark:text-gray-400'
})

const barWidth = computed(() => `${Math.min(props.utilization, 100)}%`)

const displayPercent = computed(() => {
  const percent = Math.round(props.utilization)
  return percent > 999 ? '>999%' : `${percent}%`
})

const shouldShowResetTime = computed(() => {
  if (props.resetsAt) return true
  return Boolean(props.showNowWhenIdle && props.utilization <= 0)
})

const formatResetTime = computed(() => {
  if (props.showNowWhenIdle && props.utilization <= 0) return t('common.now')
  if (!props.resetsAt) return '-'

  const date = new Date(props.resetsAt)
  const diffMs = date.getTime() - now.value.getTime()
  if (diffMs <= 0) return t('common.now')

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${days}d ${diffHours % 24}h`
  }
  if (diffHours > 0) return `${diffHours}h ${diffMins}m`
  return `${diffMins}m`
})

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
