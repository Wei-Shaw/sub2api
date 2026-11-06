<template>
  <!-- Gemini platform inside showUsageWindows: show quota + local usage window -->
  <template v-if="mode === 'gemini-window'">
    <div v-if="geminiAuthTypeLabel" class="mb-1 flex items-center gap-1">
      <span
        :class="[
          'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
          geminiTierClass
        ]"
      >
        {{ geminiAuthTypeLabel }}
      </span>
      <span v-if="showGeminiQuotaPolicy" class="group relative cursor-help">
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

    <div class="space-y-1">
      <div v-if="showGeminiTodayStats && todayStats" class="mb-0.5 flex items-center">
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
      <template v-if="isGeminiVertex">
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
      </template>
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
      <div v-else-if="!showGeminiTodayStats" class="text-xs text-gray-400">
        {{ t('admin.accounts.gemini.rateLimit.unlimited') }}
      </div>
    </div>
  </template>

  <!-- Non-OAuth/Setup-Token accounts -->
  <template v-else>
    <AccountQuotaInfo v-if="account.platform === 'gemini' && !isGeminiThirdPartyApiKey" :account="account" />
    <div v-else class="space-y-1">
      <OllamaCloudUsageCell
        v-if="account.ollama_cloud_usage?.eligible"
        :account="account"
        @updated="(s) => $emit('ollama-updated', s)"
      />
      <OpenCodeGoUsageCell
        v-else-if="account.opencode_go_usage?.eligible"
        :account="account"
        @updated="(s) => $emit('opencode-updated', s)"
      />
      <div v-if="todayStats" class="mb-0.5 flex items-center">
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
      <div v-else-if="todayStatsLoading" class="mb-0.5 flex items-center gap-1">
        <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>

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

      <div
        v-if="!todayStats && !todayStatsLoading && !hasApiKeyQuota && !account.ollama_cloud_usage?.eligible && !account.opencode_go_usage?.eligible"
        class="text-xs text-gray-400"
      >-</div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountUsageInfo, WindowStats } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import AccountQuotaInfo from './AccountQuotaInfo.vue'
import OllamaCloudUsageCell from './OllamaCloudUsageCell.vue'
import OpenCodeGoUsageCell from './OpenCodeGoUsageCell.vue'
import { useGeminiAndKeyUsage } from './useGeminiAndKeyUsage'

const props = defineProps<{
  mode: 'gemini-window' | 'key-fallback'
  account: Account
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
  todayStats?: WindowStats | null
  todayStatsLoading?: boolean
}>()

defineEmits<{
  'ollama-updated': [state: NonNullable<Account['ollama_cloud_usage']>]
  'opencode-updated': [state: NonNullable<Account['opencode_go_usage']>]
}>()

const { t } = useI18n()

const {
  isGeminiThirdPartyApiKey,
  isGeminiVertex,
  showGeminiTodayStats,
  geminiUsageAvailable,
  showGeminiQuotaPolicy,
  geminiAuthTypeLabel,
  geminiTierClass,
  geminiQuotaPolicyChannel,
  geminiQuotaPolicyLimits,
  geminiQuotaPolicyDocsUrl,
  geminiUsageBars,
  hasApiKeyQuota,
  quotaDailyBar,
  quotaWeeklyBar,
  quotaTotalBar,
  formatKeyRequests,
  formatKeyTokens,
  formatKeyCost,
  formatKeyUserCost
} = useGeminiAndKeyUsage(
  toRef(props, 'account'),
  toRef(props, 'usageInfo'),
  toRef(props, 'todayStats'),
  t as (key: string, named?: Record<string, unknown>) => string
)
</script>
