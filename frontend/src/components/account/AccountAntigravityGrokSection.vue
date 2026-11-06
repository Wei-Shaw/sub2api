<template>
  <!-- Antigravity OAuth accounts: fetch usage from API -->
  <template v-if="account.platform === 'antigravity' && account.type === 'oauth'">
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
      <span v-if="hasIneligibleTiers" class="group relative cursor-help">
        <svg class="h-3.5 w-3.5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
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

    <!-- Loading state -->
    <div v-if="loading" class="space-y-1.5">
      <div class="flex items-center gap-1">
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
    </div>

    <!-- Content / Degraded Error / Request Error -->
    <div v-else class="space-y-1">
      <!-- Keep the last quota snapshot and retry available on upstream errors. -->
      <div v-if="usageInfo?.error && !isForbidden && !needsReauth">
        <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
          {{ usageErrorLabel }}
        </span>
      </div>
      <div v-else-if="error" class="text-xs text-red-500">
        {{ error }}
      </div>

      <AntigravityQuotaPanel :key="account.id" :usage="usageInfo" />
      <div v-if="aiCreditsDisplay" class="text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
      </div>

      <div class="mt-1 flex items-center gap-1">
        <button
          type="button"
          data-test="antigravity-active-query"
          class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30 transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="activeQueryLoading"
          @click="$emit('active-query')"
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
  </template>

  <!-- Grok OAuth accounts: passive xAI quota headers + local Sub2API usage -->
  <template v-else-if="account.platform === 'grok' && account.type === 'oauth'">
    <div v-if="loading" class="space-y-1.5">
      <div class="flex items-center gap-1">
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
        <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      </div>
    </div>
    <div v-else-if="error" class="text-xs text-red-500">
      {{ error }}
    </div>
    <div v-else-if="needsReauth" class="space-y-1">
      <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300">
        {{ t('admin.accounts.needsReauth') }}
      </span>
    </div>
    <div v-else-if="isForbidden" class="space-y-1">
      <span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300">
        {{ usageInfo?.grok_entitlement_status || t('admin.accounts.forbidden') }}
      </span>
    </div>
    <div v-else-if="usageInfo" class="space-y-1">
      <template v-if="grokIsFree">
        <UsageProgressBar
          v-if="grokFreeTokenBar"
          label="24h"
          :title="t('admin.accounts.usageWindow.grokFreeQuota24hHint', { limit: formatCompactNumber(grokFreeTokenBar.limit) })"
          :utilization="grokFreeTokenBar.utilization"
          :window-stats="grokFreeQuotaUsage"
          :show-now-when-idle="true"
          color="emerald"
        />
        <div v-else-if="grokQuotaUnknown" class="text-[10px] text-gray-500 dark:text-gray-400">
          {{ grokQuotaUnknownLabel }}
        </div>
      </template>
      <template v-else>
        <UsageProgressBar
          v-if="grokWeeklyBillingBar"
          label="7d"
          :utilization="grokWeeklyBillingBar.utilization"
          :resets-at="grokWeeklyBillingBar.resetsAt"
          :window-stats="grokWeeklyBillingBar.windowStats"
          :show-now-when-idle="true"
          color="indigo"
        />
        <UsageProgressBar
          v-if="grokMonthlyBillingBar"
          label="30d"
          :utilization="grokMonthlyBillingBar.utilization"
          :resets-at="grokMonthlyBillingBar.resetsAt"
          :window-stats="grokMonthlyBillingBar.windowStats"
          :show-now-when-idle="true"
          color="indigo"
        />
        <div
          v-if="grokPrepaidMoneyLine"
          class="flex flex-wrap items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400"
        >
          <span
            v-if="grokPrepaidMoneyLine.showPrepaid"
            class="rounded bg-emerald-50 px-1 py-0.5 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
            :title="t('admin.accounts.usageWindow.grokPrepaid')"
          >
            {{ t('admin.accounts.usageWindow.grokPrepaid') }} ${{ grokPrepaidMoneyLine.prepaid }}
          </span>
          <span
            v-if="grokPrepaidMoneyLine.showUsedLimit"
            :title="t('admin.accounts.usageWindow.grokMonthlyLimit')"
          >
            {{ t('admin.accounts.usageWindow.grokUsed') }}
            {{ grokPrepaidMoneyLine.used }}/{{ grokPrepaidMoneyLine.limit }}
          </span>
        </div>
        <div v-if="grokQuotaUnknown" class="text-[10px] text-gray-500 dark:text-gray-400">
          {{ grokQuotaUnknownLabel }}
        </div>
      </template>
      <div v-if="usageInfo.error" class="truncate text-xs text-amber-600 dark:text-amber-400 max-w-[200px]" :title="usageInfo.error">
        {{ usageErrorLabel }}
      </div>
      <div v-if="grokRetryAfterLabel" class="text-[10px] text-amber-600 dark:text-amber-400">
        {{ t('admin.accounts.usageWindow.grokRetryAfter', { time: grokRetryAfterLabel }) }}
      </div>
      <GrokQuotaProbeCell :account="account" compact @probed="$emit('grok-probed')" />
    </div>
    <div v-else class="space-y-1">
      <div class="text-xs text-gray-400">-</div>
      <GrokQuotaProbeCell :account="account" compact @probed="$emit('grok-probed')" />
    </div>
  </template>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountUsageInfo } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import UsageProgressBar from './UsageProgressBar.vue'
import GrokQuotaProbeCell from './GrokQuotaProbeCell.vue'
import AntigravityQuotaPanel from './AntigravityQuotaPanel.vue'
import { useAntigravityAndGrokUsage } from './useAntigravityAndGrokUsage'

const props = defineProps<{
  account: Account
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
  activeQueryLoading: boolean
}>()

defineEmits<{
  'active-query': []
  'grok-probed': []
}>()

const { t } = useI18n()

const {
  aiCreditsDisplay,
  antigravityTierLabel,
  antigravityTierClass,
  hasIneligibleTiers,
  isForbidden,
  validationURL,
  needsReauth,
  usageErrorLabel,
  forbiddenLabel,
  forbiddenBadgeClass,
  linkCopied,
  copyValidationURL,
  grokWeeklyBillingBar,
  grokMonthlyBillingBar,
  grokPrepaidMoneyLine,
  grokIsFree,
  grokFreeQuotaUsage,
  grokFreeTokenBar,
  grokQuotaUnknown,
  grokQuotaUnknownLabel,
  grokRetryAfterLabel
} = useAntigravityAndGrokUsage(
  toRef(props, 'account'),
  toRef(props, 'usageInfo'),
  t as (key: string, named?: Record<string, unknown>) => string
)
</script>
