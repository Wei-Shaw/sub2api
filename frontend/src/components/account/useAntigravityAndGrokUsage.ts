import { computed, ref, type Ref } from 'vue'
import type { Account, AccountUsageInfo, WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import { antigravityPlan } from '@/utils/antigravityPlan'

export interface GrokQuotaBarInfo {
  utilization: number
  resetsAt: string | null
  windowStats?: WindowStats | null
}

export function useAntigravityAndGrokUsage(
  account: Ref<Account>,
  usageInfo: Ref<AccountUsageInfo | null>,
  t: (key: string, named?: Record<string, unknown>) => string
) {
  const aiCreditsDisplay = computed(() => {
    const credits = usageInfo.value?.ai_credits
    if (!credits || credits.length === 0) return null
    const total = credits.reduce((sum, credit) => sum + (credit.amount ?? 0), 0)
    if (total <= 0) return null
    return total.toFixed(0)
  })

  const antigravityTier = computed(() => {
    const reported = usageInfo.value?.subscription_tier_raw
    if (reported) return reported
    const extra = account.value.extra as Record<string, unknown> | undefined
    const loadCodeAssist = extra?.load_code_assist as Record<string, unknown> | undefined
    const paidTier = loadCodeAssist?.paidTier as Record<string, unknown> | undefined
    if (paidTier && typeof paidTier.id === 'string') return paidTier.id
    const currentTier = loadCodeAssist?.currentTier as Record<string, unknown> | undefined
    if (currentTier && typeof currentTier.id === 'string') return currentTier.id
    return account.value.credentials?.plan_type ?? null
  })

  const antigravityTierLabel = computed(() => antigravityPlan(antigravityTier.value)?.label ?? null)
  const antigravityTierClass = computed(() => antigravityPlan(antigravityTier.value)?.className ?? '')

  const hasIneligibleTiers = computed(() => {
    const extra = account.value.extra as Record<string, unknown> | undefined
    const loadCodeAssist = extra?.load_code_assist as Record<string, unknown> | undefined
    const ineligibleTiers = loadCodeAssist?.ineligibleTiers as unknown[] | undefined
    return Array.isArray(ineligibleTiers) && ineligibleTiers.length > 0
  })

  const isForbidden = computed(() => !!usageInfo.value?.is_forbidden)
  const forbiddenType = computed(() => usageInfo.value?.forbidden_type || 'forbidden')
  const validationURL = computed(() => {
    const raw = usageInfo.value?.validation_url
    if (!raw) return ''
    try {
      const url = new URL(raw)
      return url.protocol === 'https:' && !url.username && !url.password ? url.href : ''
    } catch {
      return ''
    }
  })
  const needsReauth = computed(() => !!usageInfo.value?.needs_reauth)

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
      setTimeout(() => {
        linkCopied.value = false
      }, 2000)
    } catch {
      // ignore
    }
  }

  // Grok helpers
  const grokBilling = computed(() => usageInfo.value?.grok_billing || null)
  const grokLocalUsage7d = computed(
    () => usageInfo.value?.grok_local_usage_7d || usageInfo.value?.seven_day?.window_stats || null
  )
  const grokLocalUsageMonthly = computed(
    () =>
      usageInfo.value?.grok_local_usage_monthly || usageInfo.value?.thirty_day?.window_stats || null
  )
  const grokWeeklyBillingBar = computed((): GrokQuotaBarInfo | null => {
    const billing = grokBilling.value
    if (billing?.period_type?.toLowerCase() !== 'weekly' || billing.usage_percent == null) {
      return null
    }
    return {
      utilization: Math.min(100, Math.max(0, billing.usage_percent)),
      resetsAt: billing.period_end || null,
      windowStats: grokLocalUsage7d.value
    }
  })

  const grokMonthlyBillingBar = computed((): GrokQuotaBarInfo | null => {
    const billing = grokBilling.value
    if (!billing) return null
    let utilization: number | null = null
    if (billing.used_percent != null && Number.isFinite(billing.used_percent)) {
      utilization = billing.used_percent
    } else if (
      billing.monthly_limit_cents != null &&
      billing.monthly_limit_cents > 0 &&
      billing.used_cents != null
    ) {
      utilization = (billing.used_cents / billing.monthly_limit_cents) * 100
    }
    if (utilization == null) return null
    if (billing.period_type?.toLowerCase() === 'weekly' && billing.monthly_limit_cents == null) {
      return null
    }
    return {
      utilization: Math.min(100, Math.max(0, utilization)),
      resetsAt: billing.billing_period_end || billing.period_end || null,
      windowStats: grokLocalUsageMonthly.value
    }
  })

  const formatGrokMoney = (value?: number | null) => {
    if (value == null || Number.isNaN(value)) return '0'
    if (value >= 1000) return formatCompactNumber(value)
    if (value >= 100) return value.toFixed(0)
    if (value >= 10) return value.toFixed(1)
    return value.toFixed(2)
  }

  const grokPrepaidMoneyLine = computed(() => {
    const billing = grokBilling.value
    if (!billing) return null
    const prepaid = billing.prepaid_balance
    const showPrepaid = prepaid != null && Number.isFinite(prepaid) && prepaid > 0
    const limitRaw =
      billing.monthly_limit != null
        ? billing.monthly_limit
        : billing.monthly_limit_cents != null
          ? billing.monthly_limit_cents / 100
          : null
    const showUsedLimit = limitRaw != null && Number.isFinite(limitRaw) && limitRaw > 0
    if (!showPrepaid && !showUsedLimit) return null
    const used =
      billing.monthly_used != null
        ? billing.monthly_used
        : billing.used_cents != null
          ? billing.used_cents / 100
          : 0
    return {
      showPrepaid,
      showUsedLimit,
      prepaid: showPrepaid ? formatGrokMoney(prepaid) : null,
      used: showUsedLimit ? formatGrokMoney(used) : null,
      limit: showUsedLimit ? formatGrokMoney(limitRaw) : null
    }
  })

  const grokPlanLabelIsFree = (value: string) => value.includes('free') || value.includes('basic')
  const grokPlanLabelIsPaid = (value: string) =>
    value !== '' && !grokPlanLabelIsFree(value) && !value.includes('unknown')

  const grokIsFree = computed(() => {
    if (account.value.platform !== 'grok' || account.value.type !== 'oauth') return false
    const billing = grokBilling.value
    const plan = (billing?.plan || '').trim().toLowerCase()
    const tier = (usageInfo.value?.subscription_tier || '').trim().toLowerCase()
    const entitlement = (usageInfo.value?.grok_entitlement_status || '').toLowerCase()
    if (grokPlanLabelIsFree(tier)) return true
    if (grokPlanLabelIsPaid(tier)) return false
    if (
      billing?.usage_percent != null ||
      billing?.used_percent != null ||
      (billing?.monthly_limit_cents != null && billing.monthly_limit_cents > 0)
    ) {
      return false
    }
    if (grokPlanLabelIsPaid(plan)) return false
    if (grokPlanLabelIsFree(plan) || grokPlanLabelIsFree(entitlement)) return true
    return billing != null
  })

  const grokFreeQuotaUsage = computed(() => usageInfo.value?.grok_local_usage_24h || null)
  const grokFreeTokenBar = computed(() => {
    if (!grokIsFree.value || !grokFreeQuotaUsage.value) return null
    const limit = usageInfo.value?.grok_free_token_limit
    if (typeof limit !== 'number' || limit <= 0) return null
    const used = Math.max(0, grokFreeQuotaUsage.value.tokens || 0)
    return { utilization: Math.min(100, (used / limit) * 100), limit }
  })

  const grokQuotaUnknown = computed(() => {
    if (account.value.platform !== 'grok') return false
    if (grokIsFree.value) return !grokFreeTokenBar.value
    if (grokWeeklyBillingBar.value || grokMonthlyBillingBar.value || grokPrepaidMoneyLine.value) {
      return false
    }
    return usageInfo.value?.grok_quota_snapshot_state !== 'observed'
  })

  const grokQuotaUnknownLabel = computed(() => {
    return usageInfo.value?.grok_quota_snapshot_state === 'no_headers'
      ? t('admin.accounts.usageWindow.grokNoHeaders')
      : t('admin.accounts.usageWindow.grokUnknown')
  })

  const grokRetryAfterLabel = computed(() => {
    const seconds = usageInfo.value?.grok_retry_after_seconds
    if (seconds == null || seconds <= 0) return null
    if (seconds < 60) return `${seconds}s`
    const minutes = Math.ceil(seconds / 60)
    return `${minutes}m`
  })

  return {
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
  }
}
