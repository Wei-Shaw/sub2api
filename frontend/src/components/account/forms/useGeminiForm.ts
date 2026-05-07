import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGeminiOAuth } from '@/composables/useGeminiOAuth'
import { getPresetMappingsByPlatform } from '@/composables/useModelWhitelist'
import type { PlatformFormPayload, PlatformFormValidation, OAuthFlowConfig, EditFormPayload } from './types'
import type { Account, CreateAccountRequest } from '@/types'
import * as editH from './editHelpers'

interface ModelMapping { from: string; to: string }

export function useGeminiForm() {
  const { t } = useI18n()
  const geminiOAuth = useGeminiOAuth()

  const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')
  const geminiAIStudioOAuthEnabled = ref(false)
  const showAdvancedOAuth = ref(false)
  const showGeminiHelpDialog = ref(false)
  const geminiTierGoogleOne = ref<'google_one_free' | 'google_ai_pro' | 'google_ai_ultra'>('google_one_free')
  const geminiTierGcp = ref<'gcp_standard' | 'gcp_enterprise'>('gcp_standard')
  const geminiTierAIStudio = ref<'aistudio_free' | 'aistudio_paid'>('aistudio_free')
  const vertexServiceAccountJson = ref('')
  const vertexProjectId = ref('')
  const vertexClientEmail = ref('')
  const vertexLocation = ref('global')
  const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
  const allowedModels = ref<string[]>([])
  const modelMappings = ref<ModelMapping[]>([])
  const poolModeEnabled = ref(false)
  const poolModeRetryCount = ref(3)
  const customErrorCodesEnabled = ref(false)
  const selectedErrorCodes = ref<number[]>([])
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<{ error_code: number | null; keywords: string; duration_minutes: number | null; description: string }[]>([])

  const presetMappings = computed(() => getPresetMappingsByPlatform('gemini'))
  const geminiSelectedTier = computed(() => {
    switch (geminiOAuthType.value) {
      case 'google_one': return geminiTierGoogleOne.value
      case 'code_assist': return geminiTierGcp.value
      default: return geminiTierAIStudio.value
    }
  })
  const oauthConfig: OAuthFlowConfig = { showProjectId: true, platform: 'gemini' }
  const geminiHelpLinks = { apiKey: 'https://aistudio.google.com/app/apikey', aiStudioPricing: 'https://ai.google.dev/pricing', gcpProject: 'https://console.cloud.google.com/welcome/new' }

  function selectOAuthType(type: 'code_assist' | 'google_one' | 'ai_studio') {
    if (type === 'ai_studio' && !geminiAIStudioOAuthEnabled.value) return
    geminiOAuthType.value = type
  }

  async function checkAIStudioCapability() {
    const caps = await geminiOAuth.getCapabilities()
    geminiAIStudioOAuthEnabled.value = !!caps?.ai_studio_oauth_enabled
    if (!geminiAIStudioOAuthEnabled.value && geminiOAuthType.value === 'ai_studio') geminiOAuthType.value = 'code_assist'
  }

  function validate(accountCategory: string): PlatformFormValidation {
    if (accountCategory === 'service_account') {
      if (!vertexServiceAccountJson.value.trim()) return { valid: false, error: t('admin.accounts.vertexSaJsonMissingFields') }
      if (!vertexLocation.value.trim()) return { valid: false, error: t('admin.accounts.vertexLocationRequired') }
    }
    return { valid: true }
  }

  function getPayload(accountCategory: string): PlatformFormPayload {
    if (accountCategory === 'service_account') {
      return { credentials: { service_account_json: vertexServiceAccountJson.value.trim(), project_id: vertexProjectId.value.trim(), client_email: vertexClientEmail.value.trim(), location: vertexLocation.value.trim(), tier_id: 'vertex' }, typeOverride: 'service_account' as any }
    }
    if (accountCategory === 'oauth-based') return { credentials: {}, needsOAuthFlow: true }
    return { credentials: { tier_id: geminiTierAIStudio.value } }
  }

  async function handleOAuthExchange(code: string, oauthState?: string, _projectId?: string): Promise<CreateAccountRequest | null> {
    if (!code.trim() || !geminiOAuth.sessionId.value) return null
    const stateToUse = oauthState || geminiOAuth.state.value; if (!stateToUse) return null
    const tokenInfo = await geminiOAuth.exchangeAuthCode({ code: code.trim(), sessionId: geminiOAuth.sessionId.value, state: stateToUse, proxyId: null, oauthType: geminiOAuthType.value, tierId: geminiSelectedTier.value })
    if (!tokenInfo) return null
    const creds = geminiOAuth.buildCredentials(tokenInfo); const extra = geminiOAuth.buildExtraInfo(tokenInfo)
    return { name: '', platform: 'gemini', type: 'oauth', credentials: creds, extra }
  }

  // ---- Edit mode ----

  function initFromAccount(account: Account): void {
    reset()
    const credentials = account.credentials as Record<string, unknown> | undefined
    editH.loadTempUnschedFromCredentials(credentials, tempUnschedEnabled, tempUnschedRules)
    if (account.type === 'service_account') {
      vertexProjectId.value = (credentials?.project_id as string) || ''
      vertexClientEmail.value = (credentials?.client_email as string) || ''
      vertexLocation.value = (credentials?.location as string) || (credentials?.vertex_location as string) || 'us-central1'
      editH.loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
    } else if (account.type === 'apikey') {
      editH.loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
      editH.loadPoolModeFromCredentials(credentials, poolModeEnabled, poolModeRetryCount)
      editH.loadCustomErrorCodesFromCredentials(credentials, customErrorCodesEnabled, selectedErrorCodes)
    } else if (account.type === 'oauth') {
      editH.loadModelMappingFromCredentials(credentials, modelRestrictionMode, allowedModels, modelMappings)
    }
  }

  function getEditPayload(account: Account): EditFormPayload {
    const currentCreds = (account.credentials as Record<string, unknown>) || {}
    const newCreds: Record<string, unknown> = { ...currentCreds }
    editH.applyTempUnschedToCredentials(newCreds, tempUnschedEnabled.value, tempUnschedRules.value)
    if (account.type === 'service_account') {
      if (!vertexProjectId.value.trim() || !vertexClientEmail.value.trim()) return { credentials: undefined }
      newCreds.project_id = vertexProjectId.value.trim(); newCreds.client_email = vertexClientEmail.value.trim()
      newCreds.location = vertexLocation.value.trim(); newCreds.tier_id = 'vertex'
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    } else if (account.type === 'apikey') {
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      editH.applyPoolModeToCredentials(newCreds, poolModeEnabled.value, poolModeRetryCount.value)
      editH.applyCustomErrorCodesToCredentials(newCreds, customErrorCodesEnabled.value, selectedErrorCodes.value)
    } else if (account.type === 'oauth') {
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    }
    return { credentials: newCreds }
  }

  function reset() {
    geminiOAuthType.value = 'google_one'; geminiAIStudioOAuthEnabled.value = false; showAdvancedOAuth.value = false; showGeminiHelpDialog.value = false
    geminiTierGoogleOne.value = 'google_one_free'; geminiTierGcp.value = 'gcp_standard'; geminiTierAIStudio.value = 'aistudio_free'
    vertexServiceAccountJson.value = ''; vertexProjectId.value = ''; vertexClientEmail.value = ''; vertexLocation.value = 'global'
    modelRestrictionMode.value = 'whitelist'; allowedModels.value = []; modelMappings.value = []
    poolModeEnabled.value = false; poolModeRetryCount.value = 3; customErrorCodesEnabled.value = false; selectedErrorCodes.value = []
    tempUnschedEnabled.value = false; tempUnschedRules.value = []
    geminiOAuth.resetState()
  }

  return {
    geminiOAuthType, geminiAIStudioOAuthEnabled, showAdvancedOAuth, showGeminiHelpDialog,
    geminiTierGoogleOne, geminiTierGcp, geminiTierAIStudio, geminiSelectedTier,
    vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
    modelRestrictionMode, allowedModels, modelMappings, poolModeEnabled, poolModeRetryCount,
    customErrorCodesEnabled, selectedErrorCodes, tempUnschedEnabled, tempUnschedRules,
    presetMappings, geminiHelpLinks, geminiOAuth, oauthConfig,
    selectOAuthType, checkAIStudioCapability,
    validate, getPayload, reset, handleOAuthExchange,
    initFromAccount, getEditPayload
  }
}
