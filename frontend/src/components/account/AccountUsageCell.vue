<template>
  <div ref="rootRef" v-if="showUsageWindows">
    <!-- Anthropic OAuth and Setup Token accounts: fetch real usage data -->
    <template
      v-if="
        account.platform === 'anthropic' &&
        (account.type === 'oauth' || account.type === 'setup-token')
      "
    >
      <div v-if="loading" class="space-y-1.5">
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

      <div v-else-if="error" class="text-xs text-red-500">
        {{ error }}
      </div>

      <div v-else-if="usageInfo" class="space-y-1">
        <div v-if="usageInfo.error" class="text-xs text-amber-600 dark:text-amber-400 truncate max-w-[200px]" :title="usageInfo.error">
          {{ usageInfo.error }}
        </div>
        <UsageProgressBar
          v-if="usageInfo.five_hour"
          label="5h"
          :utilization="usageInfo.five_hour.utilization"
          :resets-at="usageInfo.five_hour.resets_at"
          :window-stats="usageInfo.five_hour.window_stats"
          color="indigo"
        />
        <UsageProgressBar
          v-if="usageInfo.seven_day"
          label="7d"
          :utilization="usageInfo.seven_day.utilization"
          :resets-at="usageInfo.seven_day.resets_at"
          color="emerald"
        />
        <UsageProgressBar
          v-if="usageInfo.seven_day_sonnet"
          label="7d S"
          :utilization="usageInfo.seven_day_sonnet.utilization"
          :resets-at="usageInfo.seven_day_sonnet.resets_at"
          color="purple"
        />
        <UsageProgressBar
          v-if="usageInfo.seven_day_fable"
          label="7d F"
          :utilization="usageInfo.seven_day_fable.utilization"
          :resets-at="usageInfo.seven_day_fable.resets_at"
          color="amber"
        />
      </div>

      <div v-else class="space-y-1">
        <div class="text-xs text-gray-400">-</div>
      </div>
      <ClaudeResetCreditsCell :account="account" class="mt-1" @redeemed="loadActiveUsage">
        <template v-if="usageInfo" #pre-actions>
          <span
            v-if="usageInfo.source === 'passive'"
            class="text-[9px] text-gray-400 dark:text-gray-500 italic"
          >
            {{ t('admin.accounts.usageWindow.passiveSampled') }}
          </span>
          <button
            type="button"
            class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30 transition-colors disabled:cursor-not-allowed disabled:opacity-50"
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
        </template>
      </ClaudeResetCreditsCell>
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
          :estimated-total-cost="openAISevenDayEstimatedTotalCost"
          :show-now-when-idle="true"
          color="emerald"
        />
        <OpenAIQuotaResetCell :account="account" @account-updated="handleQuotaResetAccountUpdated">
          <template #pre-actions>
            <button
              type="button"
              class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30 transition-colors disabled:cursor-not-allowed disabled:opacity-50"
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
          </template>
        </OpenAIQuotaResetCell>
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
      <div v-else>
        <div class="text-xs text-gray-400">-</div>
        <OpenAIQuotaResetCell
          :account="account"
          class="mt-1"
          @account-updated="handleQuotaResetAccountUpdated"
        />
      </div>
    </template>

    <!-- Antigravity OAuth & Grok OAuth accounts -->
    <AccountAntigravityGrokSection
      v-else-if="(account.platform === 'antigravity' || account.platform === 'grok') && account.type === 'oauth'"
      :account="account"
      :usage-info="usageInfo"
      :loading="loading"
      :error="error"
      :active-query-loading="activeQueryLoading"
      @active-query="loadActiveUsage"
      @grok-probed="handleGrokProbed"
    />

    <!-- Multi-protocol API-key providers (CN vendors, OpenCode, Command Code) -->
    <template v-else-if="isMultiProtocolApiKeyPlatform(account.platform)">
      <OllamaCloudUsageCell
        v-if="account.ollama_cloud_usage?.eligible"
        :account="account"
        @updated="handleOllamaCloudUsageUpdated"
      />
      <OpenCodeGoUsageCell
        v-else-if="account.opencode_go_usage?.eligible"
        :account="account"
        @updated="handleOpenCodeGoUsageUpdated"
      />
      <div v-else class="space-y-1">
        <div
          v-if="!cnQuotaCellVisible && !cnBalanceCellVisible"
          class="text-xs text-gray-400"
          :title="t('admin.accounts.cnProviders.noBalanceEndpoint')"
        >-</div>
        <CNProviderQuotaCell :account="account" />
        <CNProviderBalanceCell :account="account" />
      </div>
    </template>

    <!-- Gemini platform: show quota + local usage window -->
    <AccountGeminiAndKeySection
      v-else-if="account.platform === 'gemini'"
      mode="gemini-window"
      :account="account"
      :usage-info="usageInfo"
      :loading="loading"
      :error="error"
      :today-stats="todayStats"
      :today-stats-loading="todayStatsLoading"
    />

    <!-- Other accounts: no usage window -->
    <template v-else>
      <div class="text-xs text-gray-400">-</div>
    </template>
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div ref="rootRef" v-else>
    <AccountGeminiAndKeySection
      mode="key-fallback"
      :account="account"
      :usage-info="usageInfo"
      :loading="loading"
      :error="error"
      :today-stats="todayStats"
      :today-stats-loading="todayStatsLoading"
      @ollama-updated="handleOllamaCloudUsageUpdated"
      @opencode-updated="handleOpenCodeGoUsageUpdated"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountUsageInfo, WindowStats } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
import ClaudeResetCreditsCell from './ClaudeResetCreditsCell.vue'
import OpenAIQuotaResetCell from './OpenAIQuotaResetCell.vue'
import CNProviderQuotaCell from './CNProviderQuotaCell.vue'
import CNProviderBalanceCell from './CNProviderBalanceCell.vue'
import OllamaCloudUsageCell from './OllamaCloudUsageCell.vue'
import OpenCodeGoUsageCell from './OpenCodeGoUsageCell.vue'
import AccountAntigravityGrokSection from './AccountAntigravityGrokSection.vue'
import AccountGeminiAndKeySection from './AccountGeminiAndKeySection.vue'
import { useAccountUsageLoader } from './useAccountUsageLoader'
import {
  cnQuotaCellVisible as cnQuotaCellVisibleFn,
  cnBalanceCellVisible as cnBalanceCellVisibleFn,
  isMultiProtocolApiKeyPlatform
} from './credentialsBuilder'

const props = withDefaults(
  defineProps<{
    account: Account
    todayStats?: WindowStats | null
    todayStatsLoading?: boolean
    manualRefreshToken?: number
    batchedUsage?: AccountUsageInfo | null
    batchedUsageError?: string | null
    batchedUsageLoading?: boolean
    requestBatchedUsage?: ((account: Account, options?: { force?: boolean }) => void) | null
  }>(),
  {
    todayStats: null,
    todayStatsLoading: false,
    manualRefreshToken: 0,
    batchedUsage: null,
    batchedUsageError: null,
    batchedUsageLoading: false,
    requestBatchedUsage: null
  }
)

const emit = defineEmits<{
  'account-updated': [account: Account]
  'usage-loaded': [usage: AccountUsageInfo]
}>()

const { t } = useI18n()
const rootRef = ref<HTMLElement | null>(null)

const {
  loading,
  activeQueryLoading,
  error,
  usageInfo,
  showUsageWindows,
  hasOpenAIUsageFallback,
  openAISevenDayEstimatedTotalCost,
  loadActiveUsage,
  handleGrokProbed
} = useAccountUsageLoader(
  props,
  rootRef,
  t as (key: string) => string,
  (usage) => {
    emit('usage-loaded', usage)
    // Keep the account's plan badge in sync with this read-only observation.
    // Do not write credentials back or guess the multiplier from legacy Ultra.
    const tier = usage.subscription_tier_raw
    if (props.account.platform === 'antigravity' && tier && !usage.is_forbidden && !usage.needs_reauth &&
        props.account.credentials?.plan_type !== tier) {
      emit('account-updated', {
        ...props.account,
        credentials: { ...props.account.credentials, plan_type: tier }
      })
    }
  }
)

const cnQuotaCellVisible = computed(() => cnQuotaCellVisibleFn(props.account))
const cnBalanceCellVisible = computed(() => cnBalanceCellVisibleFn(props.account))

const handleQuotaResetAccountUpdated = (account: Account) => {
  emit('account-updated', account)
}

const handleOllamaCloudUsageUpdated = (state: NonNullable<Account['ollama_cloud_usage']>) => {
  emit('account-updated', { ...props.account, ollama_cloud_usage: state })
}

const handleOpenCodeGoUsageUpdated = (state: NonNullable<Account['opencode_go_usage']>) => {
  emit('account-updated', { ...props.account, opencode_go_usage: state })
}
</script>
