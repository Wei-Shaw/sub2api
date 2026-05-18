/**
 * Main Anthropic form composable -- orchestrates sub-composables.
 *
 * Split into:
 *  - useAnthropicBedrockForm  -- Bedrock-specific refs
 *  - useAnthropicOAuthForm    -- OAuth quota control refs
 *  - useAnthropicEditMode     -- edit-mode init/payload builders
 *  - useAccountOAuth          -- OAuth auth flow (URL generation)
 *  - formShared               -- model mapping / shared credentials
 */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  PlatformFormPayload, PlatformFormValidation, OAuthFlowConfig,
  ModelMapping, SdkAccount, SdkCreateAccountRequest,
} from '@sub2api/plugin-sdk'
import { applyInterceptWarmup, applyTempUnschedToCredentials, resolveAllProtocolModelIds, type TempUnschedRuleForm } from '@sub2api/plugin-sdk'
import { getClient } from '../api/client'
import { useAccountOAuth, type AddMethod } from '../composables/useAccountOAuth'
import { useAnthropicBedrockForm } from './useAnthropicBedrockForm'
import { useAnthropicOAuthForm } from './useAnthropicOAuthForm'
import { applySharedCredentials, buildModelMappingObject } from './formShared'
import * as editMode from './useAnthropicEditMode'

export function useAnthropicForm() {
  const { t } = useI18n()
  const oauth = useAccountOAuth()
  const bedrock = useAnthropicBedrockForm()
  const oauthQuota = useAnthropicOAuthForm()

  const addMethod = ref<AddMethod>('oauth')
  const apiKeyBaseUrl = ref('https://api.anthropic.com')
  const apiKeyValue = ref('')
  const editApiKey = ref('')
  const vertexServiceAccountJson = ref('')
  const vertexProjectId = ref('')
  const vertexClientEmail = ref('')
  const vertexLocation = ref('global')
  const anthropicPassthroughEnabled = ref(false)
  const webSearchEmulationMode = ref('default')
  const syncToStreamMode = ref('default')
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
  const tempUnschedRules = ref<TempUnschedRuleForm[]>([])

  watch(modelRestrictionMode, (m) => {
    if (m === 'whitelist') allowedModels.value = resolveAllProtocolModelIds(['anthropic'])
  }, { immediate: true })

  const oauthConfig: OAuthFlowConfig = {
    showCookieOption: true, showRefreshTokenOption: false, showHelp: true,
    allowMultiple: true, needsMixedChannelCheck: true, platform: 'anthropic',
    i18nPrefix: 'admin.accounts.oauth',
  }

  function isOAuthFlow(cat: string): boolean { return cat === 'oauth-based' }

  async function loadWebSearchEnabled(): Promise<void> {
    try {
      const { data } = await getClient().get<{ enabled: boolean; providers: unknown[] }>(
        '/admin/settings/web-search-emulation')
      webSearchGlobalEnabled.value = data?.enabled === true && (data?.providers?.length ?? 0) > 0
    } catch { webSearchGlobalEnabled.value = false }
  }

  function sharedOpts() {
    return {
      modelRestrictionMode: modelRestrictionMode.value,
      allowedModels: allowedModels.value,
      modelMappings: modelMappings.value,
      poolModeEnabled: poolModeEnabled.value,
      poolModeRetryCount: poolModeRetryCount.value,
      customErrorCodesEnabled: customErrorCodesEnabled.value,
      selectedErrorCodes: selectedErrorCodes.value,
      interceptWarmupRequests: interceptWarmupRequests.value,
      tempUnschedEnabled: tempUnschedEnabled.value,
      tempUnschedRules: tempUnschedRules.value,
    }
  }

  function validate(cat: string, mode?: string): PlatformFormValidation {
    if (cat === 'bedrock') {
      if (bedrock.bedrockAuthMode.value === 'sigv4') {
        if (!bedrock.bedrockAccessKeyId.value.trim()) return { valid: false, error: t('admin.accounts.bedrockAccessKeyIdRequired') }
        if (!bedrock.bedrockSecretAccessKey.value.trim()) return { valid: false, error: t('admin.accounts.bedrockSecretAccessKeyRequired') }
      } else if (!bedrock.bedrockApiKeyValue.value.trim()) return { valid: false, error: t('admin.accounts.bedrockApiKeyRequired') }
    }
    if (cat === 'service_account') {
      if (!vertexServiceAccountJson.value.trim()) return { valid: false, error: t('admin.accounts.vertexSaJsonMissingFields') }
      if (!vertexLocation.value.trim()) return { valid: false, error: t('admin.accounts.vertexLocationRequired') }
    }
    if (cat === 'apikey' && mode !== 'edit' && !apiKeyValue.value.trim()) return { valid: false, error: t('admin.accounts.pleaseEnterApiKey') }
    return { valid: true }
  }

  function getPayload(cat: string): PlatformFormPayload {
    if (cat === 'bedrock') { const c = bedrock.buildBedrockCredentials(); applySharedCredentials(c, sharedOpts()); return { credentials: c, typeOverride: 'bedrock' } }
    if (cat === 'service_account') return buildServiceAccountPayload()
    if (cat === 'apikey') return buildApiKeyPayload()
    return { credentials: {}, needsOAuthFlow: true, typeOverride: addMethod.value }
  }

  function buildServiceAccountPayload(): PlatformFormPayload {
    const c: Record<string, unknown> = { service_account_json: vertexServiceAccountJson.value.trim(), project_id: vertexProjectId.value.trim(), client_email: vertexClientEmail.value.trim(), location: vertexLocation.value.trim(), tier_id: 'vertex' }
    const mm = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (mm) c.model_mapping = mm
    applyTempUnschedToCredentials(c, tempUnschedEnabled.value, tempUnschedRules.value)
    return { credentials: c, typeOverride: 'service_account' }
  }

  function buildApiKeyPayload(): PlatformFormPayload {
    const c: Record<string, unknown> = { base_url: apiKeyBaseUrl.value.trim() || 'https://api.anthropic.com', api_key: apiKeyValue.value.trim() }
    applySharedCredentials(c, sharedOpts())
    const extra: Record<string, unknown> = {}
    if (anthropicPassthroughEnabled.value) extra.anthropic_passthrough = true
    if (webSearchEmulationMode.value !== 'default') extra.web_search_emulation = webSearchEmulationMode.value
    if (syncToStreamMode.value !== 'default') extra.sync_to_stream = syncToStreamMode.value
    return { credentials: c, extra: Object.keys(extra).length > 0 ? extra : undefined }
  }

  async function handleOAuthExchange(code: string): Promise<SdkCreateAccountRequest | null> {
    if (!code.trim() || !oauth.sessionId.value) return null
    const ep = addMethod.value === 'oauth' ? '/admin/accounts/exchange-code' : '/admin/accounts/exchange-setup-token-code'
    const { data: ti } = await getClient().post<Record<string, unknown>>(ep, { session_id: oauth.sessionId.value, code: code.trim() })
    return buildOAuthResult(ti)
  }

  async function handleCookieAuth(key: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null> {
    const keys = oauth.parseSessionKeys(key)
    if (!keys.length) return null
    const ep = addMethod.value === 'oauth' ? '/admin/accounts/cookie-auth' : '/admin/accounts/setup-token-cookie-auth'
    const results: SdkCreateAccountRequest[] = []
    for (const k of keys) {
      const { data: ti } = await getClient().post<Record<string, unknown>>(ep, { session_id: '', code: k })
      const r = buildOAuthResult(ti)
      if (r) results.push(r)
    }
    return results.length === 1 ? results[0] : results.length > 0 ? results : null
  }

  function buildOAuthResult(ti: Record<string, unknown>): SdkCreateAccountRequest {
    const base = oauth.buildExtraInfo(ti as { org_uuid?: string; account_uuid?: string; email_address?: string }) || {}
    const extra = oauthQuota.buildOAuthExtra(base)
    if (syncToStreamMode.value !== 'default') extra.sync_to_stream = syncToStreamMode.value
    const creds: Record<string, unknown> = { ...ti }
    applyInterceptWarmup(creds, interceptWarmupRequests.value, 'create')
    applyTempUnschedToCredentials(creds, tempUnschedEnabled.value, tempUnschedRules.value)
    return { name: '', platform: 'anthropic', type: addMethod.value, credentials: creds, extra: Object.keys(extra).length > 0 ? extra : undefined }
  }

  function reset() {
    addMethod.value = 'oauth'; apiKeyBaseUrl.value = 'https://api.anthropic.com'
    apiKeyValue.value = ''; editApiKey.value = ''
    vertexServiceAccountJson.value = ''; vertexProjectId.value = ''; vertexClientEmail.value = ''; vertexLocation.value = 'global'
    anthropicPassthroughEnabled.value = false; webSearchEmulationMode.value = 'default'; syncToStreamMode.value = 'default'
    interceptWarmupRequests.value = false; modelRestrictionMode.value = 'whitelist'; allowedModels.value = resolveAllProtocolModelIds(['anthropic'])
    modelMappings.value = []; poolModeEnabled.value = false; poolModeRetryCount.value = 3
    customErrorCodesEnabled.value = false; selectedErrorCodes.value = []; tempUnschedEnabled.value = false; tempUnschedRules.value = []
    bedrock.resetBedrock(); oauthQuota.resetOAuthQuota(); oauth.resetState()
  }

  const editRefs: editMode.EditModeRefs = {
    addMethod, apiKeyBaseUrl, editApiKey, vertexLocation,
    anthropicPassthroughEnabled, webSearchEmulationMode, syncToStreamMode,
    interceptWarmupRequests, modelRestrictionMode, allowedModels, modelMappings,
    poolModeEnabled, poolModeRetryCount, customErrorCodesEnabled, selectedErrorCodes,
    tempUnschedEnabled, tempUnschedRules,
  }

  return {
    addMethod, apiKeyBaseUrl, apiKeyValue, editApiKey,
    ...bedrock, ...oauthQuota,
    vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
    anthropicPassthroughEnabled, webSearchEmulationMode, syncToStreamMode, webSearchGlobalEnabled,
    interceptWarmupRequests, modelRestrictionMode, allowedModels, modelMappings,
    poolModeEnabled, poolModeRetryCount, customErrorCodesEnabled, selectedErrorCodes,
    tempUnschedEnabled, tempUnschedRules, oauth, oauthConfig,
    isOAuthFlow, loadTlsProfiles: oauthQuota.loadTlsProfiles, loadWebSearchEnabled,
    validate, getPayload, reset, handleOAuthExchange, handleCookieAuth,
    initFromAccount: (a: SdkAccount) => editMode.initFromAccount(a, editRefs, bedrock, oauthQuota, reset),
    getEditPayload: (a: SdkAccount) => editMode.getEditPayload(a, editRefs, bedrock, oauthQuota),
  }
}
