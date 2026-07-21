<template>
  <div
    v-if="eligible"
    class="inline-flex w-fit min-w-0 flex-col items-start gap-2"
    data-testid="upstream-information"
  >
    <div
      class="inline-flex h-6 w-fit max-w-[13rem] items-center gap-[10px] rounded bg-gray-100/70 px-1 transition-colors hover:bg-gray-100 dark:bg-dark-700/60 dark:hover:bg-dark-700"
      data-testid="upstream-rate-row"
      :aria-busy="probing"
    >
      <span
        class="shrink-0 text-left text-[10px] font-medium text-gray-500 dark:text-gray-400"
        :class="metricLabelWidthClass"
      >
        {{ t('admin.accounts.upstreamBilling.rateLabel') }}
      </span>
      <div class="flex min-w-0 items-center gap-0.5 overflow-hidden">
        <HelpTooltip
          class="!ml-0 shrink-0"
          :class="metricValueWidthClass"
          width-class="w-max max-w-[calc(100vw-2rem)]"
          data-testid="upstream-billing-details"
        >
          <template #trigger>
            <span
              class="block w-full truncate text-left leading-5 tabular-nums"
              :class="rateValueClass"
              data-testid="upstream-billing-rate"
            >
              {{ primaryValue }}
            </span>
          </template>
          <div class="space-y-2">
            <section class="space-y-1" data-testid="upstream-rate-details">
              <p class="font-semibold">{{ t('admin.accounts.upstreamBilling.rateDetails') }}</p>
              <template v-if="hasEffectiveRate && data">
                <p>{{ t('admin.accounts.upstreamBilling.groupRate', { value: data.group_rate_multiplier }) }}</p>
                <p v-if="data.user_rate_multiplier != null">
                  {{ t('admin.accounts.upstreamBilling.userRate', { value: data.user_rate_multiplier }) }}
                </p>
                <p>
                  {{
                    data.peak_rate_enabled
                      ? t('admin.accounts.upstreamBilling.peakRate', {
                          start: data.peak_start,
                          end: data.peak_end,
                          value: data.peak_rate_multiplier,
                          timezone: data.timezone
                        })
                      : t('admin.accounts.upstreamBilling.noPeakRate')
                  }}
                </p>
                <p>{{ t('admin.accounts.upstreamBilling.effectiveRate', { value: currentEffectiveRate ?? '-' }) }}</p>
                <p>{{ t('admin.accounts.upstreamBilling.updatedAt', { value: formatDate(snapshot?.received_at) }) }}</p>
              </template>
              <template v-else-if="cachedRate">
                <p data-testid="upstream-billing-last-rate">
                  {{ t('admin.accounts.upstreamBilling.lastDetectedRate', { value: lastDetectedRate }) }}
                </p>
                <p data-testid="upstream-billing-last-time">
                  {{ t('admin.accounts.upstreamBilling.lastDetectedAt', { value: formatDate(snapshot?.received_at) }) }}
                </p>
                <p data-testid="upstream-billing-elapsed">
                  {{ t('admin.accounts.upstreamBilling.elapsedSince', { value: elapsedSinceLastSuccess }) }}
                </p>
              </template>
              <p v-else>{{ statusLabel || '-' }}</p>
              <p
                v-if="probeReasonText && !rateFailed"
                class="mt-1 text-amber-300"
                data-testid="upstream-billing-probe-reason"
              >
                {{ probeReasonText }}
              </p>
              <p
                v-if="automaticProbeActive && nextProbeAt"
                data-testid="upstream-billing-next-probe"
              >
                {{ t('admin.accounts.upstreamBilling.nextProbeAt', { value: formatDate(nextProbeAt) }) }}
              </p>
              <p class="mt-2 border-t border-white/15 pt-2" data-testid="upstream-billing-probe-state">
                {{ t('admin.accounts.upstreamBilling.accountProbeState') }}
                <span :class="probeEnabled ? 'text-emerald-400' : 'text-red-400'">
                  {{ probeEnabled ? t('admin.accounts.upstreamBilling.enabled') : t('admin.accounts.upstreamBilling.disabled') }}
                </span>
              </p>
              <p
                v-if="globalProbeEnabled === false"
                class="mt-1"
                data-testid="upstream-billing-global-probe-state"
              >
                {{ t('admin.accounts.upstreamBilling.globalProbeState') }}
                <span class="text-red-400">{{ t('admin.accounts.upstreamBilling.disabled') }}</span>
              </p>
            </section>
            <p
              v-if="cachedRate && !rateFailed"
              class="font-medium text-amber-300"
              data-testid="upstream-billing-stale-notice"
            >
              {{
                t(
                  !automaticProbeActive
                    ? 'admin.accounts.upstreamBilling.manualProbeNotice'
                    : scheduledProbeOverdue
                      ? 'admin.accounts.upstreamBilling.staleProbeOverdueNotice'
                      : 'admin.accounts.upstreamBilling.staleCacheNotice',
                  { age: elapsedSinceLastSuccess }
                )
              }}
            </p>
            <div
              v-if="rateFailureReasonText"
              class="space-y-1 border-t border-white/15 pt-2 text-red-300"
              data-testid="upstream-billing-error-details"
            >
              <p class="font-semibold">{{ t('admin.accounts.upstreamBilling.probeErrorTitle') }}</p>
              <p data-testid="upstream-billing-error-message">
                {{
                  t('admin.accounts.upstreamBilling.probeErrorDetail', {
                    time: formatDate(rateFailureAt),
                    reason: rateFailureReasonText
                  })
                }}
              </p>
              <p data-testid="upstream-billing-error-guidance">
                {{ rateFailureGuidanceText }}
              </p>
            </div>
          </div>
        </HelpTooltip>
        <span
          v-if="hasEffectiveRate && statusLabel && !rateFailed"
          :class="statusClass"
          class="shrink-0 whitespace-nowrap rounded bg-gray-100 px-1 text-[10px] font-medium leading-4 dark:bg-gray-800"
        >
          {{ statusLabel }}
        </span>
        <button
          type="button"
          class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-white/80 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-white/10"
          :class="rateActionColorClass"
          :disabled="probing"
          :aria-label="t('admin.accounts.upstreamBilling.manualProbe')"
          :title="t('admin.accounts.upstreamBilling.manualProbe')"
          data-testid="upstream-billing-probe"
          @click="$emit('probe')"
        >
          <Icon :name="rateActionIcon" size="xs" :stroke-width="2" :class="{ 'animate-spin': probing }" />
        </button>
      </div>
    </div>
    <div
      class="inline-flex h-6 w-fit max-w-[13rem] items-center gap-[10px] rounded bg-emerald-50/50 px-1 transition-colors hover:bg-emerald-50/80 dark:bg-emerald-900/15 dark:hover:bg-emerald-900/25"
      data-testid="upstream-quota-row"
      :aria-busy="quotaLoading"
    >
      <span
        class="shrink-0 text-left text-[10px] font-medium text-gray-500 dark:text-gray-400"
        :class="metricLabelWidthClass"
      >
        {{ t('admin.accounts.upstreamBilling.quotaLabel') }}
      </span>
      <div class="flex min-w-0 items-center gap-0.5 overflow-hidden">
        <HelpTooltip
          v-if="quotaRemaining != null"
          class="!ml-0 shrink-0"
          :class="metricValueWidthClass"
          :content="quotaPrimaryValue"
          :open-delay-ms="500"
          width-class="w-max max-w-[calc(100vw-2rem)]"
          tooltip-class="!py-2"
          data-testid="upstream-quota-details"
        >
          <template #trigger>
            <span
              class="block w-full truncate rounded-sm text-left leading-5 tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/30"
              :class="quotaValueClass(quotaRemaining)"
              :aria-label="quotaPrimaryValue"
              aria-live="polite"
              data-testid="upstream-quota-value"
              tabindex="0"
            >
              {{ quotaPrimaryValue }}
            </span>
          </template>
          <span :class="quotaValueColorClass(quotaRemaining)">{{ quotaPrimaryValue }}</span>
        </HelpTooltip>
        <span
          v-else
          class="block shrink-0 truncate text-left leading-5 tabular-nums"
          :class="[metricValueWidthClass, quotaValueClass(quotaRemaining)]"
          aria-live="polite"
          data-testid="upstream-quota-value"
        >
          {{ quotaPrimaryValue }}
        </span>
        <span
          v-if="quota && quotaError"
          class="shrink-0 whitespace-nowrap rounded bg-red-50 px-1 text-[10px] font-medium leading-4 text-red-600 dark:bg-red-900/30 dark:text-red-300"
        >
          {{ t('admin.accounts.upstreamBilling.quotaFailed') }}
        </span>
        <button
          type="button"
          class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-white/80 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-white/10"
          :class="quotaActionColorClass"
          :disabled="quotaLoading"
          :aria-label="quotaActionLabel"
          :title="quotaActionLabel"
          data-testid="upstream-quota-query"
          @click="$emit('query-quota')"
        >
          <Icon
            :name="quotaActionIcon"
            size="xs"
            :stroke-width="2"
            :class="{ 'animate-spin': quotaLoading }"
          />
        </button>
      </div>
    </div>
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import { currencySymbol } from '@/components/payment/currency'
import type { Account, UpstreamBillingProbeSnapshot, UpstreamQuotaQueryResult } from '@/types'

type ActionFeedback = 'success' | 'error'

const props = withDefaults(defineProps<{
  account: Account
  now: number
  probing?: boolean
  globalProbeEnabled?: boolean
  quotaResult?: UpstreamQuotaQueryResult | null
  quotaError?: string | null
  quotaLoading?: boolean
  rateError?: boolean
  rateErrorAt?: string | null
  rateFeedback?: ActionFeedback | null
  quotaFeedback?: ActionFeedback | null
}>(), {
  globalProbeEnabled: true,
  quotaLoading: false,
  rateError: false,
  rateErrorAt: null,
  rateFeedback: null,
  quotaFeedback: null
})

defineEmits<{
  (event: 'probe'): void
  (event: 'query-quota'): void
}>()

const { t, locale } = useI18n()
const CLOCK_SKEW_TOLERANCE_MS = 5 * 60 * 1000
const compactMetricLayout = computed(() => locale.value.startsWith('zh'))
const metricLabelWidthClass = computed(() => compactMetricLayout.value ? 'w-5' : 'w-10')
const metricValueWidthClass = computed(() => compactMetricLayout.value ? 'w-[46px]' : 'w-16')
const eligible = computed(() => props.account.platform === 'openai' && props.account.type === 'apikey')
const snapshot = computed<UpstreamBillingProbeSnapshot | undefined>(() => props.account.extra?.upstream_billing_probe)
const data = computed(() => snapshot.value?.data)
const quota = computed(() => props.quotaResult?.quota ?? null)
const probeEnabled = computed(() => props.account.extra?.upstream_billing_probe_enabled === true)
const automaticProbeActive = computed(() => probeEnabled.value && props.globalProbeEnabled !== false)
const nextProbeAt = computed(() => {
  const value = snapshot.value?.next_probe_at
  return typeof value === 'string' && Number.isFinite(Date.parse(value)) ? value : ''
})
const scheduledProbeOverdue = computed(() => (
  automaticProbeActive.value &&
  nextProbeAt.value !== '' &&
  props.now > Date.parse(nextProbeAt.value) + CLOCK_SKEW_TOLERANCE_MS
))
const receivedAt = computed(() => typeof snapshot.value?.received_at === 'string' ? Date.parse(snapshot.value.received_at) : Number.NaN)
const freshUntil = computed(() => {
  if (typeof snapshot.value?.fresh_until === 'string') return Date.parse(snapshot.value.fresh_until)
  if (snapshot.value?.status !== 'ok' || typeof snapshot.value.next_probe_at !== 'string') return Number.NaN
  const nextProbeAt = Date.parse(snapshot.value.next_probe_at)
  return Number.isFinite(nextProbeAt) && nextProbeAt > receivedAt.value
    ? receivedAt.value + 2 * (nextProbeAt - receivedAt.value)
    : Number.NaN
})
const validTimestamps = computed(() => {
  if (!Number.isFinite(receivedAt.value) || receivedAt.value > props.now + CLOCK_SKEW_TOLERANCE_MS) return false
  return Number.isFinite(freshUntil.value) && freshUntil.value > receivedAt.value
})
const stale = computed(() => {
  if (!snapshot.value) return false
  if (!Number.isFinite(receivedAt.value)) return snapshot.value.status === 'ok'
  if (!validTimestamps.value) return true
  return props.now > freshUntil.value
})
const parseMinute = (value?: string) => {
  if (typeof value !== 'string') return null
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  return hour < 24 && minute < 60 ? hour * 60 + minute : null
}
const minuteInTimeZone = (timestamp: number, timeZone?: string) => {
  if (!timeZone) return null
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone,
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23'
    }).formatToParts(new Date(timestamp))
    const hour = Number(parts.find(part => part.type === 'hour')?.value)
    const minute = Number(parts.find(part => part.type === 'minute')?.value)
    return Number.isInteger(hour) && Number.isInteger(minute) ? hour * 60 + minute : null
  } catch {
    return null
  }
}
const currentEffectiveRate = computed(() => {
  const billing = data.value
  if (!billing) return null
  if (billing.billing_scope !== 'token') return null
  const base = billing.resolved_rate_multiplier
  if (typeof base !== 'number' || !Number.isFinite(base) || base < 0) return null
  if (typeof billing.peak_rate_enabled !== 'boolean') return null
  if (!billing.peak_rate_enabled) return base
  const start = parseMinute(billing.peak_start)
  const end = parseMinute(billing.peak_end)
  const minute = minuteInTimeZone(props.now, billing.timezone)
  const peak = billing.peak_rate_multiplier
  if (start == null || end == null || minute == null || start >= end || typeof peak !== 'number' || !Number.isFinite(peak) || peak < 0) return null
  const value = minute >= start && minute < end ? base * peak : base
  return Number.isFinite(value) ? value : null
})
const lastDetectedRate = computed(() => {
  const value = data.value?.effective_rate_multiplier
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Number(value.toPrecision(12))
    : null
})
const elapsedSinceLastSuccess = computed(() => {
  if (!Number.isFinite(receivedAt.value)) return '-'
  const elapsedMinutes = Math.max(0, Math.floor((props.now - receivedAt.value) / 60_000))
  if (elapsedMinutes < 1) return t('admin.accounts.upstreamBilling.justNow')
  if (elapsedMinutes < 60) return t('admin.accounts.upstreamBilling.minutesAgo', { count: elapsedMinutes })
  return t('admin.accounts.upstreamBilling.hoursMinutesAgo', {
    hours: Math.floor(elapsedMinutes / 60),
    minutes: elapsedMinutes % 60
  })
})
const effectiveRate = computed(() => {
  if (!validTimestamps.value || stale.value || !['ok', 'failed'].includes(snapshot.value?.status ?? '')) return '-'
  const value = currentEffectiveRate.value
  return value == null ? '-' : `${Number(value.toPrecision(12))}x`
})
const cachedRate = computed(() => {
  if (!stale.value || !['ok', 'failed'].includes(snapshot.value?.status ?? '')) return ''
  if (!validTimestamps.value) return ''
  return lastDetectedRate.value == null ? '' : `${lastDetectedRate.value}x`
})
const rateFailed = computed(() => props.rateError || snapshot.value?.status === 'failed')
const statusLabel = computed(() => {
  if (props.rateError) return t('admin.accounts.upstreamBilling.failed')
  if (!snapshot.value) return t('admin.accounts.upstreamBilling.notProbed')
  if (snapshot.value.status === 'unsupported') return t('admin.accounts.upstreamBilling.unsupported')
  if (snapshot.value.status === 'failed') return t('admin.accounts.upstreamBilling.failed')
  if (stale.value) return t('admin.accounts.upstreamBilling.stale')
  return ''
})
const statusClass = computed(() => {
  if (props.rateError) return 'text-red-600 dark:text-red-400'
  if (!snapshot.value) return 'text-gray-600 dark:text-gray-300'
  if (snapshot.value.status === 'unsupported') return 'text-gray-600 dark:text-gray-300'
  if (snapshot.value.status === 'failed') return 'text-red-600 dark:text-red-400'
  if (stale.value) return 'text-amber-600 dark:text-amber-400'
  return ''
})
const hasEffectiveRate = computed(() => effectiveRate.value !== '-')
const hasNumericRate = computed(() => hasEffectiveRate.value || Boolean(cachedRate.value))
const primaryValue = computed(() => effectiveRate.value !== '-' ? effectiveRate.value : cachedRate.value || statusLabel.value || '-')
const probeReasonText = computed(() => {
  const current = snapshot.value
  if (!current || !['unsupported', 'failed'].includes(current.status)) return ''
  const httpStatus = current.http_status ?? 0

  if (current.status === 'unsupported') {
    if (httpStatus === 404) return t('admin.accounts.upstreamBilling.unsupportedNotFound')
    if (httpStatus === 405) return t('admin.accounts.upstreamBilling.unsupportedMethod')
    return t('admin.accounts.upstreamBilling.unsupportedGeneric')
  }

  if (httpStatus === 401 || httpStatus === 403) {
    return t('admin.accounts.upstreamBilling.probeAuthFailed', { status: httpStatus })
  }
  if (httpStatus === 429) return t('admin.accounts.upstreamBilling.probeRateLimited')
  if (httpStatus >= 500) {
    return t('admin.accounts.upstreamBilling.probeServerError', { status: httpStatus })
  }

  switch (current.last_error) {
    case 'invalid_response': return t('admin.accounts.upstreamBilling.probeInvalidResponse')
    case 'response_too_large': return t('admin.accounts.upstreamBilling.probeResponseTooLarge')
    case 'response_read_failed': return t('admin.accounts.upstreamBilling.probeResponseReadFailed')
    case 'empty_response': return t('admin.accounts.upstreamBilling.probeEmptyResponse')
    case 'missing_api_key': return t('admin.accounts.upstreamBilling.probeMissingApiKey')
    case 'invalid_base_url': return t('admin.accounts.upstreamBilling.probeInvalidBaseUrl')
    case 'proxy_unavailable': return t('admin.accounts.upstreamBilling.probeProxyUnavailable')
    case 'transport_unavailable': return t('admin.accounts.upstreamBilling.probeTransportUnavailable')
    case 'request_build_failed': return t('admin.accounts.upstreamBilling.probeRequestBuildFailed')
    case 'request_failed': return t('admin.accounts.upstreamBilling.probeRequestFailed')
    case 'http_error':
      return httpStatus > 0
        ? t('admin.accounts.upstreamBilling.probeHttpError', { status: httpStatus })
        : t('admin.accounts.upstreamBilling.probeFailedReason')
    default: return t('admin.accounts.upstreamBilling.probeFailedReason')
  }
})
const rateFailureReasonText = computed(() => {
  if (!rateFailed.value) return ''
  if (snapshot.value?.status === 'failed') return probeReasonText.value
  return t('admin.accounts.upstreamBilling.probeFailedReason')
})
const rateFailureGuidanceText = computed(() => (
  hasNumericRate.value
    ? t('admin.accounts.upstreamBilling.manualProbeNotice', { age: elapsedSinceLastSuccess.value })
    : t('admin.accounts.upstreamBilling.failedNoRateNotice')
))
const rateFailureAt = computed(() => (
  snapshot.value?.status === 'failed'
    ? snapshot.value.last_attempt_at
    : props.rateErrorAt ?? undefined
))
const rateValueClass = computed(() => {
  const typography = hasNumericRate.value ? 'font-mono text-[11px] font-semibold' : 'text-[10px] font-medium'
  if (props.probing) return `${typography} text-gray-600 dark:text-gray-300`
  if (rateFailed.value) return `${typography} text-red-600 dark:text-red-400`
  if (cachedRate.value) return `${typography} text-amber-600 dark:text-amber-400`
  if (hasEffectiveRate.value) return 'font-mono text-[11px] font-semibold text-sky-500 dark:text-sky-300'
  if (statusClass.value) return `text-[10px] font-medium ${statusClass.value}`
  return 'text-[10px] font-medium text-gray-600 dark:text-gray-300'
})
const isFiniteNumber = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value)
const quotaNumberFormatter = new Intl.NumberFormat(undefined, { maximumSignificantDigits: 12 })
const quotaMoneyFormatter = new Intl.NumberFormat(undefined, {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
})
const quotaSmallMoneyFormatter = new Intl.NumberFormat(undefined, {
  minimumFractionDigits: 2,
  maximumFractionDigits: 8
})
const quotaUnit = computed(() => {
  const unit = quota.value?.unit
  return unit === 'USD' || unit === 'CNY' || unit === 'TOKENS' ? unit : ''
})
const quotaCurrencyMark = computed(() => (
  quotaUnit.value === 'USD' || quotaUnit.value === 'CNY'
    ? currencySymbol(quotaUnit.value)
    : ''
))
const quotaModeLabel = computed(() => {
  switch (quota.value?.mode) {
    case 'balance': return t('admin.accounts.upstreamBilling.quotaModeBalance')
    case 'quota': return t('admin.accounts.upstreamBilling.quotaModeQuota')
    case 'subscription': return t('admin.accounts.upstreamBilling.quotaModeSubscription')
    case 'rate_limits': return t('admin.accounts.upstreamBilling.quotaModeRateLimits')
    default: return '-'
  }
})
const quotaRemaining = computed(() => {
  const current = quota.value
  if (!current || current.subscription) return null
  const candidates: number[] = []
  if (isFiniteNumber(current.remaining)) candidates.push(current.remaining)
  if (current.unit) {
    for (const window of current.windows ?? []) {
      if (isFiniteNumber(window.remaining)) candidates.push(window.remaining)
    }
  }
  return candidates.length ? Math.min(...candidates) : null
})
const formatQuotaAmount = (value: unknown) => {
  if (!isFiniteNumber(value)) return '-'
  if (quotaCurrencyMark.value) {
    const absolute = Math.abs(value)
    const formatted = (absolute > 0 && absolute < 0.01 ? quotaSmallMoneyFormatter : quotaMoneyFormatter)
      .format(absolute)
    return `${value < 0 ? '-' : ''}${quotaCurrencyMark.value}${formatted}`
  }
  const formatted = quotaNumberFormatter.format(value)
  return quotaUnit.value === 'TOKENS' ? `${formatted} TOKENS` : formatted
}
const quotaPrimaryValue = computed(() => {
  if (quota.value?.subscription) return t('admin.accounts.upstreamBilling.subscriptionDetails')
  if (quotaRemaining.value != null) return formatQuotaAmount(quotaRemaining.value)
  if (quota.value) return quotaModeLabel.value
  if (props.quotaLoading) return t('admin.accounts.upstreamBilling.queryingQuota')
  if (props.quotaError) return t('admin.accounts.upstreamBilling.quotaFailed')
  if (props.quotaResult) return t('admin.accounts.upstreamBilling.noQuotaData')
  return t('admin.accounts.upstreamBilling.notQueried')
})
const quotaValueColorClass = (amount: number | null) => {
  if (props.quotaLoading) return 'text-gray-600 dark:text-gray-300'
  if (props.quotaError) return 'text-red-600 dark:text-red-400'
  if (amount != null) {
    return amount <= 0
      ? 'text-red-600 dark:text-red-400'
      : 'text-emerald-600 dark:text-emerald-400'
  }
  if (quota.value) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-600 dark:text-gray-300'
}
const quotaValueClass = (amount: number | null) => {
  const typography = amount != null ? 'font-mono text-[11px] font-semibold' : 'text-[10px] font-medium'
  return `${typography} ${quotaValueColorClass(amount)}`
}
const actionColorClass = (loading: boolean, feedback: ActionFeedback | null) => {
  if (loading) return 'text-primary-600 dark:text-primary-400'
  if (feedback === 'success') return 'text-emerald-600 dark:text-emerald-400'
  if (feedback === 'error') return 'text-red-600 dark:text-red-400'
  return 'text-gray-400 hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400'
}
const rateActionColorClass = computed(() => actionColorClass(Boolean(props.probing), props.rateFeedback))
const quotaActionColorClass = computed(() => actionColorClass(props.quotaLoading, props.quotaFeedback))
const rateActionIcon = computed(() => {
  if (props.probing) return 'refresh'
  if (props.rateFeedback === 'success') return 'check'
  if (props.rateFeedback === 'error') return 'x'
  return 'refresh'
})
const quotaActionIcon = computed(() => {
  if (props.quotaLoading) return 'refresh'
  if (props.quotaFeedback === 'success') return 'check'
  if (props.quotaFeedback === 'error') return 'x'
  return props.quotaResult ? 'refresh' : 'search'
})
const quotaActionLabel = computed(() => t(
  props.quotaResult
    ? 'admin.accounts.upstreamBilling.refreshQuota'
    : 'admin.accounts.upstreamBilling.queryQuota'
))
const formatDate = (value?: string) => {
  if (!value || !Number.isFinite(Date.parse(value))) return '-'
  return new Date(value).toLocaleString(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}
</script>
