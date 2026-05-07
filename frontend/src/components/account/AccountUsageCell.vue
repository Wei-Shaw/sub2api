<template>
  <!-- Plugin platform usage display (driven by usage_display config) -->
  <div v-if="isPluginPlatform" class="text-xs space-y-0.5">
    <template v-if="usageDisplay">
      <div
        v-if="usageDisplay.show_req_count && todayStats"
        class="flex items-center gap-1 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formatPluginReqCount }} req
        </span>
      </div>
      <div
        v-if="usageDisplay.show_cost && todayStats"
        class="flex items-center gap-1 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
          A ${{ formatPluginCost }}
        </span>
        <span
          v-if="todayStats.user_cost != null"
          class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
          :title="t('usage.userBilled')"
        >
          U ${{ formatPluginUserCost }}
        </span>
      </div>
      <div
        v-for="row in usageDisplay.extra_rows"
        :key="row.label"
        class="flex items-center gap-1 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ row.label }}: {{ resolveExtraSource(row.source) }}
        </span>
      </div>
      <div v-if="todayStatsLoading" class="flex items-center gap-1">
        <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
      <div v-if="!todayStats && !todayStatsLoading && !(usageDisplay.extra_rows?.length)" class="text-xs text-gray-400">-</div>
    </template>
    <div v-else class="text-xs text-gray-400">-</div>
  </div>

  <!-- OAuth / complex account types: dispatch to platform sub-component -->
  <div ref="rootRef" v-else-if="showUsageWindows">
    <component
      :is="usageComponent"
      :account="account"
      :usage-info="usageInfo"
      :loading="loading"
      :error="error"
      :today-stats="todayStats"
      :today-stats-loading="todayStatsLoading"
      :active-query-loading="activeQueryLoading"
      @active-query="loadActiveUsage"
    />
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div ref="rootRef" v-else>
    <AccountQuotaInfo v-if="account.platform === 'gemini'" :account="account" />
    <KeyAccountStats
      v-else
      :account="account"
      :today-stats="todayStats"
      :today-stats-loading="todayStatsLoading"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, toRef, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import AccountQuotaInfo from './AccountQuotaInfo.vue'
import KeyAccountStats from './usage/KeyAccountStats.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { useAccountUsageLoader } from '@/composables/useAccountUsageLoader'
import type { UsageDisplayConfig } from '@/api/admin/platforms'

// Platform sub-components
import AnthropicUsageSection from './usage/AnthropicUsageSection.vue'
import OpenAIUsageSection from './usage/OpenAIUsageSection.vue'
import GeminiUsageSection from './usage/GeminiUsageSection.vue'
import AntigravityUsageSection from './usage/AntigravityUsageSection.vue'

// Component registry: maps platform+type to sub-component
const USAGE_COMPONENT_REGISTRY: Record<string, Component> = {
  'anthropic:oauth': AnthropicUsageSection,
  'anthropic:setup-token': AnthropicUsageSection,
  'openai:oauth': OpenAIUsageSection,
  'antigravity:oauth': AntigravityUsageSection,
  'gemini:*': GeminiUsageSection,
}

const props = withDefaults(
  defineProps<{
    account: Account
    todayStats?: WindowStats | null
    todayStatsLoading?: boolean
    manualRefreshToken?: number
  }>(),
  {
    todayStats: null,
    todayStatsLoading: false,
    manualRefreshToken: 0
  }
)

const { t } = useI18n()
const { getPlatformDecl } = usePlatforms()
const rootRef = ref<HTMLElement | null>(null)

const { loading, activeQueryLoading, error, usageInfo, loadActiveUsage } =
  useAccountUsageLoader({
    account: toRef(props, 'account'),
    manualRefreshToken: toRef(props, 'manualRefreshToken'),
    rootRef,
  })

// ===== Plugin platform support =====
const BUILTIN_PLATFORMS = new Set(['anthropic', 'openai', 'gemini', 'antigravity'])
const isPluginPlatform = computed(() => !BUILTIN_PLATFORMS.has(props.account.platform))

const usageDisplay = computed((): UsageDisplayConfig | undefined =>
  getPlatformDecl(props.account.platform)?.usage_display
)

const formatPluginReqCount = computed(() => {
  if (!props.todayStats) return '0'
  return formatCompactNumber(props.todayStats.requests, { allowBillions: false })
})

const formatPluginCost = computed(() => {
  if (!props.todayStats) return '0.00'
  return props.todayStats.cost.toFixed(2)
})

const formatPluginUserCost = computed(() => {
  if (!props.todayStats || props.todayStats.user_cost == null) return '0.00'
  return props.todayStats.user_cost.toFixed(2)
})

function resolveExtraSource(source: string): string {
  const parts = source.split('.')
  let current: unknown = props.account
  for (const part of parts) {
    if (current == null || typeof current !== 'object') return '-'
    current = (current as Record<string, unknown>)[part]
  }
  if (current == null) return '-'
  if (typeof current === 'number') return formatCompactNumber(current)
  return String(current)
}

// ===== Dispatch =====
const showUsageWindows = computed(() => {
  if (props.account.platform === 'gemini') return true
  return props.account.type === 'oauth' || props.account.type === 'setup-token'
})

const usageComponent = computed((): Component => {
  const key = `${props.account.platform}:${props.account.type}`
  const wildcard = `${props.account.platform}:*`
  return USAGE_COMPONENT_REGISTRY[key] || USAGE_COMPONENT_REGISTRY[wildcard] || GeminiUsageSection
})
</script>
