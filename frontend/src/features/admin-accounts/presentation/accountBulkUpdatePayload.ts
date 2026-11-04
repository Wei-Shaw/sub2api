import { openAIImageSizeTiersPayload } from './openaiImageSizeTiers'
import { buildHeaderOverridesObject, HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY, HEADER_OVERRIDES_CREDENTIAL_KEY } from './credentialsBuilder'
import { buildModelMappingObject } from './composables/useModelWhitelist'
import type { CodexFingerprintMode, ModelMapping } from './accountFormPolicy'
import { isOpenAIWSModeEnabled, type OpenAIWSMode } from '@/core/utils/openaiWsMode'
import type { OpenAICompactMode, OpenAIEndpointCapability, OpenAIResponsesMode } from '@/types'

export interface BulkAccountUpdatePayloadState {
  openAIImageSizeTiersUpdate?: string[] | null
  enableProxy: boolean
  proxyId: number | null
  enableConcurrency: boolean
  concurrency: number
  enableLoadFactor: boolean
  loadFactor: number | null
  enablePriority: boolean
  priority: number
  enableRateMultiplier: boolean
  rateMultiplier: number
  enableStatus: boolean
  status: 'active' | 'inactive'
  enableGroups: boolean
  groupIds: number[]
  enableBaseUrl: boolean
  baseUrl: string
  enableOpenAIPassthrough: boolean
  openaiPassthroughEnabled: boolean
  enableOpenAIFlattenNamespaces: boolean
  openaiFlattenNamespacesEligible: boolean
  openaiFlattenNamespacesEnabled: boolean
  excelBpsMode: 'unchanged' | 'enabled' | 'disabled'
  enableOpenAICustomRelay: boolean
  openAICustomRelayEnabled: boolean
  openAICustomRelayURL: string
  enableOpenAILongContextBilling: boolean
  openAILongContextBillingEnabled: boolean
  enableTLSFingerprint: boolean
  tlsFingerprintEnabled: boolean
  tlsFingerprintProfileId: number | null
  enableOpenAIEndpointCapabilities: boolean
  openAIEndpointCapabilities: OpenAIEndpointCapability[]
  enableOpenAIResponsesMode: boolean
  openAIResponsesMode: OpenAIResponsesMode
  enableModelRestriction: boolean
  isOpenAIModelRestrictionDisabled: boolean
  modelRestrictionMode: 'whitelist' | 'mapping'
  allowedModels: string[]
  modelMappings: ModelMapping[]
  enableCustomErrorCodes: boolean
  selectedErrorCodes: number[]
  enableInterceptWarmup: boolean
  interceptWarmupRequests: boolean
  enableHeaderOverride: boolean
  headerOverrideEnabled: boolean
  headerOverrideRows: Array<{ name: string; value: string }>
  enableOpenAIWSMode: boolean
  openaiOAuthResponsesWebSocketV2Mode: OpenAIWSMode
  enableOpenAIAPIKeyWSMode: boolean
  openaiAPIKeyResponsesWebSocketV2Mode: OpenAIWSMode
  enableCodexPrewarmContinuation: boolean
  codexPrewarmContinuationEnabled: boolean
  enableCodexFingerprintMode: boolean
  codexFingerprintMode: CodexFingerprintMode
  enableCodexThinkingTagNormalization: boolean
  codexThinkingTagNormalizationEnabled: boolean
  enableUpstreamBillingAutoProbe: boolean
  upstreamBillingAutoProbeMode: 'enabled' | 'disabled'
  enableCodexCLIOnly: boolean
  codexCLIOnlyEnabled: boolean
  enableCodexCLIOnlyAppServer: boolean
  codexCLIOnlyAppServerEnabled: boolean
  enableOpenAICompactMode: boolean
  openAICompactMode: OpenAICompactMode
  enableOpenAICompactModelMapping: boolean
  openAICompactModelMappings: ModelMapping[]
  enableRpmLimit: boolean
  rpmLimitEnabled: boolean
  bulkBaseRpm: number | null
  bulkRpmStrategy: 'tiered' | 'sticky_exempt'
  bulkRpmStickyBuffer: number | null
  userMsgQueueMode: string | null
  enableCPA: boolean
  cpaModeEnabled: boolean
  cpaUseBaseUrl: boolean
  cpaManagementUrl: string
  cpaManagementPassword: string
  cpaConcurrencyPerCredential: number
  cpaExcludeAbnormalCredentials: boolean
}

export function buildBulkAccountUpdatePayload(
  state: BulkAccountUpdatePayloadState,
): Record<string, unknown> | null {
  const updates: Record<string, unknown> = {}
  const credentials: Record<string, unknown> = {}
  let credentialsChanged = false
  const ensureExtra = (): Record<string, unknown> => {
    if (!updates.extra) updates.extra = {}
    return updates.extra as Record<string, unknown>
  }

  if (state.enableProxy) updates.proxy_id = state.proxyId === null ? 0 : state.proxyId
  if (state.enableConcurrency) updates.concurrency = state.concurrency
  if (state.enableLoadFactor) {
    updates.load_factor = state.loadFactor != null && !Number.isNaN(state.loadFactor) && state.loadFactor > 0
      ? state.loadFactor
      : 0
  }
  if (state.enablePriority) updates.priority = state.priority
  if (state.enableRateMultiplier) updates.rate_multiplier = state.rateMultiplier
  if (state.enableStatus) updates.status = state.status
  if (state.enableGroups) updates.group_ids = state.groupIds

  if (state.enableBaseUrl) {
    const baseUrl = state.baseUrl.trim()
    if (baseUrl) {
      credentials.base_url = baseUrl
      credentialsChanged = true
    }
  }

  if (state.enableOpenAIPassthrough) {
    const extra = ensureExtra()
    extra.openai_passthrough = state.openaiPassthroughEnabled
    if (!state.openaiPassthroughEnabled) extra.openai_oauth_passthrough = false
  }

  if (state.excelBpsMode !== 'unchanged') {
    updates.excel_bps_enabled = state.excelBpsMode === 'enabled'
  }

  if (state.enableOpenAIFlattenNamespaces && state.openaiFlattenNamespacesEligible) {
    ensureExtra().openai_responses_flatten_namespaces = state.openaiFlattenNamespacesEnabled
  }

  if (state.enableOpenAICustomRelay) {
    const extra = ensureExtra()
    extra.custom_base_url_enabled = state.openAICustomRelayEnabled
    extra.custom_base_url = state.openAICustomRelayEnabled
      ? state.openAICustomRelayURL.trim()
      : null
  }

  if (state.enableOpenAILongContextBilling) {
    ensureExtra().openai_long_context_billing_enabled = state.openAILongContextBillingEnabled
  }
  if (state.enableTLSFingerprint) {
    const extra = ensureExtra()
    extra.enable_tls_fingerprint = state.tlsFingerprintEnabled
    // Bulk enabling defaults to the stable account-assignment sentinel so a
    // large batch does not collapse onto one shared ClientHello profile.
    extra.tls_fingerprint_profile_id = state.tlsFingerprintEnabled
      ? state.tlsFingerprintProfileId
      : null
  }
  if (state.enableOpenAIEndpointCapabilities) {
    credentials.openai_capabilities = state.openAIEndpointCapabilities.filter(
      capability => capability === 'chat_completions' || capability === 'embeddings' || capability === 'decisions'
    )
    credentialsChanged = true
  }
  if (state.enableOpenAIResponsesMode) {
    const extra = ensureExtra()
    extra.openai_responses_mode = state.openAIResponsesMode === 'auto' ? null : state.openAIResponsesMode
  }

  if (state.enableModelRestriction && !state.isOpenAIModelRestrictionDisabled) {
    credentials.model_mapping = state.modelRestrictionMode === 'whitelist'
      ? Object.fromEntries(state.allowedModels.map((model) => [model, model]))
      : buildModelMappingObject('mapping', [], state.modelMappings) ?? {}
    credentialsChanged = true
  }

  if (state.enableCustomErrorCodes) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...state.selectedErrorCodes]
    credentialsChanged = true
  }
  if (state.enableInterceptWarmup) {
    credentials.intercept_warmup_requests = state.interceptWarmupRequests
    credentialsChanged = true
  }
  if (state.enableHeaderOverride) {
    credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] = state.headerOverrideEnabled
    credentials[HEADER_OVERRIDES_CREDENTIAL_KEY] = state.headerOverrideEnabled
      ? buildHeaderOverridesObject(state.headerOverrideRows)
      : {}
    credentialsChanged = true
  }

  if (state.enableCPA) {
    credentials.cpa_mode = state.cpaModeEnabled
    credentialsChanged = true
    if (state.cpaModeEnabled) {
      credentials.cpa_management_url = state.cpaUseBaseUrl
        ? null
        : state.cpaManagementUrl.trim().replace(/\/+$/, '')
      credentials.cpa_concurrency_per_credential = Math.trunc(state.cpaConcurrencyPerCredential)
      credentials.cpa_exclude_abnormal_credentials = state.cpaExcludeAbnormalCredentials
      const password = state.cpaManagementPassword.trim()
      if (password) credentials.cpa_management_key = password
    }
  }

  if (state.openAIImageSizeTiersUpdate != null) {
    ensureExtra().openai_image_size_tiers = openAIImageSizeTiersPayload(state.openAIImageSizeTiersUpdate) || []
  }

  if (state.enableOpenAIWSMode) {
    const extra = ensureExtra()
    extra.openai_oauth_responses_websockets_v2_mode = state.openaiOAuthResponsesWebSocketV2Mode
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(
      state.openaiOAuthResponsesWebSocketV2Mode,
    )
  }
  if (state.enableOpenAIAPIKeyWSMode) {
    const extra = ensureExtra()
    extra.openai_apikey_responses_websockets_v2_mode = state.openaiAPIKeyResponsesWebSocketV2Mode
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(
      state.openaiAPIKeyResponsesWebSocketV2Mode,
    )
  }
  if (state.enableCodexPrewarmContinuation) {
    ensureExtra().codex_prewarm_continuation_enabled =
      state.codexPrewarmContinuationEnabled
  }
  if (state.enableCodexFingerprintMode) {
    // Bulk updates are partial merges, so "off" must be explicit to disable
    // an already-enabled account.
    ensureExtra().codex_fingerprint_mode = state.codexFingerprintMode
  }
  if (state.enableCodexThinkingTagNormalization) {
    ensureExtra().codex_thinking_tag_normalization_enabled = state.codexThinkingTagNormalizationEnabled
  }
  if (state.enableUpstreamBillingAutoProbe) {
    updates.upstream_billing_probe_enabled = state.upstreamBillingAutoProbeMode === 'enabled'
  }
  if (state.enableCodexCLIOnly) ensureExtra().codex_cli_only = state.codexCLIOnlyEnabled
  if (
    state.enableCodexCLIOnlyAppServer &&
    state.enableCodexCLIOnly &&
    state.codexCLIOnlyEnabled
  ) {
    ensureExtra().codex_cli_only_allow_app_server = state.codexCLIOnlyAppServerEnabled
  }
  if (state.enableOpenAICompactMode) ensureExtra().openai_compact_mode = state.openAICompactMode
  if (state.enableOpenAICompactModelMapping) {
    credentials.compact_model_mapping = buildModelMappingObject(
      'mapping',
      [],
      state.openAICompactModelMappings,
    ) ?? {}
    credentialsChanged = true
  }

  if (state.enableRpmLimit) {
    const extra = ensureExtra()
    if (state.rpmLimitEnabled && state.bulkBaseRpm != null && state.bulkBaseRpm > 0) {
      extra.base_rpm = state.bulkBaseRpm
      extra.rpm_strategy = state.bulkRpmStrategy
      if (state.bulkRpmStickyBuffer != null && state.bulkRpmStickyBuffer > 0) {
        extra.rpm_sticky_buffer = state.bulkRpmStickyBuffer
      }
    } else {
      extra.base_rpm = 0
      extra.rpm_strategy = ''
      extra.rpm_sticky_buffer = 0
    }
  }
  if (state.userMsgQueueMode !== null) {
    const extra = ensureExtra()
    extra.user_msg_queue_mode = state.userMsgQueueMode
    extra.user_msg_queue_enabled = false
  }
  if (credentialsChanged) updates.credentials = credentials
  return Object.keys(updates).length > 0 ? updates : null
}
