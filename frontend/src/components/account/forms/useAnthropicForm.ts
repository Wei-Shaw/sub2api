import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAccountOAuth, type AddMethod } from '@/composables/useAccountOAuth'
import {
  buildModelMappingObject,
  getPresetMappingsByPlatform
} from '@/composables/useModelWhitelist'
import { applyInterceptWarmup } from '@/components/account/credentialsBuilder'
import { adminAPI } from '@/api/admin'
import type { PlatformFormPayload, PlatformFormValidation, OAuthFlowConfig, EditFormPayload } from './types'
import type { Account, CreateAccountRequest, AccountType } from '@/types'
import * as editH from './editHelpers'

interface ModelMapping { from: string; to: string }

export function useAnthropicForm() {
  const { t } = useI18n()
  const oauth = useAccountOAuth()

  const addMethod = ref<AddMethod>('oauth')
  const apiKeyBaseUrl = ref('https://api.anthropic.com')
  const apiKeyValue = ref('')
  const bedrockAuthMode = ref<'sigv4' | 'apikey'>('sigv4')
  const bedrockAccessKeyId = ref('')
  const bedrockSecretAccessKey = ref('')
  const bedrockSessionToken = ref('')
  const bedrockRegion = ref('us-east-1')
  const bedrockForceGlobal = ref(false)
  const bedrockApiKeyValue = ref('')
  const vertexServiceAccountJson = ref('')
  const vertexProjectId = ref('')
  const vertexClientEmail = ref('')
  const vertexLocation = ref('global')
  const windowCostEnabled = ref(false)
  const windowCostLimit = ref<number | null>(null)
  const windowCostStickyReserve = ref<number | null>(null)
  const sessionLimitEnabled = ref(false)
  const maxSessions = ref<number | null>(null)
  const sessionIdleTimeout = ref<number | null>(null)
  const rpmLimitEnabled = ref(false)
  const baseRpm = ref<number | null>(null)
  const rpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
  const rpmStickyBuffer = ref<number | null>(null)
  const userMsgQueueMode = ref('')
  const tlsFingerprintEnabled = ref(false)
  const tlsFingerprintProfileId = ref<number | null>(null)
  const tlsFingerprintProfiles = ref<{ id: number; name: string }[]>([])
  const sessionIdMaskingEnabled = ref(false)
  const cacheTTLOverrideEnabled = ref(false)
  const cacheTTLOverrideTarget = ref<string>('5m')
  const customBaseUrlEnabled = ref(false)
  const customBaseUrl = ref('')
  const anthropicPassthroughEnabled = ref(false)
  const webSearchEmulationMode = ref('default')
  const webSearchGlobalEnabled = ref(false)
  const interceptWarmupRequests = ref(false)
  const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
  const allowedModels = ref<string[]>([])
  const modelMappings = ref<ModelMapping[]>([])
  const poolModeEnabled = ref(false)
  const poolModeRetryCount = ref(3)
  const customErrorCodesEnabled = ref(false)
  const selectedErrorCodes = ref<number[]>([])
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<{
    error_code: number | null; keywords: string
    duration_minutes: number | null; description: string
  }[]>([])
  const editQuotaLimit = ref<number | null>(null)
  const editQuotaDailyLimit = ref<number | null>(null)
  const editQuotaWeeklyLimit = ref<number | null>(null)
  const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
  const editDailyResetHour = ref<number | null>(null)
  const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
  const editWeeklyResetDay = ref<number | null>(null)
  const editWeeklyResetHour = ref<number | null>(null)
  const editResetTimezone = ref<string | null>(null)
  const presetMappings = computed(() => getPresetMappingsByPlatform('anthropic'))
  const bedrockPresets = computed(() => getPresetMappingsByPlatform('bedrock'))
  const umqModeOptions = computed(() => [
    { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') },
    { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') },
    { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') },
  ])

  const oauthConfig: OAuthFlowConfig = {
    showCookieOption: true,
    showRefreshTokenOption: false,
    platform: 'anthropic'
  }

  function isOAuthFlow(accountCategory: string): boolean {
    return accountCategory === 'oauth-based'
  }

  async function loadTlsProfiles(): Promise<void> {
    try {
      const profiles = await adminAPI.tlsFingerprintProfiles.list()
      tlsFingerprintProfiles.value = profiles.map(p => ({ id: p.id, name: p.name }))
    } catch { tlsFingerprintProfiles.value = [] }
  }

  async function loadWebSearchEnabled(): Promise<void> {
    try {
      const cfg = await adminAPI.settings.getWebSearchEmulationConfig()
      webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
    } catch { webSearchGlobalEnabled.value = false }
  }

  function buildOAuthExtra(baseExtra?: Record<string, unknown>): Record<string, unknown> {
    const extra: Record<string, unknown> = { ...(baseExtra || {}) }
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      extra.window_cost_limit = windowCostLimit.value
      extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    }
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      extra.max_sessions = maxSessions.value
      extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    }
    if (rpmLimitEnabled.value) {
      const DEFAULT_BASE_RPM = 15
      extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0) ? baseRpm.value : DEFAULT_BASE_RPM
      extra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) extra.rpm_sticky_buffer = rpmStickyBuffer.value
    }
    if (userMsgQueueMode.value) extra.user_msg_queue_mode = userMsgQueueMode.value
    if (tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
    }
    if (sessionIdMaskingEnabled.value) extra.session_id_masking_enabled = true
    if (cacheTTLOverrideEnabled.value) {
      extra.cache_ttl_override_enabled = true
      extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    }
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = customBaseUrl.value.trim()
    }
    return extra
  }

  function buildApiKeyExtra(): Record<string, unknown> | undefined {
    const extra: Record<string, unknown> = {}
    if (anthropicPassthroughEnabled.value) extra.anthropic_passthrough = true
    if (webSearchEmulationMode.value !== 'default') extra.web_search_emulation = webSearchEmulationMode.value
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  function buildModelMapping(): Record<string, unknown> | undefined {
    return buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value) ?? undefined
  }

  const MAX_POOL_MODE_RETRY_COUNT = 10
  const DEFAULT_POOL_MODE_RETRY_COUNT = 3

  function normalizePoolRetry(value: number): number {
    if (!Number.isFinite(value)) return DEFAULT_POOL_MODE_RETRY_COUNT
    const n = Math.trunc(value)
    if (n < 0) return 0
    if (n > MAX_POOL_MODE_RETRY_COUNT) return MAX_POOL_MODE_RETRY_COUNT
    return n
  }

  function applySharedCredentials(creds: Record<string, unknown>): void {
    const mapping = buildModelMapping()
    if (mapping) creds.model_mapping = mapping
    if (poolModeEnabled.value) {
      creds.pool_mode = true
      creds.pool_mode_retry_count = normalizePoolRetry(poolModeRetryCount.value)
    }
    if (customErrorCodesEnabled.value) {
      creds.custom_error_codes_enabled = true
      creds.custom_error_codes = [...selectedErrorCodes.value]
    }
    applyInterceptWarmup(creds, interceptWarmupRequests.value, 'create')
  }

  function validate(accountCategory: string): PlatformFormValidation {
    if (accountCategory === 'bedrock') {
      if (bedrockAuthMode.value === 'sigv4') {
        if (!bedrockAccessKeyId.value.trim()) return { valid: false, error: t('admin.accounts.bedrockAccessKeyIdRequired') }
        if (!bedrockSecretAccessKey.value.trim()) return { valid: false, error: t('admin.accounts.bedrockSecretAccessKeyRequired') }
      } else if (!bedrockApiKeyValue.value.trim()) return { valid: false, error: t('admin.accounts.bedrockApiKeyRequired') }
    }
    if (accountCategory === 'service_account') {
      if (!vertexServiceAccountJson.value.trim()) return { valid: false, error: t('admin.accounts.vertexSaJsonMissingFields') }
      if (!vertexLocation.value.trim()) return { valid: false, error: t('admin.accounts.vertexLocationRequired') }
    }
    if (accountCategory === 'apikey' && !apiKeyValue.value.trim()) return { valid: false, error: t('admin.accounts.pleaseEnterApiKey') }
    return { valid: true }
  }

  function getPayload(accountCategory: string): PlatformFormPayload {
    if (accountCategory === 'bedrock') return buildBedrockPayload()
    if (accountCategory === 'service_account') return buildServiceAccountPayload()
    if (accountCategory === 'apikey') return buildApiKeyPayload()
    return { credentials: {}, needsOAuthFlow: true }
  }

  function buildBedrockPayload(): PlatformFormPayload {
    const credentials: Record<string, unknown> = { auth_mode: bedrockAuthMode.value, aws_region: bedrockRegion.value.trim() || 'us-east-1' }
    if (bedrockAuthMode.value === 'sigv4') {
      credentials.aws_access_key_id = bedrockAccessKeyId.value.trim()
      credentials.aws_secret_access_key = bedrockSecretAccessKey.value.trim()
      if (bedrockSessionToken.value.trim()) credentials.aws_session_token = bedrockSessionToken.value.trim()
    } else { credentials.api_key = bedrockApiKeyValue.value.trim() }
    if (bedrockForceGlobal.value) credentials.aws_force_global = 'true'
    applySharedCredentials(credentials)
    return { credentials, typeOverride: 'bedrock' as AccountType }
  }

  function buildServiceAccountPayload(): PlatformFormPayload {
    return { credentials: { service_account_json: vertexServiceAccountJson.value.trim(), project_id: vertexProjectId.value.trim(), client_email: vertexClientEmail.value.trim(), location: vertexLocation.value.trim(), tier_id: 'vertex' }, typeOverride: 'service_account' as AccountType }
  }

  function buildApiKeyPayload(): PlatformFormPayload {
    const credentials: Record<string, unknown> = { base_url: apiKeyBaseUrl.value.trim() || 'https://api.anthropic.com', api_key: apiKeyValue.value.trim() }
    applySharedCredentials(credentials)
    return { credentials, extra: buildApiKeyExtra() }
  }

  async function handleOAuthExchange(code: string, _oauthState?: string): Promise<CreateAccountRequest | null> {
    if (!code.trim() || !oauth.sessionId.value) return null
    const endpoint = addMethod.value === 'oauth' ? '/admin/accounts/exchange-code' : '/admin/accounts/exchange-setup-token-code'
    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, { session_id: oauth.sessionId.value, code: code.trim() })
    const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
    const extra = buildOAuthExtra(baseExtra)
    const credentials: Record<string, unknown> = { ...tokenInfo }
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
    return { name: '', platform: 'anthropic', type: addMethod.value as AccountType, credentials, extra: Object.keys(extra).length > 0 ? extra : undefined }
  }

  async function handleCookieAuth(sessionKey: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null> {
    const keys = oauth.parseSessionKeys(sessionKey)
    if (keys.length === 0) return null
    const endpoint = addMethod.value === 'oauth' ? '/admin/accounts/cookie-auth' : '/admin/accounts/setup-token-cookie-auth'
    const results: CreateAccountRequest[] = []
    for (const key of keys) {
      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, { session_id: '', code: key })
      const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
      const extra = buildOAuthExtra(baseExtra)
      const credentials: Record<string, unknown> = { ...tokenInfo }
      applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
      results.push({ name: '', platform: 'anthropic', type: addMethod.value as AccountType, credentials, extra: Object.keys(extra).length > 0 ? extra : undefined })
    }
    return results.length === 1 ? results[0] : results.length > 0 ? results : null
  }

  function reset() {
    addMethod.value = 'oauth'
    apiKeyBaseUrl.value = 'https://api.anthropic.com'
    apiKeyValue.value = ''
    bedrockAuthMode.value = 'sigv4'
    bedrockAccessKeyId.value = ''
    bedrockSecretAccessKey.value = ''
    bedrockSessionToken.value = ''
    bedrockRegion.value = 'us-east-1'
    bedrockForceGlobal.value = false
    bedrockApiKeyValue.value = ''
    vertexServiceAccountJson.value = ''
    vertexProjectId.value = ''
    vertexClientEmail.value = ''
    vertexLocation.value = 'global'
    windowCostEnabled.value = false
    windowCostLimit.value = null
    windowCostStickyReserve.value = null
    sessionLimitEnabled.value = false
    maxSessions.value = null
    sessionIdleTimeout.value = null
    rpmLimitEnabled.value = false
    baseRpm.value = null
    rpmStrategy.value = 'tiered'
    rpmStickyBuffer.value = null
    userMsgQueueMode.value = ''
    tlsFingerprintEnabled.value = false
    tlsFingerprintProfileId.value = null
    sessionIdMaskingEnabled.value = false
    cacheTTLOverrideEnabled.value = false
    cacheTTLOverrideTarget.value = '5m'
    customBaseUrlEnabled.value = false
    customBaseUrl.value = ''
    anthropicPassthroughEnabled.value = false
    webSearchEmulationMode.value = 'default'
    interceptWarmupRequests.value = false
    modelRestrictionMode.value = 'whitelist'
    allowedModels.value = []
    modelMappings.value = []
    poolModeEnabled.value = false
    poolModeRetryCount.value = 3
    customErrorCodesEnabled.value = false
    selectedErrorCodes.value = []
    tempUnschedEnabled.value = false
    tempUnschedRules.value = []
    editQuotaLimit.value = null
    editQuotaDailyLimit.value = null
    editQuotaWeeklyLimit.value = null
    editDailyResetMode.value = null
    editDailyResetHour.value = null
    editWeeklyResetMode.value = null
    editWeeklyResetDay.value = null
    editWeeklyResetHour.value = null
    editResetTimezone.value = null
    oauth.resetState()
  }

  // ---- Edit mode ----

  function initFromAccount(account: Account): void {
    reset()
    const credentials = account.credentials as Record<string, unknown> | undefined
    const extra = account.extra as Record<string, unknown> | undefined

    editH.loadTempUnschedFromCredentials(credentials, tempUnschedEnabled, tempUnschedRules)
    editH.loadInterceptWarmupFromCredentials(credentials, interceptWarmupRequests)

    // Quota
    editH.loadQuotaFromExtra(extra, {
      quotaLimit: editQuotaLimit,
      quotaDailyLimit: editQuotaDailyLimit,
      quotaWeeklyLimit: editQuotaWeeklyLimit,
      dailyResetMode: editDailyResetMode,
      dailyResetHour: editDailyResetHour,
      weeklyResetMode: editWeeklyResetMode,
      weeklyResetDay: editWeeklyResetDay,
      weeklyResetHour: editWeeklyResetHour,
      resetTimezone: editResetTimezone
    })

    if (account.type === 'apikey') {
      apiKeyBaseUrl.value = (credentials?.base_url as string) || 'https://api.anthropic.com'
      anthropicPassthroughEnabled.value = extra?.anthropic_passthrough === true
      webSearchEmulationMode.value = (extra?.web_search_emulation as string) || 'default'
      editH.loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
      editH.loadPoolModeFromCredentials(credentials, poolModeEnabled, poolModeRetryCount)
      editH.loadCustomErrorCodesFromCredentials(credentials, customErrorCodesEnabled, selectedErrorCodes)
    } else if (account.type === 'bedrock') {
      bedrockAuthMode.value = (credentials?.auth_mode as 'sigv4' | 'apikey') || 'sigv4'
      bedrockRegion.value = (credentials?.aws_region as string) || 'us-east-1'
      bedrockForceGlobal.value = credentials?.aws_force_global === 'true'
      editH.loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
      editH.loadPoolModeFromCredentials(credentials, poolModeEnabled, poolModeRetryCount)
      editH.loadCustomErrorCodesFromCredentials(credentials, customErrorCodesEnabled, selectedErrorCodes)
    } else if (account.type === 'oauth' || account.type === 'setup-token') {
      // OAuth extra
      windowCostEnabled.value = (extra?.window_cost_limit as number) > 0
      windowCostLimit.value = (extra?.window_cost_limit as number) || null
      windowCostStickyReserve.value = (extra?.window_cost_sticky_reserve as number) ?? null
      sessionLimitEnabled.value = (extra?.max_sessions as number) > 0
      maxSessions.value = (extra?.max_sessions as number) || null
      sessionIdleTimeout.value = (extra?.session_idle_timeout_minutes as number) ?? null
      rpmLimitEnabled.value = (extra?.base_rpm as number) > 0
      baseRpm.value = (extra?.base_rpm as number) || null
      rpmStrategy.value = (extra?.rpm_strategy as 'tiered' | 'sticky_exempt') || 'tiered'
      rpmStickyBuffer.value = (extra?.rpm_sticky_buffer as number) ?? null
      userMsgQueueMode.value = (extra?.user_msg_queue_mode as string) || ''
      tlsFingerprintEnabled.value = extra?.enable_tls_fingerprint === true
      tlsFingerprintProfileId.value = (extra?.tls_fingerprint_profile_id as number) ?? null
      sessionIdMaskingEnabled.value = extra?.session_id_masking_enabled === true
      cacheTTLOverrideEnabled.value = extra?.cache_ttl_override_enabled === true
      cacheTTLOverrideTarget.value = (extra?.cache_ttl_override_target as string) || '5m'
      customBaseUrlEnabled.value = extra?.custom_base_url_enabled === true
      customBaseUrl.value = (extra?.custom_base_url as string) || ''
    }
  }

  function getEditPayload(account: Account): EditFormPayload {
    const currentCreds = (account.credentials as Record<string, unknown>) || {}
    const newCreds: Record<string, unknown> = { ...currentCreds }
    const currentExtra = (account.extra as Record<string, unknown>) || {}
    const newExtra: Record<string, unknown> = { ...currentExtra }

    editH.applyTempUnschedToCredentials(newCreds, tempUnschedEnabled.value, tempUnschedRules.value)
    editH.applyInterceptWarmup(newCreds, interceptWarmupRequests.value, 'edit')

    // Quota
    editH.applyQuotaToExtra(newExtra, {
      quotaLimit: editQuotaLimit.value,
      quotaDailyLimit: editQuotaDailyLimit.value,
      quotaWeeklyLimit: editQuotaWeeklyLimit.value,
      dailyResetMode: editDailyResetMode.value,
      dailyResetHour: editDailyResetHour.value,
      weeklyResetMode: editWeeklyResetMode.value,
      weeklyResetDay: editWeeklyResetDay.value,
      weeklyResetHour: editWeeklyResetHour.value,
      resetTimezone: editResetTimezone.value
    })

    if (account.type === 'apikey') {
      newCreds.base_url = apiKeyBaseUrl.value.trim() || 'https://api.anthropic.com'
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      editH.applyPoolModeToCredentials(newCreds, poolModeEnabled.value, poolModeRetryCount.value)
      editH.applyCustomErrorCodesToCredentials(newCreds, customErrorCodesEnabled.value, selectedErrorCodes.value)
      if (anthropicPassthroughEnabled.value) newExtra.anthropic_passthrough = true
      else delete newExtra.anthropic_passthrough
      if (webSearchEmulationMode.value !== 'default') newExtra.web_search_emulation = webSearchEmulationMode.value
      else delete newExtra.web_search_emulation
    } else if (account.type === 'bedrock') {
      newCreds.aws_region = bedrockRegion.value.trim() || 'us-east-1'
      if (bedrockForceGlobal.value) newCreds.aws_force_global = 'true'
      else delete newCreds.aws_force_global
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      editH.applyPoolModeToCredentials(newCreds, poolModeEnabled.value, poolModeRetryCount.value)
      editH.applyCustomErrorCodesToCredentials(newCreds, customErrorCodesEnabled.value, selectedErrorCodes.value)
    } else if (account.type === 'oauth' || account.type === 'setup-token') {
      // OAuth extra fields
      if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
        newExtra.window_cost_limit = windowCostLimit.value
        newExtra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
      } else {
        delete newExtra.window_cost_limit
        delete newExtra.window_cost_sticky_reserve
      }
      if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
        newExtra.max_sessions = maxSessions.value
        newExtra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
      } else {
        delete newExtra.max_sessions
        delete newExtra.session_idle_timeout_minutes
      }
      if (rpmLimitEnabled.value) {
        newExtra.base_rpm = baseRpm.value ?? 15
        newExtra.rpm_strategy = rpmStrategy.value
        if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) newExtra.rpm_sticky_buffer = rpmStickyBuffer.value
        else delete newExtra.rpm_sticky_buffer
      } else {
        delete newExtra.base_rpm
        delete newExtra.rpm_strategy
        delete newExtra.rpm_sticky_buffer
      }
      if (userMsgQueueMode.value) newExtra.user_msg_queue_mode = userMsgQueueMode.value
      else delete newExtra.user_msg_queue_mode
      if (tlsFingerprintEnabled.value) {
        newExtra.enable_tls_fingerprint = true
        if (tlsFingerprintProfileId.value) newExtra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
        else delete newExtra.tls_fingerprint_profile_id
      } else {
        delete newExtra.enable_tls_fingerprint
        delete newExtra.tls_fingerprint_profile_id
      }
      if (sessionIdMaskingEnabled.value) newExtra.session_id_masking_enabled = true
      else delete newExtra.session_id_masking_enabled
      if (cacheTTLOverrideEnabled.value) {
        newExtra.cache_ttl_override_enabled = true
        newExtra.cache_ttl_override_target = cacheTTLOverrideTarget.value
      } else {
        delete newExtra.cache_ttl_override_enabled
        delete newExtra.cache_ttl_override_target
      }
      if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
        newExtra.custom_base_url_enabled = true
        newExtra.custom_base_url = customBaseUrl.value.trim()
      } else {
        delete newExtra.custom_base_url_enabled
        delete newExtra.custom_base_url
      }
    }

    return { credentials: newCreds, extra: newExtra }
  }

  return {
    addMethod, apiKeyBaseUrl, apiKeyValue,
    bedrockAuthMode, bedrockAccessKeyId, bedrockSecretAccessKey,
    bedrockSessionToken, bedrockRegion, bedrockForceGlobal, bedrockApiKeyValue,
    vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
    windowCostEnabled, windowCostLimit, windowCostStickyReserve,
    sessionLimitEnabled, maxSessions, sessionIdleTimeout,
    rpmLimitEnabled, baseRpm, rpmStrategy, rpmStickyBuffer,
    userMsgQueueMode, umqModeOptions,
    tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
    sessionIdMaskingEnabled,
    cacheTTLOverrideEnabled, cacheTTLOverrideTarget,
    customBaseUrlEnabled, customBaseUrl,
    anthropicPassthroughEnabled, webSearchEmulationMode, webSearchGlobalEnabled,
    interceptWarmupRequests,
    modelRestrictionMode, allowedModels, modelMappings,
    poolModeEnabled, poolModeRetryCount,
    customErrorCodesEnabled, selectedErrorCodes,
    tempUnschedEnabled, tempUnschedRules,
    editQuotaLimit, editQuotaDailyLimit, editQuotaWeeklyLimit,
    editDailyResetMode, editDailyResetHour,
    editWeeklyResetMode, editWeeklyResetDay, editWeeklyResetHour,
    editResetTimezone,
    presetMappings, bedrockPresets,
    oauth, oauthConfig,
    isOAuthFlow, loadTlsProfiles, loadWebSearchEnabled,
    validate, getPayload, reset,
    handleOAuthExchange, handleCookieAuth,
    initFromAccount, getEditPayload
  }
}
