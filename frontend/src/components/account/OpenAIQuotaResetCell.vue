<template>
  <div v-if="visible" class="space-y-1">
    <!--
      Unified action row. Parents that already render their own "local query"
      affordance (e.g. AccountUsageCell's active-sampling refresh) pass it in
      via the #pre-actions slot so the user sees a single row of related
      buttons rather than two near-duplicate "查询" rows.

      The 5h / 7d window bars are deliberately NOT rendered here — the local
      active-sampling display (UsageProgressBar in AccountUsageCell) already
      owns that real estate. This cell is purely about the rate-limit reset
      credit: query its count, consume one if needed.
    -->
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || resetting"
        :title="countButtonTitle"
        @click="handleQuery()"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
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
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="data"> {{ availableResetCount }}</span>
      </button>

      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="resetting || loading || !canReset"
        :title="resetButtonTitle"
        @click="openResetConfirm"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': resetting }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4"
          />
        </svg>
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>

    <div
      v-if="autoResetState"
      class="flex flex-wrap items-center gap-1 text-[10px]"
      data-testid="auto-reset-credit-state"
    >
      <span
        class="inline-flex items-center rounded px-1.5 py-0.5 font-medium"
        :class="autoResetStateClass"
      >
        {{ autoResetStateLabel }}
        <span v-if="autoResetState.trigger_window" class="ml-1 tabular-nums">
          {{ autoResetState.trigger_window }}
        </span>
      </span>
      <span v-if="autoResetState.checked_at" class="text-gray-500 dark:text-gray-400">
        {{ formatResetCreditExpiry(autoResetState.checked_at, 'short') }}
      </span>
      <span
        v-if="autoResetState.error_code"
        class="max-w-full truncate text-red-600 dark:text-red-400"
        :title="autoResetState.error_code"
      >
        {{ autoResetState.error_code }}
      </span>
    </div>

    <div
      v-if="expiryTarget"
      class="flex min-w-0 flex-wrap items-center gap-1 text-[10px]"
      data-testid="reset-credit-expiry-target-state"
    >
      <Icon name="clock" size="xs" class="shrink-0 text-blue-600 dark:text-blue-400" />
      <span
        class="min-w-0 truncate text-gray-500 tabular-nums dark:text-gray-400"
        :title="t('admin.accounts.openaiQuotaReset.expiryTarget.summary', {
          execution: formatResetCreditExpiry(expiryTargetExecutionAt, 'full'),
          expiry: formatResetCreditExpiry(expiryTarget.expires_at, 'full'),
          lead: formatExpiryTargetLeadTime(expiryTarget.lead_time_minutes)
        })"
      >
        {{ t('admin.accounts.openaiQuotaReset.expiryTarget.scheduledAt', { time: formatResetCreditExpiry(expiryTargetExecutionAt, 'short') }) }}
      </span>
      <button
        type="button"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-100 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-red-400"
        data-testid="reset-credit-expiry-target-cancel"
        :disabled="expiryTargetMutating"
        :title="t('admin.accounts.openaiQuotaReset.expiryTarget.cancelPlan')"
        :aria-label="t('admin.accounts.openaiQuotaReset.expiryTarget.cancelPlan')"
        @click="cancelExpiryTarget"
      >
        <Icon name="x" size="xs" />
      </button>
    </div>

    <div v-if="primaryResetCredit" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span
          class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCredit.expires_at, 'full') })"
        >
          {{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCredit.expires_at, 'short') }) }}
        </span>
        <button
          v-if="canConfigureExpiryTarget(primaryResetCredit)"
          type="button"
          class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          :class="isExpiryTargetCredit(primaryResetCredit) ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'text-gray-500 hover:bg-gray-100 hover:text-blue-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-blue-400'"
          :data-testid="`reset-credit-expiry-target-${primaryResetCredit.id}`"
          :disabled="expiryTargetMutating"
          :title="t('admin.accounts.openaiQuotaReset.expiryTarget.scheduleTooltip')"
          :aria-label="t('admin.accounts.openaiQuotaReset.expiryTarget.scheduleTooltip')"
          @click="openExpiryTargetDialog(primaryResetCredit)"
        >
          <Icon name="clock" size="xs" />
        </button>
        <button
          v-if="hiddenResetCreditCount > 0"
          type="button"
          data-testid="reset-credit-expiry-toggle"
          class="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium leading-4 text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
          :aria-expanded="showResetCreditDetails"
          :aria-label="resetCreditDetailsToggleLabel"
          :title="resetCreditDetailsTitle"
          @click="toggleResetCreditDetails"
        >
          +{{ hiddenResetCreditCount }}
        </button>
      </div>

      <div
        v-if="showResetCreditDetails && additionalResetCredits.length > 0"
        data-testid="reset-credit-expiry-details"
        class="inline-grid max-w-full gap-0.5 rounded border border-gray-200 bg-white px-1.5 py-1 text-[10px] leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
      >
        <span class="sr-only">{{ t('admin.accounts.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(credit, index) in additionalResetCredits"
          :key="`${credit.id || credit.expires_at}-${index}`"
          class="flex min-w-0 items-center gap-1 tabular-nums"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(credit.expires_at, 'full') })"
        >
          <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 dark:bg-dark-500" />
          <span class="min-w-0 flex-1 truncate">{{ formatResetCreditExpiry(credit.expires_at, 'short') }}</span>
          <button
            v-if="canConfigureExpiryTarget(credit)"
            type="button"
            class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            :class="isExpiryTargetCredit(credit) ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'text-gray-500 hover:bg-gray-100 hover:text-blue-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-blue-400'"
            :data-testid="`reset-credit-expiry-target-${credit.id}`"
            :disabled="expiryTargetMutating"
            :title="t('admin.accounts.openaiQuotaReset.expiryTarget.scheduleTooltip')"
            :aria-label="t('admin.accounts.openaiQuotaReset.expiryTarget.scheduleTooltip')"
            @click="openExpiryTargetDialog(credit)"
          >
            <Icon name="clock" size="xs" />
          </button>
        </span>
      </div>
    </div>

    <!-- Error / success feedback -->
    <div
      v-if="error"
      class="text-[10px] text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ truncatedError }}
    </div>
    <div
      v-else-if="resetWarning"
      class="text-[10px] text-amber-600 dark:text-amber-400"
    >
      {{ resetWarning }}
    </div>
    <div
      v-else-if="resetMessage"
      class="text-[10px] text-emerald-600 dark:text-emerald-400"
    >
      {{ resetMessage }}
    </div>

    <ConfirmDialog
      :show="showResetConfirm"
      :title="t('admin.accounts.openaiQuotaReset.confirmTitle')"
      :message="t('admin.accounts.openaiQuotaReset.confirmMessage', { count: availableResetCount })"
      :confirm-text="t('admin.accounts.openaiQuotaReset.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmReset"
      @cancel="showResetConfirm = false"
    />

    <BaseDialog
      :show="showExpiryTargetDialog"
      :title="t('admin.accounts.openaiQuotaReset.expiryTarget.dialogTitle')"
      width="narrow"
      @close="closeExpiryTargetDialog"
    >
      <form class="space-y-4" @submit.prevent="saveExpiryTarget">
        <div class="space-y-1 border-b border-gray-200 pb-3 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openaiQuotaReset.expiryTarget.creditExpiresAt') }}
          </div>
          <div class="text-sm font-medium text-gray-900 tabular-nums dark:text-gray-100">
            {{ selectedResetCredit ? formatResetCreditExpiry(selectedResetCredit.expires_at, 'full') : '-' }}
          </div>
        </div>
        <div>
          <label for="reset-credit-expiry-lead-minutes" class="input-label">
            {{ t('admin.accounts.openaiQuotaReset.expiryTarget.leadTime') }}
          </label>
          <input
            id="reset-credit-expiry-lead-minutes"
            v-model.number="expiryTargetLeadTimeMinutes"
            type="number"
            min="5"
            max="10080"
            step="1"
            class="input"
            data-testid="reset-credit-expiry-lead-minutes"
          />
        </div>
        <div class="border-t border-gray-200 pt-3 text-sm dark:border-dark-700">
          <div class="flex items-baseline justify-between gap-3">
            <span class="text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openaiQuotaReset.expiryTarget.plannedExecution') }}
            </span>
            <span class="text-right font-medium text-gray-900 tabular-nums dark:text-gray-100" data-testid="reset-credit-expiry-execution-at">
              {{ selectedExpiryTargetExecutionAt ? formatResetCreditExpiry(selectedExpiryTargetExecutionAt, 'full') : '-' }}
            </span>
          </div>
        </div>
        <p
          v-if="expiryTargetRunsImmediately"
          class="flex items-start gap-1.5 text-sm text-amber-700 dark:text-amber-300"
          data-testid="reset-credit-expiry-immediate-warning"
        >
          <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
          <span>{{ t('admin.accounts.openaiQuotaReset.expiryTarget.executeImmediately') }}</span>
        </p>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="expiryTargetMutating" @click="closeExpiryTargetDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="expiryTargetMutating" @click="saveExpiryTarget">
            {{ t('admin.accounts.openaiQuotaReset.expiryTarget.savePlan') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  cancelOpenAIResetCreditExpiryTarget,
  refreshOpenAIQuota,
  resetOpenAIQuota,
  setOpenAIResetCreditExpiryTarget,
  type OpenAIRateLimitResetCreditDetail,
  type OpenAIQuotaUsage,
  type OpenAIQuotaResetResult
} from '@/api/admin/accounts'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  account: Account
}>()

const emit = defineEmits<{
  'account-updated': [account: Account]
}>()

const { t } = useI18n()

// Visible only for OpenAI OAuth accounts.
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')

const loading = ref(false)
const resetting = ref(false)
const error = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const cachedData = ref<OpenAIQuotaUsage | null>(null)
const resetMessage = ref<string | null>(null)
const resetWarning = ref<string | null>(null)
const showResetConfirm = ref(false)
const showResetCreditDetails = ref(false)
const showExpiryTargetDialog = ref(false)
const expiryTargetMutating = ref(false)
const selectedResetCredit = ref<SelectableResetCredit | null>(null)
const expiryTargetDefaultLeadTimeMinutes = 60
const expiryTargetMinLeadTimeMinutes = 5
const expiryTargetMaxLeadTimeMinutes = 10080
const expiryTargetLeadTimeMinutes = ref(expiryTargetDefaultLeadTimeMinutes)

type AutoResetCreditState = NonNullable<NonNullable<Account['extra']>['codex_auto_reset_credit_state']>
const validAutoResetStatuses = new Set(['checking', 'available', 'resetting', 'success', 'no_credit', 'failed'])
const autoResetState = computed<AutoResetCreditState | null>(() => {
  const state = props.account.extra?.codex_auto_reset_credit_state
  if (!state || typeof state !== 'object' || !validAutoResetStatuses.has(String(state.status))) return null
  if (props.account.extra?.auto_reset_credit_enabled !== true && state.trigger_reason !== 'expiry_target') return null
  return state
})
const autoResetStateLabel = computed(() => {
  if (!autoResetState.value?.status) return ''
  const keyByStatus: Record<string, string> = {
    checking: 'checking',
    available: 'available',
    resetting: 'resetting',
    success: 'success',
    no_credit: 'noCredit',
    failed: 'failed'
  }
  return t(`admin.accounts.openaiQuotaReset.autoStatus.${keyByStatus[autoResetState.value.status]}`)
})
const autoResetStateClass = computed(() => {
  switch (autoResetState.value?.status) {
    case 'available':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'success':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'no_credit':
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'resetting':
      return 'bg-orange-50 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300'
  }
})

type ResetCreditExpiryTarget = NonNullable<NonNullable<Account['extra']>['auto_reset_credit_expiry_target']>
const expiryTarget = computed<ResetCreditExpiryTarget | null>(() => {
  const target = props.account.extra?.auto_reset_credit_expiry_target
  if (!target || typeof target !== 'object') return null
  if (
    typeof target.plan_id !== 'string' ||
    typeof target.credit_id !== 'string' ||
    typeof target.expires_at !== 'string' ||
    !Number.isInteger(target.lead_time_minutes) ||
    target.lead_time_minutes < expiryTargetMinLeadTimeMinutes ||
    target.lead_time_minutes > expiryTargetMaxLeadTimeMinutes
  ) return null
  return target
})

// Rehydrate the card from the persisted snapshot. Credits that already expired
// are dropped and the count is clamped to what remains: the snapshot has no
// freshness signal, so an unfiltered read would offer to consume credits that no
// longer exist. A snapshot claiming credits with no usable expiration left is
// treated as absent, which keeps the reset button gated on a live query.
const readCachedResetCredits = (account: Account): OpenAIQuotaUsage | null => {
  const cached = account.extra?.codex_reset_credit_snapshot
  if (!cached || typeof cached !== 'object' || Array.isArray(cached)) return null

  const { available_count: count, credits: rawCredits } = cached as {
    available_count?: unknown
    credits?: unknown
  }
  if (typeof count !== 'number' || !Number.isFinite(count)) return null

  const now = Date.now()
  const credits: OpenAIRateLimitResetCreditDetail[] = []
  if (Array.isArray(rawCredits)) {
    for (const credit of rawCredits) {
      if (!credit || typeof credit !== 'object') continue
      const expiresAt = (credit as { expires_at?: unknown }).expires_at
      if (typeof expiresAt !== 'string' || expiresAt.trim() === '') continue
      const expiryTime = new Date(expiresAt).getTime()
      // Unparsable timestamps are kept: they are already rendered verbatim and
      // dropping them would silently understate the available count.
      if (!Number.isNaN(expiryTime) && expiryTime <= now) continue
      const id = (credit as { id?: unknown }).id
      credits.push({
        expires_at: expiresAt,
        ...(typeof id === 'string' && id.trim() !== '' ? { id: id.trim() } : {})
      })
    }
  }
  const availableCount = Math.min(Math.max(count, 0), credits.length)
  // A snapshot that claimed credits but has none left is no longer informative;
  // report "unknown" so the operator re-queries instead of trusting it.
  if (count > 0 && availableCount <= 0) return null
  return {
    fetched_at: 0,
    rate_limit_reset_credits: {
      available_count: availableCount,
      credits
    }
  }
}

cachedData.value = readCachedResetCredits(props.account)
data.value = cachedData.value

// 影子账号的额度查询会 resolve 到母账号,但影子本身不支持重置(后端返回 409);
// 重置必须在母账号上进行。前端据此禁用影子的重置入口(外审 F6)。
const isShadow = computed(() => props.account.parent_account_id != null)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
// Prefer the live payload and fall back to the persisted snapshot only when the
// live state is unknown, so the count and the expirations never come from two
// different generations of the same data.
type SelectableResetCredit = Required<Pick<OpenAIRateLimitResetCreditDetail, 'expires_at'>> & OpenAIRateLimitResetCreditDetail
const resetCredits = computed<SelectableResetCredit[]>(() =>
  ((data.value ?? cachedData.value)?.rate_limit_reset_credits?.credits ?? [])
    .map((credit): SelectableResetCredit => {
      const id = credit.id?.trim()
      return {
        ...credit,
        expires_at: credit.expires_at?.trim() ?? '',
        ...(id ? { id } : {})
      }
    })
    .filter((credit) => credit.expires_at.length > 0)
    .sort((a, b) => compareResetCreditExpiry(a.expires_at, b.expires_at))
)
const resetCreditExpirations = computed(() => resetCredits.value.map((credit) => credit.expires_at))
const primaryResetCredit = computed(() => resetCredits.value[0] ?? null)
const additionalResetCredits = computed(() => resetCredits.value.slice(1))
const hiddenResetCreditCount = computed(() => additionalResetCredits.value.length)
const canReset = computed(() => availableResetCount.value > 0 && !isShadow.value)

const resetCreditDetailsTitle = computed(() =>
  resetCreditExpirations.value
    .map((expiresAt) => formatResetCreditExpiry(expiresAt, 'full'))
    .join('\n')
)

const resetCreditDetailsToggleLabel = computed(() => {
  if (showResetCreditDetails.value) {
    return t('admin.accounts.openaiQuotaReset.collapseExpirations')
  }
  return t('admin.accounts.openaiQuotaReset.expandExpirations', { count: hiddenResetCreditCount.value })
})

const resetButtonTitle = computed(() => {
  if (isShadow.value) return t('admin.accounts.openaiQuotaReset.resetTooltipShadow')
  if (!data.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
  if (!canReset.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNoCredits')
  return t('admin.accounts.openaiQuotaReset.resetTooltipReady')
})

// "次数" button doubles as the upstream-query trigger and the count display.
// Tooltip differs between "click to load" (no data yet) and "click to refresh".
const countButtonTitle = computed(() => {
  if (!data.value) return t('admin.accounts.openaiQuotaReset.countTooltipLoad')
  return t('admin.accounts.openaiQuotaReset.countTooltipRefresh')
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}…` : error.value
})

const getResetCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

const compareResetCreditExpiry = (a: string, b: string): number => {
  const diff = getResetCreditExpiryTime(a) - getResetCreditExpiryTime(b)
  if (diff !== 0) return diff
  return a.localeCompare(b)
}

const formatResetCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') {
    options.year = 'numeric'
  }

  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const canConfigureExpiryTarget = (credit: SelectableResetCredit): boolean => {
  if (isShadow.value || typeof credit.id !== 'string' || credit.id.trim() === '') return false
  const expiresAt = new Date(credit.expires_at).getTime()
  return !Number.isNaN(expiresAt) && expiresAt > Date.now()
}

const isExpiryTargetCredit = (credit: SelectableResetCredit): boolean =>
  Boolean(credit.id && expiryTarget.value?.credit_id === credit.id)

const calculateExpiryTargetExecutionAt = (expiresAt: string, leadTimeMinutes: number): string => {
  const expiry = new Date(expiresAt).getTime()
  if (Number.isNaN(expiry)) return ''
  return new Date(expiry - leadTimeMinutes * 60_000).toISOString()
}

const formatExpiryTargetLeadTime = (minutes: number): string => {
  if (minutes % 60 === 0) {
    return t('admin.accounts.openaiQuotaReset.expiryTarget.durationHours', { count: minutes / 60 })
  }
  return t('admin.accounts.openaiQuotaReset.expiryTarget.durationMinutes', { count: minutes })
}

const expiryTargetExecutionAt = computed(() => {
  if (!expiryTarget.value) return ''
  return calculateExpiryTargetExecutionAt(expiryTarget.value.expires_at, expiryTarget.value.lead_time_minutes)
})

const selectedExpiryTargetExecutionAt = computed(() => {
  if (!selectedResetCredit.value) return ''
  return calculateExpiryTargetExecutionAt(selectedResetCredit.value.expires_at, expiryTargetLeadTimeMinutes.value)
})

const expiryTargetRunsImmediately = computed(() => {
  if (!selectedExpiryTargetExecutionAt.value) return false
  return new Date(selectedExpiryTargetExecutionAt.value).getTime() <= Date.now()
})

const extractErrorMessage = (e: unknown): string => {
  // The project's axios response interceptor (api/client.ts) flattens server
  // errors into { status, code, message, reason, ... } and re-rejects them, so
  // the message lives at the top level rather than under .response.data. Fall
  // back to the raw axios shape for the cancellation/network branches that
  // bypass the flattening, and finally to the generic i18n string.
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const toggleResetCreditDetails = () => {
  if (hiddenResetCreditCount.value <= 0) return
  showResetCreditDetails.value = !showResetCreditDetails.value
}

const openExpiryTargetDialog = (credit: SelectableResetCredit) => {
  if (expiryTargetMutating.value || !canConfigureExpiryTarget(credit)) return
  selectedResetCredit.value = { ...credit }
  expiryTargetLeadTimeMinutes.value = isExpiryTargetCredit(credit)
    ? expiryTarget.value?.lead_time_minutes ?? expiryTargetDefaultLeadTimeMinutes
    : expiryTargetDefaultLeadTimeMinutes
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  showExpiryTargetDialog.value = true
}

const closeExpiryTargetDialog = () => {
  if (expiryTargetMutating.value) return
  showExpiryTargetDialog.value = false
}

const validExpiryTargetInput = (): boolean => {
  if (!selectedResetCredit.value || !canConfigureExpiryTarget(selectedResetCredit.value)) {
    error.value = t('admin.accounts.openaiQuotaReset.expiryTarget.creditUnavailable')
    return false
  }
  if (
    !Number.isInteger(expiryTargetLeadTimeMinutes.value) ||
    expiryTargetLeadTimeMinutes.value < expiryTargetMinLeadTimeMinutes ||
    expiryTargetLeadTimeMinutes.value > expiryTargetMaxLeadTimeMinutes
  ) {
    error.value = t('admin.accounts.openaiQuotaReset.expiryTarget.leadTimeInvalid')
    return false
  }
  return true
}

const saveExpiryTarget = async () => {
  const credit = selectedResetCredit.value
  if (!credit?.id || expiryTargetMutating.value || !validExpiryTargetInput()) return
  expiryTargetMutating.value = true
  showExpiryTargetDialog.value = false
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const account = await setOpenAIResetCreditExpiryTarget(props.account.id, {
      credit_id: credit.id,
      lead_time_minutes: expiryTargetLeadTimeMinutes.value
    })
    emit('account-updated', account)
    resetMessage.value = t('admin.accounts.openaiQuotaReset.expiryTarget.planSaved')
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    expiryTargetMutating.value = false
  }
}

const cancelExpiryTarget = async () => {
  if (expiryTargetMutating.value || !expiryTarget.value) return
  expiryTargetMutating.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const account = await cancelOpenAIResetCreditExpiryTarget(props.account.id)
    emit('account-updated', account)
    resetMessage.value = t('admin.accounts.openaiQuotaReset.expiryTarget.planCanceled')
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    expiryTargetMutating.value = false
  }
}

const handleQuery = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  showResetCreditDetails.value = false
  try {
    const result = await refreshOpenAIQuota(props.account.id)
    // The upstream read succeeded even when the snapshot write was rejected, so
    // the live count is always adopted. Only the persisted view is left alone,
    // which keeps the displayed expirations consistent with what is stored.
    data.value = result
    if (result.cache_persisted) {
      cachedData.value = result
    } else {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

const openResetConfirm = () => {
  if (resetting.value || loading.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  showResetConfirm.value = true
}

const confirmReset = async () => {
  showResetConfirm.value = false
  if (resetting.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  resetting.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(props.account.id)
    showResetCreditDetails.value = false
    if (result.cache_refreshed && result.quota) {
      data.value = result.quota
      cachedData.value = result.quota
    } else {
      // A credit was consumed but the post-reset count could not be read back.
      // Whatever we still hold is one generation stale, so report the count as
      // unknown instead of letting a second consumption start from stale data.
      data.value = null
    }
    if (result.account) emit('account-updated', result.account)

    if (result.warning_code === 'reset_credit_cache_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    } else if (result.warning_code === 'account_state_recovery_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    } else if (result.warning_code === 'account_state_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRefreshFailed')
    } else {
      resetMessage.value = t('admin.accounts.openaiQuotaReset.resetSuccess', {
        windows: result.windows_reset
      })
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    resetting.value = false
  }
}

watch(
  () => props.account.id,
  () => {
    // Account row may be reused across paginated lists; reset local state.
    cachedData.value = readCachedResetCredits(props.account)
    data.value = cachedData.value
    error.value = null
    resetMessage.value = null
    resetWarning.value = null
    loading.value = false
    resetting.value = false
    showResetConfirm.value = false
    showResetCreditDetails.value = false
    showExpiryTargetDialog.value = false
    expiryTargetMutating.value = false
    selectedResetCredit.value = null
    expiryTargetLeadTimeMinutes.value = expiryTargetDefaultLeadTimeMinutes
  }
)

watch(
  resetCreditExpirations,
  () => {
    if (hiddenResetCreditCount.value <= 0) {
      showResetCreditDetails.value = false
    }
  }
)
</script>
