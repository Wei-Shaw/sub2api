import { computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'

/** Minimal Gemini credentials shape for tier detection. */
interface GeminiCredentials {
  oauth_type?: string
  tier_id?: string
  project_id?: string
}

/** Minimal account shape for Gemini tier display. */
interface GeminiTierAccount {
  type: string
  credentials?: Record<string, unknown>
}

/**
 * Derives Gemini channel, user level, auth type label, tier class,
 * and quota-policy tooltip strings from an Account ref.
 */
export function useGeminiTier(account: Ref<GeminiTierAccount>) {
  const { t } = useI18n()

  const creds = computed(() => account.value.credentials as GeminiCredentials | undefined)
  const tierValue = computed(() => creds.value?.tier_id || null)
  const oauthType = computed(() => (creds.value?.oauth_type || '').trim() || null)

  const isCodeAssist = computed(() => {
    return creds.value?.oauth_type === 'code_assist' || (!creds.value?.oauth_type && !!creds.value?.project_id)
  })

  const channelShort = computed((): 'ai studio' | 'gcp' | 'google one' | 'client' | null => {
    if (account.value.type === 'apikey') return 'ai studio'
    if (oauthType.value === 'google_one') return 'google one'
    if (isCodeAssist.value) return 'gcp'
    if (oauthType.value === 'ai_studio') return 'client'
    return 'ai studio'
  })

  const userLevel = computed((): string | null => {
    const tier = (tierValue.value || '').toString().trim()
    const lo = tier.toLowerCase()
    const up = tier.toUpperCase()

    if (oauthType.value === 'google_one') {
      if (lo === 'google_one_free') return 'free'
      if (lo === 'google_ai_pro') return 'pro'
      if (lo === 'google_ai_ultra') return 'ultra'
      if (up === 'AI_PREMIUM' || up === 'GOOGLE_ONE_STANDARD') return 'pro'
      if (up === 'GOOGLE_ONE_UNLIMITED') return 'ultra'
      if (up === 'FREE' || up === 'GOOGLE_ONE_BASIC' || up === 'GOOGLE_ONE_UNKNOWN' || up === '') return 'free'
      return null
    }

    if (isCodeAssist.value) {
      if (lo === 'gcp_enterprise') return 'enterprise'
      if (lo === 'gcp_standard') return 'standard'
      if (up.includes('ULTRA') || up.includes('ENTERPRISE')) return 'enterprise'
      return 'standard'
    }

    if (account.value.type === 'apikey' || oauthType.value === 'ai_studio') {
      if (lo === 'aistudio_paid') return 'paid'
      if (lo === 'aistudio_free') return 'free'
      if (up.includes('PAID') || up.includes('PAYG') || up.includes('PAY')) return 'paid'
      if (up.includes('FREE')) return 'free'
      if (account.value.type === 'apikey') return 'free'
      return null
    }

    return null
  })

  const authTypeLabel = computed(() => {
    if (!channelShort.value) return null
    return userLevel.value ? `${channelShort.value} ${userLevel.value}` : channelShort.value
  })

  const tierClass = computed(() => {
    const ch = channelShort.value
    const lv = userLevel.value
    if (ch === 'client' || ch === 'ai studio') {
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    }
    if (ch === 'google one') {
      if (lv === 'ultra') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
      if (lv === 'pro') return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
      return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
    }
    if (ch === 'gcp') {
      if (lv === 'enterprise') return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
      return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
    }
    return ''
  })

  // ===== Quota policy tooltip =====

  const quotaPolicyChannel = computed(() => {
    if (oauthType.value === 'google_one') return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.channel')
    if (isCodeAssist.value) return t('admin.accounts.gemini.quotaPolicy.rows.gcp.channel')
    return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel')
  })

  const quotaPolicyLimits = computed(() => {
    const lo = (tierValue.value || '').toString().trim().toLowerCase()
    if (oauthType.value === 'google_one') {
      if (lo === 'google_ai_ultra' || userLevel.value === 'ultra') return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsUltra')
      if (lo === 'google_ai_pro' || userLevel.value === 'pro') return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsPro')
      return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsFree')
    }
    if (isCodeAssist.value) {
      if (lo === 'gcp_enterprise' || userLevel.value === 'enterprise') return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsEnterprise')
      return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsStandard')
    }
    if (lo === 'aistudio_paid' || userLevel.value === 'paid') return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsPaid')
    return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree')
  })

  const quotaPolicyDocsUrl = computed(() => {
    if (oauthType.value === 'google_one' || isCodeAssist.value) {
      return 'https://developers.google.com/gemini-code-assist/resources/quotas'
    }
    return 'https://ai.google.dev/pricing'
  })

  return {
    oauthType,
    isCodeAssist,
    channelShort,
    userLevel,
    authTypeLabel,
    tierClass,
    quotaPolicyChannel,
    quotaPolicyLimits,
    quotaPolicyDocsUrl,
  }
}
