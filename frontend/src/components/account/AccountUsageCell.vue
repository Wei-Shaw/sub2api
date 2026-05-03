<template>
  <div ref="rootRef" v-if="showUsageWindows">
    <!-- Anthropic OAuth and Setup Token accounts: fetch real usage data -->
    <template
      v-if="
        account.platform === 'anthropic' &&
        (account.type === 'oauth' || account.type === 'setup-token')
      "
    >
      <!-- Loading state -->
      <div v-if="loading" class="space-y-1.5">
        <!-- OAuth: 3 rows, Setup Token: 1 row -->
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
        <template v-if="account.type === 'oauth'">
          <div class="flex items-center gap-1">
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          </div>
          <div class="flex items-center gap-1">
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
            <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          </div>
        </template>
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="text-xs text-red-500">
        {{ error }}
      </div>

      <!-- Usage data -->
      <div v-else-if="usageInfo" class="space-y-1">
        <!-- API error (degraded response) -->
        <div v-if="usageInfo.error" class="text-xs text-amber-600 dark:text-amber-400 truncate max-w-[200px]" :title="usageInfo.error">
          {{ usageInfo.error }}
        </div>
        <!-- 5h Window -->
        <UsageProgressBar
          v-if="usageInfo.five_hour"
          label="5h"
          :utilization="usageInfo.five_hour.utilization"
          :resets-at="usageInfo.five_hour.resets_at"
          :window-stats="usageInfo.five_hour.window_stats"
          color="indigo"
        />

        <!-- 7d Window (OAuth only) -->
        <UsageProgressBar
          v-if="usageInfo.seven_day"
          label="7d"
          :utilization="usageInfo.seven_day.utilization"
          :resets-at="usageInfo.seven_day.resets_at"
          color="emerald"
        />

        <!-- 7d Sonnet Window (OAuth only) -->
        <UsageProgressBar
          v-if="usageInfo.seven_day_sonnet"
          label="7d S"
          :utilization="usageInfo.seven_day_sonnet.utilization"
          :resets-at="usageInfo.seven_day_sonnet.resets_at"
          color="purple"
        />

        <!-- Passive sampling label + active query button -->
        <div class="flex items-center gap-1.5 mt-0.5">
          <span
            v-if="usageInfo.source === 'passive'"
            class="text-[9px] text-gray-400 dark:text-gray-500 italic"
          >
            {{ t('admin.accounts.usageWindow.passiveSampled') }}
          </span>
          <button
            type="button"
            class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[9px] font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30 transition-colors"
            :disabled="activeQueryLoading"
            @click="loadActiveUsage"
          >
            <svg
              class="h-2.5 w-2.5"
              :class="{ 'animate-spin': activeQueryLoading }"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
              />
            </svg>
            {{ t('admin.accounts.usageWindow.activeQuery') }}
          </button>
        </div>
      </div>

      <!-- No data yet -->
      <div v-else class="text-xs text-gray-400">-</div>
    </template>

    <!-- OpenAI OAuth accounts: single source from /usage API -->
    <template v-else-if="account.platform === 'openai' && account.type === 'oauth'">
      <div v-if="hasOpenAIUsageFallback" class="space-y-1">
        <UsageProgressBar
          v-if="usageInfo?.five_hour"
          label="5h"
          :utilization="usageInfo.five_hour.utilization"
          :resets-at="usageInfo.five_hour.resets_at"
          :window-stats="usageInfo.five_hour.window_stats"
          :show-now-when-idle="true"
          color="indigo"
        />
        <UsageProgressBar
          v-if="usageInfo?.seven_day"
          label="7d"
          :utilization="usageInfo.seven_day.utilization"
          :resets-at="usageInfo.seven_day.resets_at"
          :window-stats="usageInfo.seven_day.window_stats"
          :show-now-when-idle="true"
          color="emerald"
        />
      </div>
      <div v-else-if="loading" class="space-y-1.5">
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
      </div>
      <div v-else class="text-xs text-gray-400">-</div>
    </template>

    <!-- Antigravity OAuth accounts: fetch usage from API -->
    <template v-else-if="account.platform === 'antigravity' && account.type === 'oauth'">
      <!-- 账户类型徽章 -->
      <div v-if="antigravityTierLabel" class="mb-1 flex items-center gap-1">
        <span
          :class="[
            'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
            antigravityTierClass
          ]"
        >
          {{ antigravityTierLabel }}
        </span>
        <!-- 不合格账户警告图标 -->
        <span
          v-if="hasIneligibleTiers"
          class="group relative cursor-help"
        >
          <svg
            class="h-3.5 w-3.5 text-red-500"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"
            />
          </svg>
          <span
            class="pointer-events-none absolute left-0 top-full z-50 mt-1 w-80 whitespace-normal break-words rounded bg-gray-900 px-3 py-2 text-xs leading-relaxed text-white opacity-0 shadow-lg transition-opacity group-hover:opacity-100 dark:bg-gray-700"
          >
            {{ t('admin.accounts.ineligibleWarning') }}
          </span>
        </span>
      </div>

      <!-- Forbidden state (403) -->
      <div v-if="isForbidden" class="space-y-1">
        <span
          :class="[
            'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
            forbiddenBadgeClass
          ]"
        >
          {{ forbiddenLabel }}
        </span>
        <div v-if="validationURL" class="flex items-center gap-1">
          <a
            :href="validationURL"
            target="_blank"
            rel="noopener noreferrer"
            class="text-[10px] text-blue-600 hover:text-blue-800 hover:underline dark:text-blue-400 dark:hover:text-blue-300"
            :title="t('admin.accounts.openVerification')"
          >
            {{ t('admin.accounts.openVerification') }}
          </a>
          <button
            type="button"
            class="text-[10px] text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            :title="t('admin.accounts.copyLink')"
            @click="copyValidationURL"
          >
            {{ linkCopied ? t('admin.accounts.linkCopied') : t('admin.accounts.copyLink') }}
          </button>
        </div>
      </div>

      <!-- Needs reauth (401) -->
      <div v-else-if="needsReauth" class="space-y-1">
        <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300">
          {{ t('admin.accounts.needsReauth') }}
        </span>
      </div>

      <!-- Degraded error (non-403, non-401) -->
      <div v-else-if="usageInfo?.error" class="space-y-1">
        <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
          {{ usageErrorLabel }}
        </span>
      </div>

      <!-- Loading state -->
      <div v-else-if="loading" class="space-y-1.5">
        <div class="flex items-center gap-1">
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
          <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        </div>
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="text-xs text-red-500">
        {{ error }}
      </div>

      <!-- Usage data from API -->
      <div v-else-if="hasAntigravityQuotaFromAPI" class="space-y-1">
        <!-- Gemini 3 Pro -->
        <UsageProgressBar
          v-if="antigravity3ProUsageFromAPI !== null"
          :label="t('admin.accounts.usageWindow.gemini3Pro')"
          :utilization="antigravity3ProUsageFromAPI.utilization"
          :resets-at="antigravity3ProUsageFromAPI.resetTime"
          color="indigo"
        />

        <!-- Gemini 3 Flash -->
        <UsageProgressBar
          v-if="antigravity3FlashUsageFromAPI !== null"
          :label="t('admin.accounts.usageWindow.gemini3Flash')"
          :utilization="antigravity3FlashUsageFromAPI.utilization"
          :resets-at="antigravity3FlashUsageFromAPI.resetTime"
          color="emerald"
        />

        <!-- Gemini 3 Image -->
        <UsageProgressBar
          v-if="antigravity3ImageUsageFromAPI !== null"
          :label="t('admin.accounts.usageWindow.gemini3Image')"
          :utilization="antigravity3ImageUsageFromAPI.utilization"
          :resets-at="antigravity3ImageUsageFromAPI.resetTime"
          color="purple"
        />

        <!-- Claude -->
        <UsageProgressBar
          v-if="antigravityClaudeUsageFromAPI !== null"
          :label="t('admin.accounts.usageWindow.claude')"
          :utilization="antigravityClaudeUsageFromAPI.utilization"
          :resets-at="antigravityClaudeUsageFromAPI.resetTime"
          color="amber"
        />

        <div v-if="aiCreditsDisplay" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
          💳 {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
        </div>
      </div>
      <div v-else-if="aiCreditsDisplay" class="text-[10px] text-gray-500 dark:text-gray-400">
        💳 {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
      </div>
      <div v-else class="text-xs text-gray-400">-</div>
    </template>

    <!-- Gemini platform: show quota + local usage window -->
    <template v-else-if="account.platform === 'gemini'">
      <!-- Auth Type + Tier Badge (first line) -->
      <div v-if="geminiAuthTypeLabel" class="mb-1 flex items-center gap-1">
        <span
          :class="[
            'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
            geminiTierClass
          ]"
        >
          {{ geminiAuthTypeLabel }}
        </span>
        <!-- Help icon -->
        <span
          class="group relative cursor-help"
        >
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
              <div><strong>{{ geminiQuotaPolicyChannel }}:</strong></div>
              <div class="pl-2">• {{ geminiQuotaPolicyLimits }}</div>
              <div class="mt-2">
                <a :href="geminiQuotaPolicyDocsUrl" target="_blank" rel="noopener noreferrer" class="text-blue-400 hover:text-blue-300 underline">
                  {{ t('admin.accounts.gemini.quotaPolicy.columns.docs') }} →
                </a>
              </div>
            </div>
          </span>
        </span>
      </div>

      <!-- Usage data or unlimited flow -->
      <div class="space-y-1">
        <div
          v-if="showGeminiTodayStats && todayStats"
          class="mb-0.5 flex items-center"
        >
          <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
            <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
              {{ formatKeyRequests }} req
            </span>
            <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
              {{ formatKeyTokens }}
            </span>
            <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
              A ${{ formatKeyCost }}
            </span>
            <span
              v-if="todayStats.user_cost != null"
              class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
              :title="t('usage.userBilled')"
            >
              U ${{ formatKeyUserCost }}
            </span>
          </div>
        </div>
        <div
          v-else-if="showGeminiTodayStats && todayStatsLoading"
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
        <div v-else-if="error" class="text-xs text-red-500">
          {{ error }}
        </div>
        <!-- Gemini: show daily usage bars when available -->
        <div v-else-if="geminiUsageAvailable" class="space-y-1">
          <UsageProgressBar
            v-for="bar in geminiUsageBars"
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
        <!-- AI Studio Client OAuth: show unlimited flow (no usage tracking) -->
        <div v-else class="text-xs text-gray-400">
          {{ t('admin.accounts.gemini.rateLimit.unlimited') }}
        </div>
      </div>
    </template>

    <!-- Other accounts: no usage window -->
    <template v-else>
      <div class="text-xs text-gray-400">-</div>
    </template>
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div ref="rootRef" v-else>
    <!-- Gemini API Key accounts: show quota info -->
    <AccountQuotaInfo v-if="account.platform === 'gemini'" :account="account" />
    <!-- Key/Bedrock accounts: show today stats + optional quota bars -->
    <div v-else class="space-y-1">
      <!-- Today stats row (requests, tokens, cost, user_cost) -->
      <div
        v-if="todayStats"
        class="mb-0.5 flex items-center"
      >
        <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
          <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
            {{ formatKeyRequests }} req
          </span>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
            {{ formatKeyTokens }}
          </span>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800" :title="t('usage.accountBilled')">
            A ${{ formatKeyCost }}
          </span>
          <span
            v-if="todayStats.user_cost != null"
            class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
            :title="t('usage.userBilled')"
          >
            U ${{ formatKeyUserCost }}
          </span>
        </div>
      </div>
      <!-- Loading skeleton for today stats -->
      <div
        v-else-if="todayStatsLoading"
        class="mb-0.5 flex items-center gap-1"
      >
        <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>

      <!-- API Key accounts with quota limits: show progress bars -->
      <UsageProgressBar
        v-if="quotaDailyBar"
        label="1d"
        :utilization="quotaDailyBar.utilization"
        :resets-at="quotaDailyBar.resetsAt"
        color="indigo"
      />
      <UsageProgressBar
        v-if="quotaWeeklyBar"
        label="7d"
        :utilization="quotaWeeklyBar.utilization"
        :resets-at="quotaWeeklyBar.resetsAt"
        color="emerald"
      />
      <UsageProgressBar
        v-if="quotaTotalBar"
        label="total"
        :utilization="quotaTotalBar.utilization"
        color="purple"
      />

      <!-- No data at all -->
      <div v-if="!todayStats && !todayStatsLoading && !hasApiKeyQuota" class="text-xs text-gray-400">-</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo, GeminiCredentials, WindowStats } from '@/types'
import { buildOpenAIUsageRefreshKey } from '@/utils/accountUsageRefresh'
import { enqueueUsageRequest } from '@/utils/usageLoadQueue'
import { formatCompactNumber } from '@/utils/format'
import UsageProgressBar from './UsageProgressBar.vue'
import AccountQuotaInfo from './AccountQuotaInfo.vue'

// Module-level cache shared across all AccountUsageCell instances
const _usageCache = new Map<number, { data: AccountUsageInfo; ts: number }>()
const USAGE_CACHE_TTL = 5 * 60 * 1000 // 5 minutes

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
const desktopViewportQuery = '(min-width: 768px)'

const unmounted = ref(false)
onBeforeUnmount(() => { unmounted.value = true })

const loading = ref(false)
const activeQueryLoading = ref(false)
const error = ref<string | null>(null)
const usageInfo = ref<AccountUsageInfo | null>(null)
const rootRef = ref<HTMLElement | null>(null)
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
)
const hasEnteredViewport = ref(false)
const pendingAutoLoad = ref(false)
const pendingAutoLoadSource = ref<'passive' | 'active' | undefined>(undefined)

let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null
let visibilityObserver: IntersectionObserver | null = null

// Show usage windows for OAuth and Setup Token accounts
const showUsageWindows = computed(() => {
  // Gemini: we can always compute local usage windows from DB logs (simulated quotas).
  if (props.account.platform === 'gemini') return true
  return props.account.type === 'oauth' || props.account.type === 'setup-token'
})

const shouldFetchUsage = computed(() => {
  if (props.account.platform === 'anthropic') {
    return props.account.type === 'oauth' || props.account.type === 'setup-token'
  }
  if (props.account.platform === 'gemini') {
    return true
  }
  if (props.account.platform === 'antigravity') {
    return props.account.type === 'oauth'
  }
  if (props.account.platform === 'openai') {
    return props.account.type === 'oauth'
  }
  return false
})

const showGeminiTodayStats = computed(() => {
  return props.account.platform === 'gemini' && props.account.type === 'service_account'
})

const geminiUsageAvailable = computed(() => {
  return (
    !!usageInfo.value?.gemini_shared_daily ||
    !!usageInfo.value?.gemini_pro_daily ||
    !!usageInfo.value?.gemini_flash_daily ||
    !!usageInfo.value?.gemini_shared_minute ||
    !!usageInfo.value?.gemini_pro_minute ||
    !!usageInfo.value?.gemini_flash_minute
  )
})

const hasOpenAIUsageFallback = computed(() => {
  if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return false
  return !!usageInfo.value?.five_hour || !!usageInfo.value?.seven_day
})

const openAIUsageRefreshKey = computed(() => buildOpenAIUsageRefreshKey(props.account))

const shouldAutoLoadUsageOnMount = computed(() => {
  return shouldFetchUsage.value
})

const shouldLazyLoadOnMobile = computed(() => {
  return shouldFetchUsage.value && !isDesktopViewport.value
})

// Antigravity quota types (用于 API 返回的数据)
interface AntigravityUsageResult {
  utilization: number
  resetTime: string | null
}

// ===== Antigravity quota from API (usageInfo.antigravity_quota) =====

// 检查是否有从 API 获取的配额数据
const hasAntigravityQuotaFromAPI = computed(() => {
  return usageInfo.value?.antigravity_quota && Object.keys(usageInfo.value.antigravity_quota).length > 0
})

// 从 API 配额数据中获取使用率（多模型取最高使用率）
const getAntigravityUsageFromAPI = (
  modelNames: string[]
): AntigravityUsageResult | null => {
  const quota = usageInfo.value?.antigravity_quota
  if (!quota) return null

  let maxUtilization = 0
  let earliestReset: string | null = null

  for (const model of modelNames) {
    const modelQuota = quota[model]
    if (!modelQuota) continue

    if (modelQuota.utilization > maxUtilization) {
      maxUtilization = modelQuota.utilization
    }
    if (modelQuota.reset_time) {
      if (!earliestReset || modelQuota.reset_time < earliestReset) {
        earliestReset = modelQuota.reset_time
      }
    }
  }

  // 如果没有找到任何匹配的模型
  if (maxUtilization === 0 && earliestReset === null) {
    const hasAnyData = modelNames.some((m) => quota[m])
    if (!hasAnyData) return null
  }

  return {
    utilization: maxUtilization,
    resetTime: earliestReset
  }
}

// Gemini 3 Pro from API
const antigravity3ProUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI(['gemini-3-pro-low', 'gemini-3-pro-high', 'gemini-3-pro-preview'])
)

// Gemini 3 Flash from API
const antigravity3FlashUsageFromAPI = computed(() => getAntigravityUsageFromAPI(['gemini-3-flash']))

// Gemini Image from API
const antigravity3ImageUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI(['gemini-2.5-flash-image', 'gemini-3.1-flash-image', 'gemini-3-pro-image'])
)

// Claude from API (all Claude model variants)
const antigravityClaudeUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI([
    'claude-sonnet-4-5', 'claude-opus-4-5-thinking',
    'claude-sonnet-4-6', 'claude-opus-4-6', 'claude-opus-4-6-thinking',
  ])
)

const aiCreditsDisplay = computed(() => {
  const credits = usageInfo.value?.ai_credits
  if (!credits || credits.length === 0) return null
  const total = credits.reduce((sum, credit) => sum + (credit.amount ?? 0), 0)
  if (total <= 0) return null
  return total.toFixed(0)
})

// Antigravity 账户类型（从 load_code_assist 响应中提取）
const antigravityTier = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return null

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return null

  // 优先取 paidTier，否则取 currentTier
  const paidTier = loadCodeAssist.paidTier as Record<string, unknown> | undefined
  if (paidTier && typeof paidTier.id === 'string') {
    return paidTier.id
  }

  const currentTier = loadCodeAssist.currentTier as Record<string, unknown> | undefined
  if (currentTier && typeof currentTier.id === 'string') {
    return currentTier.id
  }

  return null
})

// Gemini 账户类型（从 credentials 中提取）
const geminiTier = computed(() => {
  if (props.account.platform !== 'gemini') return null
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.tier_id || null
})

const geminiOAuthType = computed(() => {
  if (props.account.platform !== 'gemini') return null
  const creds = props.account.credentials as GeminiCredentials | undefined
  return (creds?.oauth_type || '').trim() || null
})

// Gemini 是否为 Code Assist OAuth
const isGeminiCodeAssist = computed(() => {
  if (props.account.platform !== 'gemini') return false
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.oauth_type === 'code_assist' || (!creds?.oauth_type && !!creds?.project_id)
})

const geminiChannelShort = computed((): 'ai studio' | 'gcp' | 'google one' | 'client' | null => {
  if (props.account.platform !== 'gemini') return null

  // API Key accounts are AI Studio.
  if (props.account.type === 'apikey') return 'ai studio'

  if (geminiOAuthType.value === 'google_one') return 'google one'
  if (isGeminiCodeAssist.value) return 'gcp'
  if (geminiOAuthType.value === 'ai_studio') return 'client'

  // Fallback (unknown legacy data): treat as AI Studio.
  return 'ai studio'
})

const geminiUserLevel = computed((): string | null => {
  if (props.account.platform !== 'gemini') return null

  const tier = (geminiTier.value || '').toString().trim()
  const tierLower = tier.toLowerCase()
  const tierUpper = tier.toUpperCase()

  // Google One: free / pro / ultra
  if (geminiOAuthType.value === 'google_one') {
    if (tierLower === 'google_one_free') return 'free'
    if (tierLower === 'google_ai_pro') return 'pro'
    if (tierLower === 'google_ai_ultra') return 'ultra'

    // Backward compatibility (legacy tier markers)
    if (tierUpper === 'AI_PREMIUM' || tierUpper === 'GOOGLE_ONE_STANDARD') return 'pro'
    if (tierUpper === 'GOOGLE_ONE_UNLIMITED') return 'ultra'
    if (tierUpper === 'FREE' || tierUpper === 'GOOGLE_ONE_BASIC' || tierUpper === 'GOOGLE_ONE_UNKNOWN' || tierUpper === '') return 'free'

    return null
  }

  // GCP Code Assist: standard / enterprise
  if (isGeminiCodeAssist.value) {
    if (tierLower === 'gcp_enterprise') return 'enterprise'
    if (tierLower === 'gcp_standard') return 'standard'

    // Backward compatibility
    if (tierUpper.includes('ULTRA') || tierUpper.includes('ENTERPRISE')) return 'enterprise'
    return 'standard'
  }

  // AI Studio (API Key) and Client OAuth: free / paid
  if (props.account.type === 'apikey' || geminiOAuthType.value === 'ai_studio') {
    if (tierLower === 'aistudio_paid') return 'paid'
    if (tierLower === 'aistudio_free') return 'free'

    // Backward compatibility
    if (tierUpper.includes('PAID') || tierUpper.includes('PAYG') || tierUpper.includes('PAY')) return 'paid'
    if (tierUpper.includes('FREE')) return 'free'
    if (props.account.type === 'apikey') return 'free'
    return null
  }

  return null
})

// Gemini 认证类型（按要求：授权方式简称 + 用户等级）
const geminiAuthTypeLabel = computed(() => {
  if (props.account.platform !== 'gemini') return null
  if (!geminiChannelShort.value) return null
  return geminiUserLevel.value ? `${geminiChannelShort.value} ${geminiUserLevel.value}` : geminiChannelShort.value
})

// Gemini 账户类型徽章样式（统一样式）
const geminiTierClass = computed(() => {
  // Use channel+level to choose a stable color without depending on raw tier_id variants.
  const channel = geminiChannelShort.value
  const level = geminiUserLevel.value

  if (channel === 'client' || channel === 'ai studio') {
    return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
  }

  if (channel === 'google one') {
    if (level === 'ultra') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    if (level === 'pro') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
  }

  if (channel === 'gcp') {
    if (level === 'enterprise') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
  }

  return ''
})

// Gemini 配额政策信息
const geminiQuotaPolicyChannel = computed(() => {
  if (geminiOAuthType.value === 'google_one') {
    return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.channel')
  }
  if (isGeminiCodeAssist.value) {
    return t('admin.accounts.gemini.quotaPolicy.rows.gcp.channel')
  }
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel')
})

const geminiQuotaPolicyLimits = computed(() => {
  const tierLower = (geminiTier.value || '').toString().trim().toLowerCase()

  if (geminiOAuthType.value === 'google_one') {
    if (tierLower === 'google_ai_ultra' || geminiUserLevel.value === 'ultra') {
      return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsUltra')
    }
    if (tierLower === 'google_ai_pro' || geminiUserLevel.value === 'pro') {
      return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsPro')
    }
    return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsFree')
  }

  if (isGeminiCodeAssist.value) {
    if (tierLower === 'gcp_enterprise' || geminiUserLevel.value === 'enterprise') {
      return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsEnterprise')
    }
    return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsStandard')
  }

  // AI Studio (API Key / custom OAuth)
  if (tierLower === 'aistudio_paid' || geminiUserLevel.value === 'paid') {
    return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsPaid')
  }
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree')
})

const geminiQuotaPolicyDocsUrl = computed(() => {
  if (geminiOAuthType.value === 'google_one' || isGeminiCodeAssist.value) {
    return 'https://developers.google.com/gemini-code-assist/resources/quotas'
  }
  return 'https://ai.google.dev/pricing'
})

const geminiUsesSharedDaily = computed(() => {
  if (props.account.platform !== 'gemini') return false
  // Per requirement: Google One & GCP are shared RPD pools (no per-model breakdown).
  return (
    !!usageInfo.value?.gemini_shared_daily ||
    !!usageInfo.value?.gemini_shared_minute ||
    geminiOAuthType.value === 'google_one' ||
    isGeminiCodeAssist.value
  )
})

const geminiUsageBars = computed(() => {
  if (props.account.platform !== 'gemini') return []
  if (!usageInfo.value) return []

  const bars: Array<{
    key: string
    label: string
    utilization: number
    resetsAt: string | null
    windowStats?: WindowStats | null
    color: 'indigo' | 'emerald'
  }> = []

  if (geminiUsesSharedDaily.value) {
    const sharedDaily = usageInfo.value.gemini_shared_daily
    if (sharedDaily) {
      bars.push({
        key: 'shared_daily',
        label: '1d',
        utilization: sharedDaily.utilization,
        resetsAt: sharedDaily.resets_at,
        windowStats: sharedDaily.window_stats,
        color: 'indigo'
      })
    }
    return bars
  }

  const pro = usageInfo.value.gemini_pro_daily
  if (pro) {
    bars.push({
      key: 'pro_daily',
      label: 'pro',
      utilization: pro.utilization,
      resetsAt: pro.resets_at,
      windowStats: pro.window_stats,
      color: 'indigo'
      })
  }

  const flash = usageInfo.value.gemini_flash_daily
  if (flash) {
    bars.push({
      key: 'flash_daily',
      label: 'flash',
      utilization: flash.utilization,
      resetsAt: flash.resets_at,
      windowStats: flash.window_stats,
      color: 'emerald'
    })
  }

  return bars
})

// 账户类型显示标签
const antigravityTierLabel = computed(() => {
  switch (antigravityTier.value) {
    case 'free-tier':
      return t('admin.accounts.tier.free')
    case 'g1-pro-tier':
      return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier':
      return t('admin.accounts.tier.ultra')
    default:
      return null
  }
})

// 账户类型徽章样式
const antigravityTierClass = computed(() => {
  switch (antigravityTier.value) {
    case 'free-tier':
      return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    case 'g1-pro-tier':
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    case 'g1-ultra-tier':
      return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
    default:
      return ''
  }
})

// 检测账户是否有不合格状态（ineligibleTiers）
const hasIneligibleTiers = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return false

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return false

  const ineligibleTiers = loadCodeAssist.ineligibleTiers as unknown[] | undefined
  return Array.isArray(ineligibleTiers) && ineligibleTiers.length > 0
})

// Antigravity 403 forbidden 状态
const isForbidden = computed(() => !!usageInfo.value?.is_forbidden)
const forbiddenType = computed(() => usageInfo.value?.forbidden_type || 'forbidden')
const validationURL = computed(() => usageInfo.value?.validation_url || '')

// 需要重新授权（401）
const needsReauth = computed(() => !!usageInfo.value?.needs_reauth)

// 降级错误标签（rate_limited / network_error）
const usageErrorLabel = computed(() => {
  const code = usageInfo.value?.error_code
  if (code === 'rate_limited') return t('admin.accounts.rateLimited')
  return t('admin.accounts.usageError')
})

const forbiddenLabel = computed(() => {
  switch (forbiddenType.value) {
    case 'validation':
      return t('admin.accounts.forbiddenValidation')
    case 'violation':
      return t('admin.accounts.forbiddenViolation')
    default:
      return t('admin.accounts.forbidden')
  }
})

const forbiddenBadgeClass = computed(() => {
  if (forbiddenType.value === 'validation') {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300'
  }
  return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
})

const linkCopied = ref(false)
const copyValidationURL = async () => {
  if (!validationURL.value) return
  try {
    await navigator.clipboard.writeText(validationURL.value)
    linkCopied.value = true
    setTimeout(() => { linkCopied.value = false }, 2000)
  } catch {
    // fallback: ignore
  }
}

const isAnthropicOAuthOrSetupToken = computed(() => {
  return props.account.platform === 'anthropic' && (props.account.type === 'oauth' || props.account.type === 'setup-token')
})

const loadUsage = async (options?: { source?: 'passive' | 'active'; bypassCache?: boolean }) => {
  if (!shouldFetchUsage.value) return

  // Check cache
  if (!options?.bypassCache) {
    const cached = _usageCache.get(props.account.id)
    if (cached && Date.now() - cached.ts < USAGE_CACHE_TTL) {
      usageInfo.value = cached.data
      loading.value = false
      return
    }
  }

  loading.value = true
  error.value = null

  try {
    const fetchFn = () => adminAPI.accounts.getUsage(props.account.id, options?.source)
    const result = await enqueueUsageRequest(props.account, fetchFn)
    if (!unmounted.value) {
      usageInfo.value = result
      _usageCache.set(props.account.id, { data: result, ts: Date.now() })
    }
  } catch (e: any) {
    if (!unmounted.value) {
      error.value = t('common.error')
      console.error('Failed to load usage:', e)
    }
  } finally {
    if (!unmounted.value) loading.value = false
  }
}

const flushPendingAutoLoad = () => {
  if (!pendingAutoLoad.value) return
  const source = pendingAutoLoadSource.value
  pendingAutoLoad.value = false
  pendingAutoLoadSource.value = undefined
  loadUsage({ source }).catch((e) => {
    console.error('Failed to load deferred usage:', e)
  })
}

const requestAutoLoad = (source?: 'passive' | 'active') => {
  if (!shouldFetchUsage.value) return
  if (shouldLazyLoadOnMobile.value && !hasEnteredViewport.value) {
    pendingAutoLoad.value = true
    pendingAutoLoadSource.value = source
    return
  }
  loadUsage({ source }).catch((e) => {
    console.error('Failed to auto load usage:', e)
  })
}

const detachVisibilityObserver = () => {
  visibilityObserver?.disconnect()
  visibilityObserver = null
}

const attachVisibilityObserver = () => {
  detachVisibilityObserver()
  if (!shouldLazyLoadOnMobile.value || hasEnteredViewport.value) return
  if (typeof window === 'undefined' || typeof IntersectionObserver === 'undefined') {
    hasEnteredViewport.value = true
    flushPendingAutoLoad()
    return
  }
  if (!rootRef.value) return

  visibilityObserver = new IntersectionObserver((entries) => {
    if (!entries.some((entry) => entry.isIntersecting)) return
    hasEnteredViewport.value = true
    detachVisibilityObserver()
    flushPendingAutoLoad()
  }, {
    root: null,
    rootMargin: '200px 0px',
    threshold: 0.01
  })
  visibilityObserver.observe(rootRef.value)
}

const loadActiveUsage = async () => {
  activeQueryLoading.value = true
  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id, 'active')
  } catch (e: any) {
    console.error('Failed to load active usage:', e)
  } finally {
    activeQueryLoading.value = false
  }
}

// ===== API Key quota progress bars =====

interface QuotaBarInfo {
  utilization: number
  resetsAt: string | null
}

const makeQuotaBar = (
  used: number,
  limit: number,
  startKey?: string
): QuotaBarInfo => {
  const utilization = limit > 0 ? (used / limit) * 100 : 0
  let resetsAt: string | null = null
  if (startKey) {
    const extra = props.account.extra as Record<string, unknown> | undefined
    const isDaily = startKey.includes('daily')
    const mode = isDaily
      ? (extra?.quota_daily_reset_mode as string) || 'rolling'
      : (extra?.quota_weekly_reset_mode as string) || 'rolling'

    if (mode === 'fixed') {
      // Use pre-computed next reset time for fixed mode
      const resetAtKey = isDaily ? 'quota_daily_reset_at' : 'quota_weekly_reset_at'
      resetsAt = (extra?.[resetAtKey] as string) || null
    } else {
      // Rolling mode: compute from start + period
      const startStr = extra?.[startKey] as string | undefined
      if (startStr) {
        const startDate = new Date(startStr)
        const periodMs = isDaily ? 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000
        resetsAt = new Date(startDate.getTime() + periodMs).toISOString()
      }
    }
  }
  return { utilization, resetsAt }
}

const hasApiKeyQuota = computed(() => {
  if (props.account.type !== 'apikey' && props.account.type !== 'bedrock') return false
  return (
    (props.account.quota_daily_limit ?? 0) > 0 ||
    (props.account.quota_weekly_limit ?? 0) > 0 ||
    (props.account.quota_limit ?? 0) > 0
  )
})

const quotaDailyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_daily_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_daily_used ?? 0, limit, 'quota_daily_start')
})

const quotaWeeklyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_weekly_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_weekly_used ?? 0, limit, 'quota_weekly_start')
})

const quotaTotalBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_used ?? 0, limit)
})

// ===== Key account today stats formatters =====

const formatKeyRequests = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.requests, { allowBillions: false })
})

const formatKeyTokens = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.tokens)
})

const formatKeyCost = computed(() => {
  if (!props.todayStats) return '0.00'
  return props.todayStats.cost.toFixed(2)
})

const formatKeyUserCost = computed(() => {
  if (!props.todayStats || props.todayStats.user_cost == null) return '0.00'
  return props.todayStats.user_cost.toFixed(2)
})

onMounted(() => {
  if (typeof window !== 'undefined') {
    desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
    isDesktopViewport.value = desktopViewportMediaQuery.matches
    desktopViewportListener = (event: MediaQueryListEvent) => {
      isDesktopViewport.value = event.matches
    }
    if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
      desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.addListener(desktopViewportListener)
    }
  }

  if (!shouldAutoLoadUsageOnMount.value) return
  const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
  requestAutoLoad(source)
})

watch(openAIUsageRefreshKey, (nextKey, prevKey) => {
  if (!prevKey || nextKey === prevKey) return
  if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return

  requestAutoLoad()
})

watch(
  () => props.manualRefreshToken,
  (nextToken, prevToken) => {
    if (nextToken === prevToken) return
    if (!shouldFetchUsage.value) return

    const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
    _usageCache.delete(props.account.id)
    loadUsage({ source, bypassCache: true }).catch((e) => {
      console.error('Failed to refresh usage after manual refresh:', e)
    })
  }
)

watch(
  [rootRef, shouldLazyLoadOnMobile],
  () => {
    if (shouldLazyLoadOnMobile.value) {
      attachVisibilityObserver()
      return
    }
    detachVisibilityObserver()
  },
  { immediate: true, flush: 'post' }
)

watch(isDesktopViewport, (isDesktop) => {
  if (isDesktop) {
    detachVisibilityObserver()
    hasEnteredViewport.value = true
    flushPendingAutoLoad()
    return
  }
  hasEnteredViewport.value = false
  attachVisibilityObserver()
})

onUnmounted(() => {
  detachVisibilityObserver()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    }
  }
  desktopViewportListener = null
  desktopViewportMediaQuery = null
})
</script>
