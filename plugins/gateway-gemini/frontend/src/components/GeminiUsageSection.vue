<template>
  <div>
    <!-- Auth Type + Tier Badge (first line) -->
    <div v-if="authTypeLabel" class="mb-1 flex items-center gap-1">
      <span
        :class="[
          'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
          tierClass
        ]"
      >
        {{ authTypeLabel }}
      </span>
      <!-- Help icon -->
      <span class="group relative cursor-help">
        <svg
          class="h-3.5 w-3.5 text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
          fill="currentColor"
          viewBox="0 0 20 20"
        >
          <path
            fill-rule="evenodd"
            d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 00-.867.5 1 1 0 11-1.731-1A3 3 0 0113 8a3.001 3.001 0 01-2 2.83V11a1 1 0 11-2 0v-1a1 1 0 011-1 1 1 0 100-2zm0 8a1 1 0 100-2 1 1 0 000 2z"
            clip-rule="evenodd"
          />
        </svg>
        <span
          class="pointer-events-none absolute left-0 top-full z-50 mt-1 w-80 whitespace-normal break-words rounded bg-gray-900 px-3 py-2 text-xs leading-relaxed text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-gray-700"
        >
          <div class="font-semibold mb-1">{{ t('admin.accounts.gemini.quotaPolicy.title') }}</div>
          <div class="mb-2 text-gray-300">{{ t('admin.accounts.gemini.quotaPolicy.note') }}</div>
          <div class="space-y-1">
            <div><strong>{{ quotaPolicyChannel }}:</strong></div>
            <div class="pl-2">{{ quotaPolicyLimits }}</div>
            <div class="mt-2">
              <a :href="quotaPolicyDocsUrl" target="_blank" rel="noopener noreferrer" class="text-semantic-info underline">
                {{ t('admin.accounts.gemini.quotaPolicy.columns.docs') }} &rarr;
              </a>
            </div>
          </div>
        </span>
      </span>
    </div>

    <!-- Usage data or unlimited flow -->
    <div class="space-y-1">
      <div
        v-if="showTodayStats && todayStats"
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
            A ${{ formatCost }}
          </span>
          <span
            v-if="todayStats.user_cost != null"
            class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
            :title="t('usage.userBilled')"
          >
            U ${{ formatUserCost }}
          </span>
        </div>
      </div>
      <div
        v-else-if="showTodayStats && todayStatsLoading"
        class="mb-0.5 flex items-center gap-1"
      >
        <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
      <div v-if="loading" class="space-y-1">
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
      </div>
      <div v-else-if="error" class="text-semantic-danger text-xs">
        {{ error }}
      </div>
      <!-- Gemini: show daily usage bars when available -->
      <div v-else-if="usageAvailable" class="space-y-1">
        <UsageProgressBar
          v-for="bar in usageBars"
          :key="bar.key"
          :label="bar.label"
          :utilization="bar.utilization"
          :resets-at="bar.resetsAt"
          :window-stats="bar.windowStats"
          :color="bar.color"
        />
        <p class="mt-1 text-[9px] leading-tight text-gray-400 dark:text-gray-500 italic">
          * {{ t('admin.accounts.gemini.quotaPolicy.simulatedNote') || 'Simulated quota' }}
        </p>
      </div>
      <!-- AI Studio Client OAuth: show unlimited flow -->
      <div v-else class="text-xs text-gray-400">
        {{ t('admin.accounts.gemini.rateLimit.unlimited') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { UsageProgressBar, formatCompactNumber } from '@sub2api/plugin-sdk'
import { useGeminiTier } from '../composables/useGeminiTier'

/** Minimal WindowStats shape. */
interface WindowStats {
  requests: number
  tokens: number
  cost: number
  standard_cost?: number
  user_cost?: number
}

/** Minimal UsageProgress shape. */
interface UsageProgress {
  utilization: number
  resets_at: string | null
  window_stats?: WindowStats | null
}

/** Minimal account usage info for Gemini display. */
interface GeminiUsageInfo {
  gemini_shared_daily?: UsageProgress | null
  gemini_pro_daily?: UsageProgress | null
  gemini_flash_daily?: UsageProgress | null
  gemini_shared_minute?: UsageProgress | null
  gemini_pro_minute?: UsageProgress | null
  gemini_flash_minute?: UsageProgress | null
}

const props = defineProps<{
  account: { type: string; credentials?: Record<string, unknown>; [k: string]: unknown }
  usageInfo: GeminiUsageInfo | null
  loading: boolean
  error: string | null
  todayStats?: WindowStats | null
  todayStatsLoading?: boolean
}>()

const { t } = useI18n()

const {
  oauthType, isCodeAssist, authTypeLabel, tierClass,
  quotaPolicyChannel, quotaPolicyLimits, quotaPolicyDocsUrl,
} = useGeminiTier(toRef(props, 'account'))

// ===== Today stats (service_account only) =====

const showTodayStats = computed(() => props.account.type === 'service_account')

const formatRequests = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.requests, { allowBillions: false })
})
const formatTokens = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.tokens)
})
const formatCost = computed(() => {
  if (!props.todayStats) return '0.00'
  return props.todayStats.cost.toFixed(2)
})
const formatUserCost = computed(() => {
  if (!props.todayStats || props.todayStats.user_cost == null) return '0.00'
  return props.todayStats.user_cost.toFixed(2)
})

// ===== Usage bars =====

const usageAvailable = computed(() => {
  return (
    !!props.usageInfo?.gemini_shared_daily ||
    !!props.usageInfo?.gemini_pro_daily ||
    !!props.usageInfo?.gemini_flash_daily ||
    !!props.usageInfo?.gemini_shared_minute ||
    !!props.usageInfo?.gemini_pro_minute ||
    !!props.usageInfo?.gemini_flash_minute
  )
})

const usesSharedDaily = computed(() => {
  return (
    !!props.usageInfo?.gemini_shared_daily ||
    !!props.usageInfo?.gemini_shared_minute ||
    oauthType.value === 'google_one' ||
    isCodeAssist.value
  )
})

const usageBars = computed(() => {
  if (!props.usageInfo) return []

  const bars: Array<{
    key: string
    label: string
    utilization: number
    resetsAt: string | null
    windowStats?: WindowStats | null
    color: 'indigo' | 'emerald'
  }> = []

  if (usesSharedDaily.value) {
    const sd = props.usageInfo.gemini_shared_daily
    if (sd) {
      bars.push({
        key: 'shared_daily',
        label: '1d',
        utilization: sd.utilization,
        resetsAt: sd.resets_at,
        windowStats: sd.window_stats,
        color: 'indigo',
      })
    }
    return bars
  }

  const pro = props.usageInfo.gemini_pro_daily
  if (pro) {
    bars.push({
      key: 'pro_daily',
      label: 'pro',
      utilization: pro.utilization,
      resetsAt: pro.resets_at,
      windowStats: pro.window_stats,
      color: 'indigo',
    })
  }

  const flash = props.usageInfo.gemini_flash_daily
  if (flash) {
    bars.push({
      key: 'flash_daily',
      label: 'flash',
      utilization: flash.utilization,
      resetsAt: flash.resets_at,
      windowStats: flash.window_stats,
      color: 'emerald',
    })
  }

  return bars
})
</script>
