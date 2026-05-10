import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'
import {
  buildModelMappingObject,
  isValidWildcardPattern,
  getPresetMappingsByPlatform,
  fetchAntigravityDefaultMappings
} from '@/composables/useModelWhitelist'
import { applyInterceptWarmup } from '@/components/account/credentialsBuilder'
import type { PlatformFormPayload, PlatformFormValidation, OAuthFlowConfig, EditFormPayload, ModelMapping } from './types'
import type { Account, CreateAccountRequest } from '@/types'
import * as editH from './editHelpers'

export function useAntigravityForm() {
  const { t } = useI18n()
  const appStore = useAppStore()
  const antigravityOAuth = useAntigravityOAuth()

  const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth')
  const upstreamBaseUrl = ref('')
  const upstreamApiKey = ref('')
  const antigravityModelMappings = ref<ModelMapping[]>([])
  const mixedScheduling = ref(false)
  const allowOverages = ref(false)
  const interceptWarmupRequests = ref(false)
  const tempUnschedEnabled = ref(false)
  const tempUnschedRules = ref<editH.TempUnschedRuleForm[]>([])

  const antigravityPresetMappings = computed(() => getPresetMappingsByPlatform('antigravity'))
  const isOAuthFlow = () => antigravityAccountType.value === 'oauth'
  const oauthConfig: OAuthFlowConfig = { showRefreshTokenOption: true, needsMixedChannelCheck: true, platform: 'antigravity' }

  function buildExtra(): Record<string, unknown> | undefined {
    const extra: Record<string, unknown> = {}
    if (mixedScheduling.value) extra.mixed_scheduling = true
    if (allowOverages.value) extra.allow_overages = true
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  function buildModelMapping(): Record<string, unknown> | undefined {
    return buildModelMappingObject('mapping', [], antigravityModelMappings.value) ?? undefined
  }

  function validate(): PlatformFormValidation {
    if (antigravityAccountType.value === 'upstream') {
      if (!upstreamBaseUrl.value.trim()) return { valid: false, error: t('admin.accounts.upstream.pleaseEnterBaseUrl') }
      if (!upstreamApiKey.value.trim()) return { valid: false, error: t('admin.accounts.upstream.pleaseEnterApiKey') }
    }
    for (const m of antigravityModelMappings.value) {
      if (m.from && !isValidWildcardPattern(m.from)) return { valid: false, error: t('admin.accounts.wildcardOnlyAtEnd') }
      if (m.to && m.to.includes('*')) return { valid: false, error: t('admin.accounts.targetNoWildcard') }
    }
    return { valid: true }
  }

  function getPayload(): PlatformFormPayload {
    if (antigravityAccountType.value === 'upstream') {
      const credentials: Record<string, unknown> = { base_url: upstreamBaseUrl.value.trim(), api_key: upstreamApiKey.value.trim() }
      const mapping = buildModelMapping(); if (mapping) credentials.model_mapping = mapping
      applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
      return { credentials, extra: buildExtra(), typeOverride: 'apikey' }
    }
    return { credentials: {}, extra: buildExtra(), needsOAuthFlow: true }
  }

  function applyToCredentials(credentials: Record<string, unknown>) {
    const mapping = buildModelMapping(); if (mapping) credentials.model_mapping = mapping
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
  }

  async function handleOAuthExchange(code: string, oauthState?: string): Promise<CreateAccountRequest | null> {
    if (!code.trim() || !antigravityOAuth.sessionId.value) return null
    const stateToUse = oauthState || antigravityOAuth.state.value; if (!stateToUse) return null
    const tokenInfo = await antigravityOAuth.exchangeAuthCode({ code: code.trim(), sessionId: antigravityOAuth.sessionId.value, state: stateToUse, proxyId: null })
    if (!tokenInfo) return null
    const creds = antigravityOAuth.buildCredentials(tokenInfo); applyToCredentials(creds)
    return { name: '', platform: 'antigravity', type: 'oauth', credentials: creds, extra: buildExtra() }
  }

  async function handleRefreshToken(rt: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null> {
    const tokens = rt.split('\n').map(s => s.trim()).filter(Boolean); if (tokens.length === 0) return null
    const results: CreateAccountRequest[] = []
    for (const token of tokens) {
      const info = await antigravityOAuth.validateRefreshToken(token, null); if (!info) continue
      const creds = antigravityOAuth.buildCredentials(info); applyToCredentials(creds)
      results.push({ name: '', platform: 'antigravity', type: 'oauth', credentials: creds, extra: buildExtra() })
    }
    return results.length === 1 ? results[0] : results.length > 0 ? results : null
  }

  // ---- Edit mode ----

  function initFromAccount(account: Account): void {
    reset()
    const credentials = account.credentials as Record<string, unknown> | undefined
    const extra = account.extra as Record<string, unknown> | undefined
    editH.loadInterceptWarmupFromCredentials(credentials, interceptWarmupRequests)
    editH.loadTempUnschedFromCredentials(credentials, tempUnschedEnabled, tempUnschedRules)
    if (account.type === 'upstream') { antigravityAccountType.value = 'upstream'; upstreamBaseUrl.value = (credentials?.base_url as string) || '' }
    mixedScheduling.value = extra?.mixed_scheduling === true
    allowOverages.value = extra?.allow_overages === true
    const rawMapping = credentials?.model_mapping as Record<string, string> | undefined
    if (rawMapping && typeof rawMapping === 'object') {
      antigravityModelMappings.value = Object.entries(rawMapping).map(([from, to]) => ({ from, to }))
    } else {
      const rawWhitelist = credentials?.model_whitelist
      if (Array.isArray(rawWhitelist) && rawWhitelist.length > 0) {
        antigravityModelMappings.value = rawWhitelist.map(v => String(v).trim()).filter(v => v.length > 0).map(m => ({ from: m, to: m }))
      } else { antigravityModelMappings.value = [] }
    }
  }

  function getEditPayload(account: Account): EditFormPayload {
    const currentCreds = (account.credentials as Record<string, unknown>) || {}
    const newCreds: Record<string, unknown> = { ...currentCreds }
    editH.applyInterceptWarmup(newCreds, interceptWarmupRequests.value, 'edit')
    editH.applyTempUnschedToCredentials(newCreds, tempUnschedEnabled.value, tempUnschedRules.value)
    if (account.type === 'upstream') newCreds.base_url = upstreamBaseUrl.value.trim()
    delete newCreds.model_whitelist; delete newCreds.model_mapping
    const mm = buildModelMapping(); if (mm) newCreds.model_mapping = mm
    const currentExtra = (account.extra as Record<string, unknown>) || {}
    const newExtra: Record<string, unknown> = { ...currentExtra }
    if (mixedScheduling.value) newExtra.mixed_scheduling = true; else delete newExtra.mixed_scheduling
    if (allowOverages.value) newExtra.allow_overages = true; else delete newExtra.allow_overages
    return { credentials: newCreds, extra: newExtra }
  }

  function reset() {
    antigravityAccountType.value = 'oauth'; upstreamBaseUrl.value = ''; upstreamApiKey.value = ''
    antigravityModelMappings.value = []; mixedScheduling.value = false; allowOverages.value = false
    interceptWarmupRequests.value = false; tempUnschedEnabled.value = false; tempUnschedRules.value = []
    antigravityOAuth.resetState()
    // Fire-and-forget default mappings fetch for create mode
    fetchAntigravityDefaultMappings().then(defaults => {
      if (defaults && defaults.length > 0) {
        antigravityModelMappings.value = defaults
      }
    })
  }

  function addModelMapping() { antigravityModelMappings.value.push({ from: '', to: '' }) }
  function removeModelMapping(index: number) { antigravityModelMappings.value.splice(index, 1) }
  function addPresetMapping(from: string, to: string) {
    if (antigravityModelMappings.value.some(m => m.from === from)) {
      appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
      return
    }
    antigravityModelMappings.value.push({ from, to })
  }

  return {
    antigravityAccountType, upstreamBaseUrl, upstreamApiKey,
    antigravityModelMappings, antigravityPresetMappings,
    mixedScheduling, allowOverages, interceptWarmupRequests,
    antigravityOAuth, oauthConfig,
    validate, getPayload, isOAuthFlow, reset,
    handleOAuthExchange, handleRefreshToken,
    addModelMapping, removeModelMapping, addPresetMapping,
    applyToCredentials,
    tempUnschedEnabled, tempUnschedRules,
    initFromAccount, getEditPayload
  }
}
