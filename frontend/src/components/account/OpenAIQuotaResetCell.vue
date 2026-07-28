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
        @click="handleQuery"
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

    <div v-if="primaryResetCreditExpiry" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span
          class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'full') })"
        >
          {{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'short') }) }}
        </span>
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
        v-if="showResetCreditDetails && resetCreditExpirations.length > 1"
        data-testid="reset-credit-expiry-details"
        class="inline-grid max-w-full gap-0.5 rounded border border-gray-200 bg-white px-1.5 py-1 text-[10px] leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
      >
        <span class="sr-only">{{ t('admin.accounts.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(expiresAt, index) in resetCreditExpirations"
          :key="`${expiresAt}-${index}`"
          class="flex min-w-0 items-center gap-1 tabular-nums"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(expiresAt, 'full') })"
        >
          <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 dark:bg-dark-500" />
          <span class="truncate">{{ formatResetCreditExpiry(expiresAt, 'short') }}</span>
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  queryOpenAIQuota,
  resetOpenAIQuota,
  type OpenAIQuotaUsage,
  type OpenAIQuotaResetResult
} from '@/api/admin/accounts'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

// Visible only for OpenAI OAuth accounts.
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')

const loading = ref(false)
const resetting = ref(false)
const error = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const resetMessage = ref<string | null>(null)
const showResetConfirm = ref(false)
const showResetCreditDetails = ref(false)

// 影子账号的额度查询会 resolve 到母账号,但影子本身不支持重置(后端返回 409);
// 重置必须在母账号上进行。前端据此禁用影子的重置入口(外审 F6)。
const isShadow = computed(() => props.account.parent_account_id != null)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
const resetCreditExpirations = computed(() =>
  (data.value?.rate_limit_reset_credits?.credits ?? [])
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareResetCreditExpiry)
)
const primaryResetCreditExpiry = computed(() => resetCreditExpirations.value[0] ?? '')
const hiddenResetCreditCount = computed(() => Math.max(resetCreditExpirations.value.length - 1, 0))
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

const handleQuery = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  resetMessage.value = null
  showResetCreditDetails.value = false
  try {
    data.value = await queryOpenAIQuota(props.account.id)
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
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(props.account.id)
    // Refresh the reset-credit count so the badge reflects the consumed credit.
    // handleQuery clears resetMessage on entry, so the success toast is set
    // AFTER it resolves.
    await handleQuery()
    resetMessage.value = t('admin.accounts.openaiQuotaReset.resetSuccess', {
      windows: result.windows_reset
    })
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
    data.value = null
    error.value = null
    resetMessage.value = null
    loading.value = false
    resetting.value = false
    showResetConfirm.value = false
    showResetCreditDetails.value = false
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
