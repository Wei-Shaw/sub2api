import { ref, computed, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type PlatformFormPayload,
  type PlatformFormValidation,
  type OAuthFlowConfig,
  type EditFormPayload,
  type ModelMapping,
  type SdkAccount,
  type SdkCreateAccountRequest,
  type TempUnschedRuleForm,
  type CommonAccountFields,
  loadModelMappingFromCredentials,
  applyModelMappingToCredentials,
  loadPoolModeFromCredentials,
  applyPoolModeToCredentials,
  loadCustomErrorCodesFromCredentials,
  applyCustomErrorCodesToCredentials,
  loadTempUnschedFromCredentials,
  applyTempUnschedToCredentials,
} from '@sub2api/plugin-sdk'
import { useGeminiOAuth } from '../composables/useGeminiOAuth'
import { buildModelMappingObject } from './geminiModels'
import { resolveAllProtocolModelIds } from '@sub2api/plugin-sdk'

/** Default Vertex AI location, matching backend vertexDefaultLocation. */
const vertexDefaultLocation = 'us-central1'

export function useGeminiForm(commonFields: Ref<CommonAccountFields>) {
  const { t } = useI18n()
  const geminiOAuth = useGeminiOAuth()

  // ---- Reactive state ----
  const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')
  const geminiAIStudioOAuthEnabled = ref(false)
  const showAdvancedOAuth = ref(false)
  const showGeminiHelpDialog = ref(false)
  const apiKeyBaseUrl = ref('https://generativelanguage.googleapis.com')
  const apiKeyValue = ref('')
  const editApiKey = ref('')
  const geminiTierGoogleOne = ref<'google_one_free' | 'google_ai_pro' | 'google_ai_ultra'>('google_one_free')
  const geminiTierGcp = ref<'gcp_standard' | 'gcp_enterprise'>('gcp_standard')
  const geminiTierAIStudio = ref<'aistudio_free' | 'aistudio_paid'>('aistudio_free')
  const vertexServiceAccountJson = ref('')
  const vertexProjectId = ref('')
  const vertexClientEmail = ref('')
  const vertexLocation = ref(vertexDefaultLocation)
  const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
  const allowedModels = ref<string[]>([])
  const modelMappings = ref<ModelMapping[]>([])
  const poolModeEnabled = ref(false)
  const poolModeRetryCount = ref(3)
  const customErrorCodesEnabled = ref(false)
  const selectedErrorCodes = ref<number[]>([])
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
  const isEditMode = ref(false)

  // ---- Computed ----
  const geminiSelectedTier = computed(() => {
    switch (geminiOAuthType.value) {
      case 'google_one': return geminiTierGoogleOne.value
      case 'code_assist': return geminiTierGcp.value
      default: return geminiTierAIStudio.value
    }
  })

  watch(modelRestrictionMode, (newMode) => {
    if (newMode === 'whitelist') {
      allowedModels.value = resolveAllProtocolModelIds(['gemini'])
    }
  }, { immediate: true })

  const oauthConfig: OAuthFlowConfig = {
    showProjectId: true, platform: 'gemini',
    showStateWarning: true,
    i18nPrefix: 'admin.accounts.oauth.gemini',
  }
  const geminiHelpLinks = {
    apiKey: 'https://aistudio.google.com/app/apikey',
    aiStudioPricing: 'https://ai.google.dev/pricing',
    gcpProject: 'https://console.cloud.google.com/welcome/new'
  }

  // ---- Actions ----
  function selectOAuthType(type: 'code_assist' | 'google_one' | 'ai_studio') {
    if (type === 'ai_studio' && !geminiAIStudioOAuthEnabled.value) return
    geminiOAuthType.value = type
  }

  async function checkAIStudioCapability() {
    const caps = await geminiOAuth.getCapabilities()
    geminiAIStudioOAuthEnabled.value = !!caps?.ai_studio_oauth_enabled
    if (!geminiAIStudioOAuthEnabled.value && geminiOAuthType.value === 'ai_studio') {
      geminiOAuthType.value = 'code_assist'
    }
  }

  function validate(accountCategory: string, mode?: string): PlatformFormValidation {
    if (accountCategory === 'apikey' && mode !== 'edit' && !apiKeyValue.value.trim()) {
      return { valid: false, error: t('admin.accounts.pleaseEnterApiKey') }
    }
    if (accountCategory === 'service_account') {
      if (!isEditMode.value && !vertexServiceAccountJson.value.trim()) {
        return { valid: false, error: t('admin.accounts.vertexSaJsonMissingFields') }
      }
      if (!vertexLocation.value.trim()) {
        return { valid: false, error: t('admin.accounts.vertexLocationRequired') }
      }
    }
    return { valid: true }
  }

  function getPayload(accountCategory: string): PlatformFormPayload {
    if (accountCategory === 'service_account') {
      return { ...buildServiceAccountPayload(), common: commonFields.value }
    }
    if (accountCategory === 'oauth-based') {
      return { credentials: {}, common: commonFields.value, needsOAuthFlow: true }
    }
    return { ...buildApiKeyPayload(), common: commonFields.value }
  }

  function buildServiceAccountPayload(): PlatformFormPayload {
    const credentials: Record<string, unknown> = {
      service_account_json: vertexServiceAccountJson.value.trim(),
      project_id: vertexProjectId.value.trim(),
      client_email: vertexClientEmail.value.trim(),
      location: vertexLocation.value.trim(),
      tier_id: 'vertex'
    }
    const mm = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (mm) credentials.model_mapping = mm
    applyTempUnschedToCredentials(credentials, tempUnschedEnabled.value, tempUnschedRules.value)
    return { credentials, typeOverride: 'service_account' }
  }

  function buildApiKeyPayload(): PlatformFormPayload {
    const credentials: Record<string, unknown> = {
      base_url: apiKeyBaseUrl.value.trim() || 'https://generativelanguage.googleapis.com',
      api_key: apiKeyValue.value.trim(),
      tier_id: geminiTierAIStudio.value
    }
    const mm = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (mm) credentials.model_mapping = mm
    if (poolModeEnabled.value) {
      credentials.pool_mode = true
      credentials.pool_mode_retry_count = poolModeRetryCount.value
    }
    if (customErrorCodesEnabled.value) {
      credentials.custom_error_codes_enabled = true
      credentials.custom_error_codes = [...selectedErrorCodes.value]
    }
    applyTempUnschedToCredentials(credentials, tempUnschedEnabled.value, tempUnschedRules.value)
    return { credentials }
  }

  async function handleOAuthExchange(
    code: string, oauthState?: string
  ): Promise<SdkCreateAccountRequest | null> {
    if (!code.trim() || !geminiOAuth.sessionId.value) return null
    const stateToUse = oauthState || geminiOAuth.state.value
    if (!stateToUse) return null
    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: code.trim(),
      sessionId: geminiOAuth.sessionId.value,
      state: stateToUse,
      proxyId: null,
      oauthType: geminiOAuthType.value,
      tierId: geminiSelectedTier.value
    })
    if (!tokenInfo) return null
    const creds = geminiOAuth.buildCredentials(tokenInfo)
    const extra = geminiOAuth.buildExtraInfo(tokenInfo)
    applyTempUnschedToCredentials(creds, tempUnschedEnabled.value, tempUnschedRules.value)
    return { name: '', platform: 'gemini', type: 'oauth', credentials: creds, extra }
  }

  // ---- Edit mode ----
  function initFromAccount(account: SdkAccount): void {
    reset()
    isEditMode.value = true
    const credentials = account.credentials
    loadTempUnschedFromCredentials(credentials, tempUnschedEnabled, tempUnschedRules)
    if (account.type === 'service_account') {
      initServiceAccountEdit(credentials)
    } else if (account.type === 'apikey') {
      initApiKeyEdit(credentials)
    } else if (account.type === 'oauth') {
      initOAuthEdit(credentials)
    }
    const a = account as Record<string, unknown>
    commonFields.value = {
      name: (a.name as string) ?? '',
      notes: (a.notes as string) ?? '',
      proxy_id: (a.proxy_id as number) ?? null,
      concurrency: (a.concurrency as number) ?? 10,
      load_factor: (a.load_factor as number) ?? null,
      priority: (a.priority as number) ?? 1,
      rate_multiplier: (a.rate_multiplier as number) ?? 1,
      expires_at: (a.expires_at as number) ?? null,
      auto_pause_on_expired: (a.auto_pause_on_expired as boolean) ?? true,
      group_ids: (a.group_ids as number[]) ?? [],
      quota_enabled: !!((a.extra as Record<string, unknown>)?.quota_limit || (a.extra as Record<string, unknown>)?.quota_daily_limit || (a.extra as Record<string, unknown>)?.quota_weekly_limit),
      quota_limit: ((a.extra as Record<string, unknown>)?.quota_limit as number) ?? null,
      quota_daily_limit: ((a.extra as Record<string, unknown>)?.quota_daily_limit as number) ?? null,
      quota_weekly_limit: ((a.extra as Record<string, unknown>)?.quota_weekly_limit as number) ?? null,
    }
  }

  function initServiceAccountEdit(credentials: Record<string, unknown> | undefined): void {
    vertexProjectId.value = (credentials?.project_id as string) || ''
    vertexClientEmail.value = (credentials?.client_email as string) || ''
    vertexLocation.value = (credentials?.location as string) || (credentials?.vertex_location as string) || vertexDefaultLocation
    loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
  }

  function initApiKeyEdit(credentials: Record<string, unknown> | undefined): void {
    apiKeyBaseUrl.value = (credentials?.base_url as string) || 'https://generativelanguage.googleapis.com'
    editApiKey.value = ''
    const tierId = (credentials?.tier_id as string) || ''
    if (tierId === 'aistudio_paid' || tierId === 'aistudio_free') {
      geminiTierAIStudio.value = tierId
    }
    loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
    loadPoolModeFromCredentials(credentials, poolModeEnabled, poolModeRetryCount)
    loadCustomErrorCodesFromCredentials(credentials, customErrorCodesEnabled, selectedErrorCodes)
  }

  function initOAuthEdit(credentials: Record<string, unknown> | undefined): void {
    const oauthType = (credentials?.oauth_type as string) || ''
    if (oauthType === 'code_assist' || oauthType === 'google_one' || oauthType === 'ai_studio') {
      geminiOAuthType.value = oauthType
    }
    const tierId = (credentials?.tier_id as string) || ''
    if (geminiOAuthType.value === 'google_one') {
      if (tierId === 'google_one_free' || tierId === 'google_ai_pro' || tierId === 'google_ai_ultra') {
        geminiTierGoogleOne.value = tierId
      }
    } else if (geminiOAuthType.value === 'code_assist') {
      if (tierId === 'gcp_standard' || tierId === 'gcp_enterprise') {
        geminiTierGcp.value = tierId
      }
    } else if (geminiOAuthType.value === 'ai_studio') {
      if (tierId === 'aistudio_paid' || tierId === 'aistudio_free') {
        geminiTierAIStudio.value = tierId
      }
    }
    loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
  }

  function getEditPayload(account: SdkAccount): EditFormPayload {
    const currentCreds = (account.credentials) || {}
    const newCreds: Record<string, unknown> = { ...currentCreds }
    const unschedResult = applyTempUnschedToCredentials(newCreds, tempUnschedEnabled.value, tempUnschedRules.value)
    if (!unschedResult.valid) return { credentials: undefined, error: unschedResult.error, common: commonFields.value }
    if (account.type === 'service_account') {
      return { ...buildServiceAccountEditPayload(newCreds), common: commonFields.value }
    }
    if (account.type === 'apikey') {
      return { ...buildApiKeyEditPayload(newCreds), common: commonFields.value }
    }
    if (account.type === 'oauth') {
      newCreds.oauth_type = geminiOAuthType.value
      newCreds.tier_id = geminiSelectedTier.value
      applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    }
    return { credentials: newCreds, common: commonFields.value }
  }

  function buildServiceAccountEditPayload(newCreds: Record<string, unknown>): EditFormPayload {
    if (!vertexProjectId.value.trim() || !vertexClientEmail.value.trim()) {
      return { credentials: undefined }
    }
    newCreds.project_id = vertexProjectId.value.trim()
    newCreds.client_email = vertexClientEmail.value.trim()
    newCreds.location = vertexLocation.value.trim()
    newCreds.tier_id = 'vertex'
    applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    return { credentials: newCreds }
  }

  function buildApiKeyEditPayload(newCreds: Record<string, unknown>): EditFormPayload {
    newCreds.base_url = apiKeyBaseUrl.value.trim() || 'https://generativelanguage.googleapis.com'
    newCreds.tier_id = geminiTierAIStudio.value
    if (editApiKey.value.trim()) newCreds.api_key = editApiKey.value.trim()
    applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    applyPoolModeToCredentials(newCreds, poolModeEnabled.value, poolModeRetryCount.value)
    applyCustomErrorCodesToCredentials(newCreds, customErrorCodesEnabled.value, selectedErrorCodes.value)
    return { credentials: newCreds }
  }

  function reset() {
    isEditMode.value = false
    geminiOAuthType.value = 'google_one'
    geminiAIStudioOAuthEnabled.value = false
    showAdvancedOAuth.value = false
    showGeminiHelpDialog.value = false
    apiKeyBaseUrl.value = 'https://generativelanguage.googleapis.com'
    apiKeyValue.value = ''
    geminiTierGoogleOne.value = 'google_one_free'
    geminiTierGcp.value = 'gcp_standard'
    geminiTierAIStudio.value = 'aistudio_free'
    vertexServiceAccountJson.value = ''
    vertexProjectId.value = ''
    vertexClientEmail.value = ''
    vertexLocation.value = vertexDefaultLocation
    modelRestrictionMode.value = 'whitelist'
    allowedModels.value = resolveAllProtocolModelIds(['gemini'])
    modelMappings.value = []
    poolModeEnabled.value = false
    poolModeRetryCount.value = 3
    customErrorCodesEnabled.value = false
    selectedErrorCodes.value = []
    tempUnschedEnabled.value = false
    tempUnschedRules.value = []
    commonFields.value = {
      name: '', notes: '',
      proxy_id: null, concurrency: 10, load_factor: null, priority: 1,
      rate_multiplier: 1, expires_at: null, auto_pause_on_expired: true,
      group_ids: [], quota_enabled: false, quota_limit: null,
      quota_daily_limit: null, quota_weekly_limit: null,
    }
    geminiOAuth.resetState()
  }

  return {
    apiKeyBaseUrl, apiKeyValue, editApiKey,
    geminiOAuthType, geminiAIStudioOAuthEnabled, showAdvancedOAuth, showGeminiHelpDialog,
    geminiTierGoogleOne, geminiTierGcp, geminiTierAIStudio, geminiSelectedTier,
    vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
    modelRestrictionMode, allowedModels, modelMappings, poolModeEnabled, poolModeRetryCount,
    customErrorCodesEnabled, selectedErrorCodes, tempUnschedEnabled, tempUnschedRules,
    geminiHelpLinks, geminiOAuth, oauthConfig,
    selectOAuthType, checkAIStudioCapability,
    validate, getPayload, reset, handleOAuthExchange,
    initFromAccount, getEditPayload
  }
}
