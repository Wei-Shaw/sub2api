import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import {
  buildModelMappingObject,
  getModelsByPlatform,
  getPresetMappingsByPlatform
} from '@/composables/useModelWhitelist'
import {
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  resolveOpenAIWSModeFromExtra,
  type OpenAIWSMode
} from '@/utils/openaiWsMode'
import type { PlatformFormPayload, PlatformFormValidation, OAuthFlowConfig, EditFormPayload, ModelMapping } from './types'
import type { Account, CreateAccountRequest, OpenAICompactMode } from '@/types'
import * as editH from './editHelpers'

const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

export function useOpenAIForm() {
  const { t } = useI18n()
  const openaiOAuth = useOpenAIOAuth()
  const openaiPassthroughEnabled = ref(false)
  const openAICompactMode = ref<OpenAICompactMode>('auto')
  const openaiOAuthWSMode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
  const openaiAPIKeyWSMode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
  const codexCLIOnlyEnabled = ref(false)
  const apiKeyBaseUrl = ref('https://api.openai.com')
  const apiKeyValue = ref('')
  const openAICompactModelMappings = ref<ModelMapping[]>([])
  const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
  const allowedModels = ref<string[]>([])
  const modelMappings = ref<ModelMapping[]>([])
  const poolModeEnabled = ref(false)
  const poolModeRetryCount = ref(3)
  const customErrorCodesEnabled = ref(false)
  const selectedErrorCodes = ref<number[]>([])
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<editH.TempUnschedRuleForm[]>([])
  const presetMappings = computed(() => getPresetMappingsByPlatform('openai'))
  const openAICompactModeOptions = computed(() => [
    { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
    { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
    { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
  ])
  const openAIWSModeOptions = computed(() => [
    { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
    { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
    { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') }
  ])
  watch(modelRestrictionMode, (newMode) => {
    if (newMode === 'whitelist') {
      allowedModels.value = [...getModelsByPlatform('openai')]
    }
  })

  const isModelRestrictionDisabled = computed(() => openaiPassthroughEnabled.value)
  const oauthConfig: OAuthFlowConfig = { showRefreshTokenOption: true, showMobileRefreshTokenOption: true, showProxyWarning: false, platform: 'openai' }

  const getWSMode = (category: string): OpenAIWSMode => category === 'apikey' ? openaiAPIKeyWSMode.value : openaiOAuthWSMode.value
  const setWSMode = (category: string, mode: OpenAIWSMode) => { if (category === 'apikey') openaiAPIKeyWSMode.value = mode; else openaiOAuthWSMode.value = mode }
  const wsModeHintKey = (category: string): string => resolveOpenAIWSModeConcurrencyHintKey(getWSMode(category))

  function buildExtra(category: string, oauthExtra?: Record<string, unknown>): Record<string, unknown> | undefined {
    const extra: Record<string, unknown> = { ...(oauthExtra || {}) }
    if (category === 'oauth-based') { extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthWSMode.value; extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthWSMode.value) }
    else if (category === 'apikey') { extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyWSMode.value; extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyWSMode.value) }
    delete extra.responses_websockets_v2_enabled; delete extra.openai_ws_enabled
    if (openaiPassthroughEnabled.value) extra.openai_passthrough = true; else { delete extra.openai_passthrough; delete extra.openai_oauth_passthrough }
    if (category === 'oauth-based' && codexCLIOnlyEnabled.value) extra.codex_cli_only = true; else delete extra.codex_cli_only
    if (openAICompactMode.value !== 'auto') extra.openai_compact_mode = openAICompactMode.value; else delete extra.openai_compact_mode
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  function buildCompactModelMapping(): Record<string, unknown> | undefined {
    return buildModelMappingObject('mapping', [], openAICompactModelMappings.value) ?? undefined
  }

  function applyModelRestriction(creds: Record<string, unknown>) {
    if (!isModelRestrictionDisabled.value) { const mm = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value); if (mm) creds.model_mapping = mm }
    const cm = buildCompactModelMapping(); if (cm) creds.compact_model_mapping = cm
  }

  const validate = (category?: string): PlatformFormValidation => {
    if (category === 'apikey' && !apiKeyValue.value.trim()) {
      return { valid: false, error: t('admin.accounts.pleaseEnterApiKey') }
    }
    return { valid: true }
  }

  function getPayload(category: string): PlatformFormPayload {
    if (category === 'oauth-based') return { credentials: {}, extra: buildExtra(category), needsOAuthFlow: true }
    const credentials: Record<string, unknown> = {
      base_url: apiKeyBaseUrl.value.trim() || 'https://api.openai.com',
      api_key: apiKeyValue.value.trim()
    }
    applyModelRestriction(credentials)
    if (poolModeEnabled.value) { credentials.pool_mode = true; credentials.pool_mode_retry_count = poolModeRetryCount.value }
    if (customErrorCodesEnabled.value) { credentials.custom_error_codes_enabled = true; credentials.custom_error_codes = [...selectedErrorCodes.value] }
    return { credentials, extra: buildExtra(category) }
  }

  async function handleOAuthExchange(code: string, oauthState?: string): Promise<CreateAccountRequest | null> {
    if (!code.trim() || !openaiOAuth.sessionId.value) return null
    const stateToUse = oauthState || openaiOAuth.oauthState.value || ''; if (!stateToUse) return null
    const tokenInfo = await openaiOAuth.exchangeAuthCode(code.trim(), openaiOAuth.sessionId.value, stateToUse, null)
    if (!tokenInfo) return null
    const creds = openaiOAuth.buildCredentials(tokenInfo)
    const oauthExtra = openaiOAuth.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
    applyModelRestriction(creds)
    return { name: '', platform: 'openai', type: 'oauth', credentials: creds, extra: buildExtra('oauth-based', oauthExtra) }
  }

  const handleRefreshToken = (rt: string) => batchRT(rt)
  const handleMobileRefreshToken = (rt: string) => batchRT(rt, OPENAI_MOBILE_RT_CLIENT_ID)

  async function batchRT(input: string, clientId?: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null> {
    const tokens = input.split('\n').map(s => s.trim()).filter(Boolean); if (tokens.length === 0) return null
    const results: CreateAccountRequest[] = []
    for (const token of tokens) {
      const info = await openaiOAuth.validateRefreshToken(token, null, clientId); if (!info) continue
      const creds = openaiOAuth.buildCredentials(info); if (clientId) creds.client_id = clientId
      const oauthExtra = openaiOAuth.buildExtraInfo(info) as Record<string, unknown> | undefined
      applyModelRestriction(creds)
      results.push({ name: '', platform: 'openai', type: 'oauth', credentials: creds, extra: buildExtra('oauth-based', oauthExtra) })
    }
    return results.length === 1 ? results[0] : results.length > 0 ? results : null
  }

  // ---- Edit mode ----

  function initFromAccount(account: Account): void {
    reset()
    const credentials = account.credentials as Record<string, unknown> | undefined
    const extra = account.extra as Record<string, unknown> | undefined
    editH.loadTempUnschedFromCredentials(credentials, tempUnschedEnabled, tempUnschedRules)
    openaiPassthroughEnabled.value = extra?.openai_passthrough === true || extra?.openai_oauth_passthrough === true
    openAICompactMode.value = (extra?.openai_compact_mode as OpenAICompactMode) || 'auto'
    openaiOAuthWSMode.value = resolveOpenAIWSModeFromExtra(extra, { modeKey: 'openai_oauth_responses_websockets_v2_mode', enabledKey: 'openai_oauth_responses_websockets_v2_enabled', fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'], defaultMode: OPENAI_WS_MODE_OFF })
    openaiAPIKeyWSMode.value = resolveOpenAIWSModeFromExtra(extra, { modeKey: 'openai_apikey_responses_websockets_v2_mode', enabledKey: 'openai_apikey_responses_websockets_v2_enabled', fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'], defaultMode: OPENAI_WS_MODE_OFF })
    codexCLIOnlyEnabled.value = account.type === 'oauth' ? (extra?.codex_cli_only === true) : false
    editH.loadCompactModelMappingsFromCredentials(credentials, openAICompactModelMappings)
    if (account.type === 'apikey') {
      apiKeyBaseUrl.value = (credentials?.base_url as string) || 'https://api.openai.com'
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
    const currentExtra = (account.extra as Record<string, unknown>) || {}
    const newExtra: Record<string, unknown> = { ...currentExtra }
    editH.applyTempUnschedToCredentials(newCreds, tempUnschedEnabled.value, tempUnschedRules.value)
    const shouldApply = !openaiPassthroughEnabled.value
    if (account.type === 'apikey' || account.type === 'oauth') {
      editH.applyModelMappingToCredentials(newCreds, modelRestrictionMode.value, allowedModels.value, modelMappings.value, !shouldApply, currentCreds.model_mapping)
      const cm = buildCompactModelMapping(); if (cm) newCreds.compact_model_mapping = cm; else delete newCreds.compact_model_mapping
    }
    if (account.type === 'apikey') {
      newCreds.base_url = apiKeyBaseUrl.value.trim() || 'https://api.openai.com'
      editH.applyPoolModeToCredentials(newCreds, poolModeEnabled.value, poolModeRetryCount.value)
      editH.applyCustomErrorCodesToCredentials(newCreds, customErrorCodesEnabled.value, selectedErrorCodes.value)
    }
    if (account.type === 'oauth') { newExtra.openai_oauth_responses_websockets_v2_mode = openaiOAuthWSMode.value; newExtra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthWSMode.value) }
    else if (account.type === 'apikey') { newExtra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyWSMode.value; newExtra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyWSMode.value) }
    delete newExtra.responses_websockets_v2_enabled; delete newExtra.openai_ws_enabled
    if (openaiPassthroughEnabled.value) newExtra.openai_passthrough = true; else { delete newExtra.openai_passthrough; delete newExtra.openai_oauth_passthrough }
    if (openAICompactMode.value !== 'auto') newExtra.openai_compact_mode = openAICompactMode.value; else delete newExtra.openai_compact_mode
    if (account.type === 'oauth') { const hadCodex = currentExtra.codex_cli_only === true; if (codexCLIOnlyEnabled.value) newExtra.codex_cli_only = true; else if (hadCodex) newExtra.codex_cli_only = false; else delete newExtra.codex_cli_only }
    return { credentials: newCreds, extra: newExtra }
  }

  function reset() {
    apiKeyBaseUrl.value = 'https://api.openai.com'; apiKeyValue.value = ''
    openaiPassthroughEnabled.value = false; openAICompactMode.value = 'auto'
    openaiOAuthWSMode.value = OPENAI_WS_MODE_OFF; openaiAPIKeyWSMode.value = OPENAI_WS_MODE_OFF
    codexCLIOnlyEnabled.value = false; openAICompactModelMappings.value = []
    modelRestrictionMode.value = 'whitelist'; allowedModels.value = [...getModelsByPlatform('openai')]; modelMappings.value = []
    poolModeEnabled.value = false; poolModeRetryCount.value = 3
    customErrorCodesEnabled.value = false; selectedErrorCodes.value = []
    tempUnschedEnabled.value = false; tempUnschedRules.value = []
    openaiOAuth.resetState()
  }

  return {
    apiKeyBaseUrl, apiKeyValue,
    openaiPassthroughEnabled, openAICompactMode, openaiOAuthWSMode, openaiAPIKeyWSMode,
    codexCLIOnlyEnabled, openAICompactModelMappings, modelRestrictionMode, allowedModels,
    modelMappings, poolModeEnabled, poolModeRetryCount, customErrorCodesEnabled,
    selectedErrorCodes, tempUnschedEnabled, tempUnschedRules, presetMappings,
    openAICompactModeOptions, openAIWSModeOptions, isModelRestrictionDisabled,
    openaiOAuth, oauthConfig, getWSMode, setWSMode, wsModeHintKey,
    buildExtra, buildCompactModelMapping, applyModelRestriction,
    validate, getPayload, reset, handleOAuthExchange, handleRefreshToken, handleMobileRefreshToken,
    initFromAccount, getEditPayload
  }
}
