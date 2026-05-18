/**
 * Antigravity account form composable (plugin-local).
 *
 * Migrated from host useAntigravityForm.ts. Host-internal imports
 * replaced with plugin-sdk helpers or plugin-local modules.
 */
import { ref, computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  ModelMapping, PlatformFormPayload, PlatformFormValidation,
  OAuthFlowConfig, SdkAccount, SdkCreateAccountRequest, TempUnschedRuleForm,
  CommonAccountFields,
} from '@sub2api/plugin-sdk'
import { applyInterceptWarmup, applyTempUnschedToCredentials } from '@sub2api/plugin-sdk'
import { useAntigravityOAuth } from './useAntigravityOAuth'
import { antigravityPresetMappings } from './presets'
import { isValidWildcardPattern, buildModelMappingObject, fetchDefaultMappings } from './modelMapping'
import { initFromAccount as doInit, getEditPayload as doEditPayload } from './editHelpers'

export { isValidWildcardPattern }

export function useAntigravityForm(commonFields: Ref<CommonAccountFields>) {
  const { t } = useI18n()
  const antigravityOAuth = useAntigravityOAuth()

  const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth')
  const upstreamBaseUrl = ref('')
  const upstreamApiKey = ref('')
  const editUpstreamApiKey = ref('')
  const antigravityModelMappings = ref<ModelMapping[]>([])
  const mixedScheduling = ref(false)
  const allowOverages = ref(false)
  const interceptWarmupRequests = ref(false)
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
  let editModeActive = false

  const presetMappings = computed(() => antigravityPresetMappings)
  const isOAuthFlow = () => antigravityAccountType.value === 'oauth'
  const oauthConfig: OAuthFlowConfig = {
    showRefreshTokenOption: true, needsMixedChannelCheck: true, platform: 'antigravity',
    showImportantNotice: true,
    i18nPrefix: 'admin.accounts.oauth.antigravity',
  }
  const formRefs = {
    antigravityAccountType, upstreamBaseUrl, editUpstreamApiKey,
    antigravityModelMappings, mixedScheduling, allowOverages,
    interceptWarmupRequests, tempUnschedEnabled, tempUnschedRules,
  }

  fetchDefaultMappings().then(defaults => {
    if (!editModeActive && defaults.length > 0 && antigravityModelMappings.value.length === 0)
      antigravityModelMappings.value = defaults
  })

  function buildExtra(): Record<string, unknown> | undefined {
    const extra: Record<string, unknown> = {}
    if (mixedScheduling.value) extra.mixed_scheduling = true
    if (allowOverages.value) extra.allow_overages = true
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  function buildMapping(): Record<string, unknown> | undefined {
    return buildModelMappingObject(antigravityModelMappings.value) ?? undefined
  }

  function validate(mode?: string): PlatformFormValidation {
    if (antigravityAccountType.value === 'upstream') {
      if (!upstreamBaseUrl.value.trim())
        return { valid: false, error: t('admin.accounts.upstream.pleaseEnterBaseUrl') }
      if (mode !== 'edit' && !upstreamApiKey.value.trim())
        return { valid: false, error: t('admin.accounts.upstream.pleaseEnterApiKey') }
    }
    for (const m of antigravityModelMappings.value) {
      if (m.from && !isValidWildcardPattern(m.from))
        return { valid: false, error: t('admin.accounts.wildcardOnlyAtEnd') }
      if (m.to && m.to.includes('*'))
        return { valid: false, error: t('admin.accounts.targetNoWildcard') }
    }
    return { valid: true }
  }

  function getPayload(): PlatformFormPayload {
    if (antigravityAccountType.value === 'upstream') {
      const credentials: Record<string, unknown> = {
        base_url: upstreamBaseUrl.value.trim(), api_key: upstreamApiKey.value.trim(),
      }
      const mapping = buildMapping()
      if (mapping) credentials.model_mapping = mapping
      applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
      applyTempUnschedToCredentials(credentials, tempUnschedEnabled.value, tempUnschedRules.value)
      return { credentials, extra: buildExtra(), common: commonFields.value, typeOverride: 'apikey' }
    }
    return { credentials: {}, extra: buildExtra(), common: commonFields.value, needsOAuthFlow: true }
  }

  function applyToCredentials(credentials: Record<string, unknown>) {
    const mapping = buildMapping()
    if (mapping) credentials.model_mapping = mapping
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
    applyTempUnschedToCredentials(credentials, tempUnschedEnabled.value, tempUnschedRules.value)
  }

  async function handleOAuthExchange(
    code: string, oauthState?: string
  ): Promise<SdkCreateAccountRequest | null> {
    if (!code.trim() || !antigravityOAuth.sessionId.value) return null
    const stateToUse = oauthState || antigravityOAuth.state.value
    if (!stateToUse) return null
    const tokenInfo = await antigravityOAuth.exchangeAuthCode({
      code: code.trim(), sessionId: antigravityOAuth.sessionId.value,
      state: stateToUse, proxyId: null,
    })
    if (!tokenInfo) return null
    const creds = antigravityOAuth.buildCredentials(tokenInfo)
    applyToCredentials(creds)
    return { name: '', platform: 'antigravity', type: 'oauth', credentials: creds, extra: buildExtra() }
  }

  async function handleRefreshToken(
    rt: string
  ): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null> {
    const tokens = rt.split('\n').map(s => s.trim()).filter(Boolean)
    if (tokens.length === 0) return null
    const results: SdkCreateAccountRequest[] = []
    for (const token of tokens) {
      const info = await antigravityOAuth.validateRefreshToken(token, null)
      if (!info) continue
      const creds = antigravityOAuth.buildCredentials(info)
      applyToCredentials(creds)
      results.push({ name: '', platform: 'antigravity', type: 'oauth', credentials: creds, extra: buildExtra() })
    }
    if (results.length === 1) return results[0]
    return results.length > 0 ? results : null
  }

  function initFromAccount(account: SdkAccount): void {
    editModeActive = true
    reset()
    doInit(account, formRefs)
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

  function getEditPayload(account: SdkAccount) {
    const payload = doEditPayload(account, formRefs)
    payload.common = commonFields.value
    return payload
  }

  function reset() {
    antigravityAccountType.value = 'oauth'
    upstreamBaseUrl.value = ''; upstreamApiKey.value = ''
    antigravityModelMappings.value = []
    mixedScheduling.value = false; allowOverages.value = false
    interceptWarmupRequests.value = false
    tempUnschedEnabled.value = false; tempUnschedRules.value = []
    commonFields.value = {
      name: '', notes: '',
      proxy_id: null, concurrency: 10, load_factor: null, priority: 1,
      rate_multiplier: 1, expires_at: null, auto_pause_on_expired: true,
      group_ids: [], quota_enabled: false, quota_limit: null,
      quota_daily_limit: null, quota_weekly_limit: null,
    }
    antigravityOAuth.resetState()
    fetchDefaultMappings().then(defaults => {
      if (!editModeActive && defaults.length > 0) antigravityModelMappings.value = defaults
    })
  }

  function addModelMapping() { antigravityModelMappings.value.push({ from: '', to: '' }) }
  function removeModelMapping(index: number) { antigravityModelMappings.value.splice(index, 1) }
  function addPresetMapping(from: string, to: string) {
    if (antigravityModelMappings.value.some(m => m.from === from)) return
    antigravityModelMappings.value.push({ from, to })
  }

  return {
    antigravityAccountType, upstreamBaseUrl, upstreamApiKey, editUpstreamApiKey,
    antigravityModelMappings, antigravityPresetMappings: presetMappings,
    mixedScheduling, allowOverages, interceptWarmupRequests,
    antigravityOAuth, oauthConfig, validate, getPayload, isOAuthFlow, reset,
    handleOAuthExchange, handleRefreshToken,
    addModelMapping, removeModelMapping, addPresetMapping, applyToCredentials,
    tempUnschedEnabled, tempUnschedRules, initFromAccount, getEditPayload,
  }
}
