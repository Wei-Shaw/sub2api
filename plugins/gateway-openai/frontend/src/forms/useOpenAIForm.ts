import { ref, computed, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useOpenAIOAuth } from '../composables/useOpenAIOAuth'
import { buildModelMappingObject, resolveAllProtocolModelIds, applyTempUnschedToCredentials } from '@sub2api/plugin-sdk'
import {
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode,
} from '../utils/openaiWsMode'
import type {
  PlatformFormPayload,
  PlatformFormValidation,
  OAuthFlowConfig,
  ModelMapping,
  SdkAccount,
  SdkCreateAccountRequest,
  CommonAccountFields,
} from '@sub2api/plugin-sdk'
import type { TempUnschedRuleForm } from '@sub2api/plugin-sdk'
import {
  initFromAccount as editInit,
  getEditPayload as editPayload,
  type OpenAIFormEditRefs,
} from './openAIFormEdit'

type OpenAICompactMode = 'auto' | 'force_on' | 'force_off'
type CodexImageGenBridgeMode = 'inherit' | 'enabled' | 'disabled'
const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

export function useOpenAIForm(commonFields: Ref<CommonAccountFields>) {
  const { t } = useI18n()
  const openaiOAuth = useOpenAIOAuth()

  const openaiPassthroughEnabled = ref(false)
  const openAICompactMode = ref<OpenAICompactMode>('auto')
  const openaiOAuthWSMode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
  const openaiAPIKeyWSMode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
  const codexCLIOnlyEnabled = ref(false)
  const codexImageGenerationBridgeMode = ref<CodexImageGenBridgeMode>('inherit')
  const apiKeyBaseUrl = ref('https://api.openai.com')
  const apiKeyValue = ref('')
  const editApiKey = ref('')
  const openAICompactModelMappings = ref<ModelMapping[]>([])
  const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
  const allowedModels = ref<string[]>([])
  const modelMappings = ref<ModelMapping[]>([])
  const poolModeEnabled = ref(false)
  const poolModeRetryCount = ref(3)
  const customErrorCodesEnabled = ref(false)
  const selectedErrorCodes = ref<number[]>([])
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<TempUnschedRuleForm[]>([])

  // ---- Computed options ----
  const openAICompactModeOptions = computed(() => [
    { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
    { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
    { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') },
  ])
  const openAIWSModeOptions = computed(() => [
    { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
    { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
    { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') },
  ])
  const codexImageGenerationBridgeOptions = computed<
    Array<{ value: CodexImageGenBridgeMode; label: string; description: string }>
  >(() => [
    { value: 'inherit', label: t('admin.accounts.openai.codexImageGenerationBridgeInherit'), description: t('admin.accounts.openai.codexImageGenerationBridgeInheritDesc') },
    { value: 'enabled', label: t('admin.accounts.openai.codexImageGenerationBridgeEnabled'), description: t('admin.accounts.openai.codexImageGenerationBridgeEnabledDesc') },
    { value: 'disabled', label: t('admin.accounts.openai.codexImageGenerationBridgeDisabled'), description: t('admin.accounts.openai.codexImageGenerationBridgeDisabledDesc') },
  ])
  const codexImageGenerationBridgeBadgeLabel = computed(() => {
    switch (codexImageGenerationBridgeMode.value) {
      case 'enabled': return t('admin.accounts.openai.codexImageGenerationBridgeBadgeEnabled')
      case 'disabled': return t('admin.accounts.openai.codexImageGenerationBridgeBadgeDisabled')
      default: return t('admin.accounts.openai.codexImageGenerationBridgeBadgeInherit')
    }
  })
  const codexImageGenerationBridgeBadgeClass = computed(() => {
    switch (codexImageGenerationBridgeMode.value) {
      case 'enabled': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
      case 'disabled': return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300'
      default: return 'bg-slate-100 text-slate-600 dark:bg-dark-600 dark:text-slate-300'
    }
  })
  const isModelRestrictionDisabled = computed(() => openaiPassthroughEnabled.value)

  watch(modelRestrictionMode, (newMode) => {
    if (newMode === 'whitelist') allowedModels.value = resolveAllProtocolModelIds(['openai'])
  }, { immediate: true })

  const oauthConfig: OAuthFlowConfig = {
    showRefreshTokenOption: true, showMobileRefreshTokenOption: true,
    showCodexSessionImportOption: true, showProxyWarning: false, platform: 'openai',
    showImportantNotice: true,
    i18nPrefix: 'admin.accounts.oauth.openai',
  }

  const getWSMode = (cat: string): OpenAIWSMode => cat === 'apikey' ? openaiAPIKeyWSMode.value : openaiOAuthWSMode.value
  const setWSMode = (cat: string, mode: OpenAIWSMode) => {
    if (cat === 'apikey') openaiAPIKeyWSMode.value = mode; else openaiOAuthWSMode.value = mode
  }
  const wsModeHintKey = (cat: string): string => resolveOpenAIWSModeConcurrencyHintKey(getWSMode(cat))

  function buildExtra(category: string, oauthExtra?: Record<string, unknown>): Record<string, unknown> | undefined {
    const extra: Record<string, unknown> = { ...(oauthExtra || {}) }
    if (category === 'oauth-based') {
      extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthWSMode.value
      extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthWSMode.value)
    } else if (category === 'apikey') {
      extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyWSMode.value
      extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyWSMode.value)
    }
    delete extra.responses_websockets_v2_enabled; delete extra.openai_ws_enabled
    if (openaiPassthroughEnabled.value) extra.openai_passthrough = true
    else { delete extra.openai_passthrough; delete extra.openai_oauth_passthrough }
    if (category === 'oauth-based' && codexCLIOnlyEnabled.value) extra.codex_cli_only = true
    else delete extra.codex_cli_only
    if (openAICompactMode.value !== 'auto') extra.openai_compact_mode = openAICompactMode.value
    else delete extra.openai_compact_mode
    if (codexImageGenerationBridgeMode.value !== 'inherit') {
      extra.codex_image_generation_bridge = codexImageGenerationBridgeMode.value === 'enabled'
    } else {
      delete extra.codex_image_generation_bridge
    }
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  function applyModelRestriction(creds: Record<string, unknown>) {
    if (!isModelRestrictionDisabled.value) {
      const mm = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      if (mm) creds.model_mapping = mm
    }
    const cm = buildModelMappingObject('mapping', [], openAICompactModelMappings.value) ?? undefined
    if (cm) creds.compact_model_mapping = cm
  }

  const validate = (category?: string, mode?: string): PlatformFormValidation => {
    if (category === 'apikey' && mode !== 'edit' && !apiKeyValue.value.trim()) {
      return { valid: false, error: t('admin.accounts.pleaseEnterApiKey') }
    }
    return { valid: true }
  }

  function getPayload(category: string): PlatformFormPayload {
    if (category === 'oauth-based') return { credentials: {}, extra: buildExtra(category), common: commonFields.value, needsOAuthFlow: true }
    const credentials: Record<string, unknown> = {
      base_url: apiKeyBaseUrl.value.trim() || 'https://api.openai.com',
      api_key: apiKeyValue.value.trim(),
    }
    applyModelRestriction(credentials)
    if (poolModeEnabled.value) { credentials.pool_mode = true; credentials.pool_mode_retry_count = poolModeRetryCount.value }
    if (customErrorCodesEnabled.value) { credentials.custom_error_codes_enabled = true; credentials.custom_error_codes = [...selectedErrorCodes.value] }
    applyTempUnschedToCredentials(credentials, tempUnschedEnabled.value, tempUnschedRules.value)
    return { credentials, extra: buildExtra(category), common: commonFields.value }
  }

  async function handleOAuthExchange(code: string, oauthState?: string): Promise<SdkCreateAccountRequest | null> {
    if (!code.trim() || !openaiOAuth.sessionId.value) return null
    const stateToUse = oauthState || openaiOAuth.oauthState.value || ''
    if (!stateToUse) return null
    const tokenInfo = await openaiOAuth.exchangeAuthCode(code.trim(), openaiOAuth.sessionId.value, stateToUse, null)
    if (!tokenInfo) return null
    const creds = openaiOAuth.buildCredentials(tokenInfo)
    const oauthX = openaiOAuth.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
    applyModelRestriction(creds)
    applyTempUnschedToCredentials(creds, tempUnschedEnabled.value, tempUnschedRules.value)
    return { name: '', platform: 'openai', type: 'oauth', credentials: creds, extra: buildExtra('oauth-based', oauthX) }
  }

  const handleRefreshToken = (rt: string) => batchRT(rt)
  const handleMobileRefreshToken = (rt: string) => batchRT(rt, OPENAI_MOBILE_RT_CLIENT_ID)

  async function batchRT(input: string, clientId?: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null> {
    const tokens = input.split('\n').map(s => s.trim()).filter(Boolean)
    if (tokens.length === 0) return null
    const results: SdkCreateAccountRequest[] = []
    for (const token of tokens) {
      const info = await openaiOAuth.validateRefreshToken(token, null, clientId)
      if (!info) continue
      const creds = openaiOAuth.buildCredentials(info)
      if (clientId) creds.client_id = clientId
      const oauthX = openaiOAuth.buildExtraInfo(info) as Record<string, unknown> | undefined
      applyModelRestriction(creds)
      applyTempUnschedToCredentials(creds, tempUnschedEnabled.value, tempUnschedRules.value)
      results.push({ name: '', platform: 'openai', type: 'oauth', credentials: creds, extra: buildExtra('oauth-based', oauthX) })
    }
    return results.length === 1 ? results[0] : results.length > 0 ? results : null
  }

  const editRefs: OpenAIFormEditRefs = {
    openaiPassthroughEnabled, openAICompactMode, openaiOAuthWSMode, openaiAPIKeyWSMode,
    codexCLIOnlyEnabled, codexImageGenerationBridgeMode, apiKeyBaseUrl, editApiKey,
    openAICompactModelMappings, modelRestrictionMode, allowedModels, modelMappings,
    poolModeEnabled, poolModeRetryCount, customErrorCodesEnabled, selectedErrorCodes,
    tempUnschedEnabled, tempUnschedRules,
  }

  function reset() {
    apiKeyBaseUrl.value = 'https://api.openai.com'; apiKeyValue.value = ''
    openaiPassthroughEnabled.value = false; openAICompactMode.value = 'auto'
    openaiOAuthWSMode.value = OPENAI_WS_MODE_OFF; openaiAPIKeyWSMode.value = OPENAI_WS_MODE_OFF
    codexCLIOnlyEnabled.value = false; codexImageGenerationBridgeMode.value = 'inherit'
    openAICompactModelMappings.value = []; modelRestrictionMode.value = 'whitelist'
    allowedModels.value = resolveAllProtocolModelIds(['openai']); modelMappings.value = []
    poolModeEnabled.value = false; poolModeRetryCount.value = 3
    customErrorCodesEnabled.value = false; selectedErrorCodes.value = []
    tempUnschedEnabled.value = false; tempUnschedRules.value = []
    commonFields.value = {
      name: '', notes: '',
      proxy_id: null, concurrency: 10, load_factor: null, priority: 1,
      rate_multiplier: 1, expires_at: null, auto_pause_on_expired: true,
      group_ids: [], quota_enabled: false, quota_limit: null,
      quota_daily_limit: null, quota_weekly_limit: null,
    }
    openaiOAuth.resetState()
  }

  return {
    apiKeyBaseUrl, apiKeyValue, editApiKey,
    openaiPassthroughEnabled, openAICompactMode, openaiOAuthWSMode, openaiAPIKeyWSMode,
    codexCLIOnlyEnabled, codexImageGenerationBridgeMode,
    codexImageGenerationBridgeOptions, codexImageGenerationBridgeBadgeLabel, codexImageGenerationBridgeBadgeClass,
    openAICompactModelMappings, modelRestrictionMode, allowedModels,
    modelMappings, poolModeEnabled, poolModeRetryCount,
    customErrorCodesEnabled, selectedErrorCodes, tempUnschedEnabled, tempUnschedRules,
    openAICompactModeOptions, openAIWSModeOptions, isModelRestrictionDisabled,
    openaiOAuth, oauthConfig, getWSMode, setWSMode, wsModeHintKey,
    validate, getPayload, reset, handleOAuthExchange, handleRefreshToken, handleMobileRefreshToken,
    initFromAccount: (account: SdkAccount) => {
      reset()
      editInit(account, editRefs)
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
    },
    getEditPayload: (account: SdkAccount) => {
      const payload = editPayload(account, editRefs)
      payload.common = commonFields.value
      return payload
    },
  }
}