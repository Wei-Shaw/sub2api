import { applyOpenAIImageSizeTiersToExtra, type OpenAIImageSizeTier } from './openaiImageSizeTiers'
import { supportsAccountQuota } from '@/features/admin-accounts/presentation/accountQuota'
import type { ComputedRef, Ref } from 'vue'
import type { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  cnSupportsNativeResponses,
  defaultCNAdaptiveBaseURLs,
  normalizeOpenCodeProtocolRules,
} from '@/core/constants/account'
import type { OpenAIWSMode } from '@/core/utils/openaiWsMode'
import { isOpenAIWSModeEnabled } from '@/core/utils/openaiWsMode'
import {
  applyAntigravityProjectID,
  applyHeaderOverride,
  applyInterceptWarmup,
  applyPlanType,
  isHeaderOverrideCapable,
  validateHeaderOverrideRows,
} from './credentialsBuilder'
import {
  normalizePoolModeRetryCount,
  parsePoolModeRetryStatusCodes,
} from './accountFormPolicy'
import type {
  EditAccountAdvancedContext,
  EditAccountCredentialContext,
  EditAccountPolicyContext,
} from './accountEditorContext'
import { buildModelMappingObject } from './composables/useModelWhitelist'
import type { useQuotaNotifyState } from './composables/useQuotaNotifyState'
import { isUpstreamBillingProbeEligible } from './upstreamBillingProbeEligibility'

type UpdatePayload = Record<string, unknown>

type EditorFields =
  Pick<
    EditAccountCredentialContext,
    | 'antigravityModelMappings'
    | 'antigravityProjectId'
    | 'autoDisableOnUpstreamInsufficientBalance'
    | 'cpaConcurrencyPerCredential'
    | 'cpaExcludeAbnormalCredentials'
    | 'cpaManagementKey'
    | 'cpaManagementUrl'
    | 'cpaModeEnabled'
    | 'cpaUseBaseUrl'
    | 'customErrorCodesEnabled'
    | 'editApiKey'
    | 'editBaseUrl'
    | 'editCNAccountMode'
    | 'editCNAPIProtocol'
    | 'editCNAdaptiveBaseURLs'
    | 'editOpenCodeAccountMode'
    | 'editOpenCodeProtocolRules'
    | 'editZhipuOrganization'
    | 'editZhipuProject'
    | 'editBedrockAccessKeyId'
    | 'editBedrockApiKeyValue'
    | 'editBedrockForceGlobal'
    | 'editBedrockRegion'
    | 'editBedrockSecretAccessKey'
    | 'editBedrockSessionToken'
    | 'editVertexLocation'
    | 'editVertexProjectId'
    | 'form'
    | 'grokClientToolCacheEnabled'
    | 'grokOAuthBaseUrl'
    | 'grokOAuthCustomBaseUrlEnabled'
    | 'headerOverrideEnabled'
    | 'headerOverrideRows'
    | 'isBedrockAPIKeyMode'
    | 'poolModeEnabled'
    | 'poolModeRetryCount'
    | 'poolModeRetryStatusCodesInput'
    | 'selectedErrorCodes'
  > &
  Pick<
    EditAccountAdvancedContext,
    | 'anthropicAPIKeyAuthScheme'
    | 'anthropicPassthroughEnabled'
    | 'codexFingerprintMode'
    | 'codexPrewarmContinuationEnabled'
    | 'codexThinkingTagNormalizationEnabled'
    | 'codexCLIOnlyAppServerEnabled'
    | 'codexCLIOnlyEnabled'
    | 'codexImageToolMode'
    | 'editDailyResetHour'
    | 'editDailyResetMode'
    | 'editQuotaDailyLimit'
    | 'editQuotaLimit'
    | 'editQuotaWeeklyLimit'
    | 'editResetTimezone'
    | 'editWeeklyResetDay'
    | 'editWeeklyResetHour'
    | 'editWeeklyResetMode'
    | 'interceptWarmupRequests'
    | 'isOpenAIPersonalAccessTokenAccount'
    | 'isSparkShadow'
    | 'openAIForceImageAPIEnabled'
    | 'openAILongContextBillingEnabled'
    | 'openAIResponsesMode'
    | 'openAITextGenerationCapabilityEnabled'
    | 'openaiFlattenNamespacesEnabled'
    | 'openaiPassthroughEnabled'
    | 'upstreamBillingAutoProbeEnabled'
    | 'upstreamBillingRateSyncEnabled'
    | 'webSearchEmulationMode'
  > &
  Pick<
    EditAccountPolicyContext,
    | 'allowOverages'
    | 'autoPause5hDisabled'
    | 'autoPause5hThreshold'
    | 'autoPause7dDisabled'
    | 'autoPause7dThreshold'
    | 'autoPauseOnExpired'
    | 'baseRpm'
    | 'cacheTTLOverrideEnabled'
    | 'cacheTTLOverrideTarget'
    | 'customBaseUrl'
    | 'customBaseUrlEnabled'
    | 'editPlanType'
    | 'excelBpsEnabled'
    | 'maxSessions'
    | 'mixedScheduling'
    | 'openAICompactMode'
    | 'openAICompactModelMappings'
    | 'rpmLimitEnabled'
    | 'rpmStickyBuffer'
    | 'rpmStrategy'
    | 'sessionIdMaskingEnabled'
    | 'sessionIdleTimeout'
    | 'sessionLimitEnabled'
    | 'tlsFingerprintEnabled'
    | 'tlsFingerprintProfileId'
    | 'userMsgQueueMode'
    | 'windowCostEnabled'
    | 'windowCostLimit'
    | 'windowCostStickyReserve'
  >

export interface EditAccountUpdatePayloadContext extends EditorFields {
  openAIImageSizeTiers?: Ref<OpenAIImageSizeTier[]>
  applyCodexWebSearchCapability: (credentials: UpdatePayload) => void
  applyOpenAIEndpointCapabilities: (credentials: UpdatePayload) => void
  applyOpenAIModelMappingCredentials: (credentials: UpdatePayload) => void
  applyTempUnschedConfig: (credentials: UpdatePayload) => boolean
  buildModelRestrictionMapping: () => Record<string, string> | null
  defaultBaseUrl: ComputedRef<string>
  editVertexClientEmail: Ref<string>
  grokClientToolCacheExtraKey: string
  maxCPAConcurrencyPerCredential: number
  notifications: {
    showError: (message: string) => void
  }
  openaiAPIKeyResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  openaiOAuthResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  t: ReturnType<typeof useI18n>['t']
  writeQuotaNotifyToExtra: ReturnType<typeof useQuotaNotifyState>['writeToExtra']
}

function applyPoolModeCredentials(
  credentials: UpdatePayload,
  context: EditAccountUpdatePayloadContext,
): void {
  if (context.poolModeEnabled.value) {
    credentials.pool_mode = true
    credentials.pool_mode_retry_count = normalizePoolModeRetryCount(
      context.poolModeRetryCount.value,
    )
    const statusCodes = parsePoolModeRetryStatusCodes(
      context.poolModeRetryStatusCodesInput.value,
    )
    if (statusCodes.length > 0) {
      credentials.pool_mode_retry_status_codes = statusCodes
    } else {
      delete credentials.pool_mode_retry_status_codes
    }
    return
  }

  delete credentials.pool_mode
  delete credentials.pool_mode_retry_count
  delete credentials.pool_mode_retry_status_codes
}

function applyModelRestriction(
  credentials: UpdatePayload,
  context: EditAccountUpdatePayloadContext,
): void {
  const modelMapping = context.buildModelRestrictionMapping()
  if (modelMapping) {
    credentials.model_mapping = modelMapping
  } else {
    delete credentials.model_mapping
  }
}

function buildAPIKeyCredentials(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  // 多模态平台没有内置上游端点，Base URL 必须由管理员填写
  if (account.platform === 'multimodal' && !context.editBaseUrl.value.trim()) {
    context.notifications.showError(context.t('admin.accounts.pleaseEnterBaseUrl'))
    return null
  }
  const currentCredentials =
    (account.credentials as UpdatePayload | undefined) || {}
  const credentials: UpdatePayload = {
    ...currentCredentials,
    base_url: context.editBaseUrl.value.trim() || context.defaultBaseUrl.value,
  }

  const isCNProvider = ['kimi', 'zhipu', 'deepseek', 'minimax'].includes(account.platform)
  if (isCNProvider || account.platform === 'opencode_go') {
    const mode = account.platform === 'opencode_go'
      ? context.editOpenCodeAccountMode.value
      : context.editCNAccountMode.value
    const protocol = account.platform === 'opencode_go' ? 'adaptive' : context.editCNAPIProtocol.value
    credentials.account_mode = mode
    credentials.api_protocol = protocol
    if (protocol === 'adaptive') {
      const defaults = defaultCNAdaptiveBaseURLs(account.platform as 'kimi' | 'zhipu' | 'deepseek' | 'minimax' | 'opencode_go', mode)
      const apiBaseURLs: Record<string, string> = {
        chat_completions: context.editCNAdaptiveBaseURLs.value.chat_completions.trim() || defaults.chat_completions,
        anthropic: context.editCNAdaptiveBaseURLs.value.anthropic.trim() || defaults.anthropic,
      }
      if (cnSupportsNativeResponses(account.platform)) {
        apiBaseURLs.responses = context.editCNAdaptiveBaseURLs.value.responses.trim() || defaults.responses
      }
      credentials.api_base_urls = apiBaseURLs
      credentials.base_url = apiBaseURLs.chat_completions
    } else {
      delete credentials.api_base_urls
    }
    if (account.platform === 'opencode_go') {
      credentials.protocol_rules = normalizeOpenCodeProtocolRules(context.editOpenCodeProtocolRules.value)
    }
    if (account.platform === 'zhipu') {
      const organization = context.editZhipuOrganization.value.trim()
      const project = context.editZhipuProject.value.trim()
      if (mode === 'coding' && organization) {
        credentials.zhipu_organization = organization
        if (project) credentials.zhipu_project = project
        else delete credentials.zhipu_project
      } else {
        delete credentials.zhipu_organization
        delete credentials.zhipu_project
      }
    }
  }

  const hasExistingAPIKey =
    account.credentials_status?.has_api_key ??
    Boolean(currentCredentials.api_key)
  if (context.editApiKey.value.trim()) {
    credentials.api_key = context.editApiKey.value.trim()
  } else if (!hasExistingAPIKey) {
    context.notifications.showError(context.t('admin.accounts.apiKeyIsRequired'))
    return null
  }

  const shouldApplyModelMapping = !(
    account.platform === 'openai' && context.openaiPassthroughEnabled.value
  )
  if (shouldApplyModelMapping) {
    applyModelRestriction(credentials, context)
  } else if (currentCredentials.model_mapping) {
    credentials.model_mapping = currentCredentials.model_mapping
  }

  if (account.platform === 'openai') {
    context.applyOpenAIEndpointCapabilities(credentials)
    const compactModelMapping = buildModelMappingObject(
      'mapping',
      [],
      context.openAICompactModelMappings.value,
    )
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    } else {
      delete credentials.compact_model_mapping
    }
  }

  if (context.cpaModeEnabled.value) {
    const managementURL = context.cpaManagementUrl.value
      .trim()
      .replace(/\/+$/, '')
    if (!context.cpaUseBaseUrl.value && !managementURL) {
      context.notifications.showError(
        context.t('admin.accounts.cpaManagementUrlRequired'),
      )
      return null
    }
    const hasExistingManagementKey =
      account.credentials_status?.has_cpa_management_key ?? false
    const managementKey = context.cpaManagementKey.value.trim()
    if (!managementKey && !hasExistingManagementKey) {
      context.notifications.showError(
        context.t('admin.accounts.cpaManagementKeyRequired'),
      )
      return null
    }
    const perCredential = Math.trunc(
      Number(context.cpaConcurrencyPerCredential.value),
    )
    if (
      !Number.isFinite(perCredential) ||
      perCredential < 1 ||
      perCredential > context.maxCPAConcurrencyPerCredential
    ) {
      context.notifications.showError(
        context.t('admin.accounts.cpaConcurrencyInvalid', {
          max: context.maxCPAConcurrencyPerCredential,
        }),
      )
      return null
    }
    credentials.cpa_mode = true
    if (context.cpaUseBaseUrl.value) {
      delete credentials.cpa_management_url
    } else {
      credentials.cpa_management_url = managementURL
    }
    credentials.cpa_concurrency_per_credential = perCredential
    credentials.cpa_exclude_abnormal_credentials = context.cpaExcludeAbnormalCredentials.value
    if (managementKey) credentials.cpa_management_key = managementKey
  } else {
    credentials.cpa_mode = false
    credentials.cpa_management_key = ''
    delete credentials.cpa_management_url
    delete credentials.cpa_concurrency_per_credential
    delete credentials.cpa_exclude_abnormal_credentials
  }

  applyPoolModeCredentials(credentials, context)

  if (context.customErrorCodesEnabled.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...context.selectedErrorCodes.value]
  } else {
    delete credentials.custom_error_codes_enabled
    delete credentials.custom_error_codes
  }

  if (isHeaderOverrideCapable(account.platform, 'apikey')) {
    if (context.headerOverrideEnabled.value) {
      const headerError = validateHeaderOverrideRows(
        context.headerOverrideRows.value,
      )
      if (headerError) {
        context.notifications.showError(
          context.t(`admin.accounts.headerOverride.${headerError}`),
        )
        return null
      }
    }
    applyHeaderOverride(
      credentials,
      context.headerOverrideEnabled.value,
      context.headerOverrideRows.value,
      'edit',
    )
  }

  applyInterceptWarmup(
    credentials,
    context.interceptWarmupRequests.value,
    'edit',
  )
  return context.applyTempUnschedConfig(credentials) ? credentials : null
}

function buildUpstreamCredentials(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  const credentials: UpdatePayload = {
    ...((account.credentials as UpdatePayload | undefined) || {}),
    base_url: context.editBaseUrl.value.trim(),
  }
  if (context.editApiKey.value.trim()) {
    credentials.api_key = context.editApiKey.value.trim()
  }
  applyInterceptWarmup(
    credentials,
    context.interceptWarmupRequests.value,
    'edit',
  )
  return context.applyTempUnschedConfig(credentials) ? credentials : null
}

function buildServiceAccountCredentials(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  if (!context.editVertexProjectId.value.trim()) {
    context.notifications.showError(
      context.t('admin.accounts.vertexSaJsonMissingProjectId'),
    )
    return null
  }
  if (!context.editVertexClientEmail.value.trim()) {
    context.notifications.showError(
      context.t('admin.accounts.vertexSaJsonMissingClientEmail'),
    )
    return null
  }
  if (!context.editVertexLocation.value.trim()) {
    context.notifications.showError(
      context.t('admin.accounts.vertexLocationRequired'),
    )
    return null
  }

  const currentCredentials =
    (account.credentials as UpdatePayload | undefined) || {}
  const status = account.credentials_status
  const hasExistingServiceAccountJSON = status
    ? Boolean(status.has_service_account_json || status.has_service_account)
    : Boolean(
        currentCredentials.service_account_json ||
          currentCredentials.service_account,
      )
  if (!hasExistingServiceAccountJSON) {
    context.notifications.showError(
      context.t('admin.accounts.vertexSaJsonRequired'),
    )
    return null
  }

  const credentials: UpdatePayload = {
    ...currentCredentials,
    project_id: context.editVertexProjectId.value.trim(),
    client_email: context.editVertexClientEmail.value.trim(),
    location: context.editVertexLocation.value.trim(),
    tier_id: 'vertex',
  }
  applyModelRestriction(credentials, context)
  applyInterceptWarmup(
    credentials,
    context.interceptWarmupRequests.value,
    'edit',
  )
  return context.applyTempUnschedConfig(credentials) ? credentials : null
}

function buildBedrockCredentials(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  const credentials: UpdatePayload = {
    ...((account.credentials as UpdatePayload | undefined) || {}),
    aws_region: context.editBedrockRegion.value.trim(),
  }
  if (context.editBedrockForceGlobal.value) {
    credentials.aws_force_global = 'true'
  } else {
    delete credentials.aws_force_global
  }

  if (context.isBedrockAPIKeyMode.value) {
    if (context.editBedrockApiKeyValue.value.trim()) {
      credentials.api_key = context.editBedrockApiKeyValue.value.trim()
    }
  } else {
    credentials.aws_access_key_id = context.editBedrockAccessKeyId.value.trim()
    if (context.editBedrockSecretAccessKey.value.trim()) {
      credentials.aws_secret_access_key =
        context.editBedrockSecretAccessKey.value.trim()
    }
    if (context.editBedrockSessionToken.value.trim()) {
      credentials.aws_session_token = context.editBedrockSessionToken.value.trim()
    }
  }

  applyPoolModeCredentials(credentials, context)
  applyModelRestriction(credentials, context)
  applyInterceptWarmup(
    credentials,
    context.interceptWarmupRequests.value,
    'edit',
  )
  return context.applyTempUnschedConfig(credentials) ? credentials : null
}

function buildDefaultCredentials(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  const credentials: UpdatePayload = {
    ...((account.credentials as UpdatePayload | undefined) || {}),
  }
  applyInterceptWarmup(
    credentials,
    context.interceptWarmupRequests.value,
    'edit',
  )
  return context.applyTempUnschedConfig(credentials) ? credentials : null
}

function applyAccountTypeCredentials(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): boolean {
  let credentials: UpdatePayload | null
  if (account.type === 'apikey') {
    credentials = buildAPIKeyCredentials(account, context)
  } else if (account.type === 'upstream') {
    credentials = buildUpstreamCredentials(account, context)
  } else if (
    (account.platform === 'gemini' || account.platform === 'anthropic') &&
    account.type === 'service_account'
  ) {
    credentials = buildServiceAccountCredentials(account, context)
  } else if (account.type === 'bedrock') {
    credentials = buildBedrockCredentials(account, context)
  } else {
    credentials = buildDefaultCredentials(account, context)
  }
  if (!credentials) return false
  payload.credentials = credentials
  return true
}

function applyOAuthModelCredentials(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (
    (account.platform !== 'openai' && account.platform !== 'grok') ||
    account.type !== 'oauth'
  ) {
    return
  }
  const currentCredentials = context.isSparkShadow.value
    ? {}
    : (payload.credentials as UpdatePayload | undefined) ||
      ((account.credentials as UpdatePayload | undefined) || {})
  const credentials = { ...currentCredentials }
  if (account.platform === 'openai') {
    context.applyOpenAIModelMappingCredentials(credentials)
    if (context.isOpenAIPersonalAccessTokenAccount.value) {
      context.applyCodexWebSearchCapability(credentials)
    }
  } else {
    applyModelRestriction(credentials, context)
  }
  payload.credentials = credentials
}

function applyGrokOAuthSettings(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): boolean {
  if (account.platform !== 'grok' || account.type !== 'oauth') return true

  const credentials: UpdatePayload = {
    ...((payload.credentials as UpdatePayload | undefined) ||
      (account.credentials as UpdatePayload | undefined) ||
      {}),
  }
  if (context.grokOAuthCustomBaseUrlEnabled.value) {
    const baseURL = context.grokOAuthBaseUrl.value.trim()
    if (!baseURL) {
      context.notifications.showError(
        context.t('admin.accounts.grokCustomBaseUrl.required'),
      )
      return false
    }
    if (!/^https?:\/\//i.test(baseURL)) {
      context.notifications.showError(
        context.t('admin.accounts.grokCustomBaseUrl.invalid'),
      )
      return false
    }
    credentials.base_url = baseURL
  } else {
    delete credentials.base_url
  }

  if (context.headerOverrideEnabled.value) {
    const headerError = validateHeaderOverrideRows(context.headerOverrideRows.value)
    if (headerError) {
      context.notifications.showError(
        context.t(`admin.accounts.headerOverride.${headerError}`),
      )
      return false
    }
  }
  applyHeaderOverride(
    credentials,
    context.headerOverrideEnabled.value,
    context.headerOverrideRows.value,
    'edit',
  )
  payload.credentials = credentials

  payload.extra = {
    ...((account.extra as UpdatePayload | undefined) || {}),
    [context.grokClientToolCacheExtraKey]:
      context.grokClientToolCacheEnabled.value,
  }
  return true
}

function applyOpenAIPlanType(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (
    account.platform !== 'openai' ||
    account.type !== 'oauth' ||
    context.isSparkShadow.value
  ) {
    return
  }
  const credentials =
    (payload.credentials as UpdatePayload | undefined) ||
    ((account.credentials as UpdatePayload | undefined) || {})
  payload.credentials = applyPlanType(
    { ...credentials },
    context.editPlanType.value,
  )
}

function applyAntigravityCredentials(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (account.platform !== 'antigravity') return
  const credentials: UpdatePayload = {
    ...((payload.credentials as UpdatePayload | undefined) ||
      (account.credentials as UpdatePayload | undefined) ||
      {}),
  }
  if (account.type === 'oauth') {
    applyAntigravityProjectID(
      credentials,
      context.antigravityProjectId.value,
      'edit',
    )
  }
  delete credentials.model_whitelist
  delete credentials.model_mapping
  const mapping = buildModelMappingObject(
    'mapping',
    [],
    context.antigravityModelMappings.value,
  )
  if (mapping) credentials.model_mapping = mapping
  payload.credentials = credentials
}

function applyAntigravityExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (account.platform !== 'antigravity') return
  const extra: UpdatePayload = {
    ...((account.extra as UpdatePayload | undefined) || {}),
  }
  if (context.mixedScheduling.value) extra.mixed_scheduling = true
  else delete extra.mixed_scheduling
  if (context.allowOverages.value) extra.allow_overages = true
  else delete extra.allow_overages
  payload.extra = extra
}

function applyAnthropicOAuthExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (
    account.platform !== 'anthropic' ||
    (account.type !== 'oauth' && account.type !== 'setup-token')
  ) {
    return
  }
  const extra: UpdatePayload = {
    ...((payload.extra as UpdatePayload | undefined) ||
      (account.extra as UpdatePayload | undefined) ||
      {}),
  }

  if (
    context.windowCostEnabled.value &&
    context.windowCostLimit.value != null &&
    context.windowCostLimit.value > 0
  ) {
    extra.window_cost_limit = context.windowCostLimit.value
    extra.window_cost_sticky_reserve =
      context.windowCostStickyReserve.value ?? 10
  } else {
    delete extra.window_cost_limit
    delete extra.window_cost_sticky_reserve
  }

  if (
    context.sessionLimitEnabled.value &&
    context.maxSessions.value != null &&
    context.maxSessions.value > 0
  ) {
    extra.max_sessions = context.maxSessions.value
    extra.session_idle_timeout_minutes = context.sessionIdleTimeout.value ?? 5
  } else {
    delete extra.max_sessions
    delete extra.session_idle_timeout_minutes
  }

  if (context.rpmLimitEnabled.value) {
    extra.base_rpm =
      context.baseRpm.value != null && context.baseRpm.value > 0
        ? context.baseRpm.value
        : 15
    extra.rpm_strategy = context.rpmStrategy.value
    if (
      context.rpmStickyBuffer.value != null &&
      context.rpmStickyBuffer.value > 0
    ) {
      extra.rpm_sticky_buffer = context.rpmStickyBuffer.value
    } else {
      delete extra.rpm_sticky_buffer
    }
  } else {
    delete extra.base_rpm
    delete extra.rpm_strategy
    delete extra.rpm_sticky_buffer
  }

  if (context.userMsgQueueMode.value) {
    extra.user_msg_queue_mode = context.userMsgQueueMode.value
  } else {
    delete extra.user_msg_queue_mode
  }
  delete extra.user_msg_queue_enabled

  if (context.tlsFingerprintEnabled.value) {
    extra.enable_tls_fingerprint = true
    if (context.tlsFingerprintProfileId.value) {
      extra.tls_fingerprint_profile_id = context.tlsFingerprintProfileId.value
    } else {
      delete extra.tls_fingerprint_profile_id
    }
  } else {
    delete extra.enable_tls_fingerprint
    delete extra.tls_fingerprint_profile_id
  }

  if (context.sessionIdMaskingEnabled.value) {
    extra.session_id_masking_enabled = true
  } else {
    delete extra.session_id_masking_enabled
  }

  if (context.cacheTTLOverrideEnabled.value) {
    extra.cache_ttl_override_enabled = true
    extra.cache_ttl_override_target = context.cacheTTLOverrideTarget.value
  } else {
    delete extra.cache_ttl_override_enabled
    delete extra.cache_ttl_override_target
  }

  if (
    context.customBaseUrlEnabled.value &&
    context.customBaseUrl.value.trim()
  ) {
    extra.custom_base_url_enabled = true
    extra.custom_base_url = context.customBaseUrl.value.trim()
  } else {
    delete extra.custom_base_url_enabled
    delete extra.custom_base_url
  }
  payload.extra = extra
}

function applyAnthropicAPIKeyExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if ((account.platform !== 'anthropic' && account.platform !== 'openai') || account.type !== 'apikey') return
  const extra: UpdatePayload = {
    ...((payload.extra as UpdatePayload | undefined) ||
      (account.extra as UpdatePayload | undefined) ||
      {}),
  }
  if (account.platform === 'anthropic') {
    if (context.anthropicPassthroughEnabled.value) {
      extra.anthropic_passthrough = true
    } else {
      delete extra.anthropic_passthrough
    }
    if (context.anthropicAPIKeyAuthScheme.value === 'authorization_bearer') {
      extra.anthropic_apikey_auth_scheme = 'authorization_bearer'
    } else {
      delete extra.anthropic_apikey_auth_scheme
    }
  }
  if (context.webSearchEmulationMode.value === 'default') {
    delete extra.web_search_emulation
  } else {
    extra.web_search_emulation = context.webSearchEmulationMode.value
  }
  payload.extra = extra
}

function applyOpenAIAutoPauseExtra(
  extra: UpdatePayload,
  context: EditAccountUpdatePayloadContext,
): void {
  if (
    context.autoPause5hThreshold.value != null &&
    context.autoPause5hThreshold.value > 0
  ) {
    extra.auto_pause_5h_threshold = context.autoPause5hThreshold.value / 100
  } else {
    delete extra.auto_pause_5h_threshold
  }
  if (
    context.autoPause7dThreshold.value != null &&
    context.autoPause7dThreshold.value > 0
  ) {
    extra.auto_pause_7d_threshold = context.autoPause7dThreshold.value / 100
  } else {
    delete extra.auto_pause_7d_threshold
  }
  if (context.autoPause5hDisabled.value) extra.auto_pause_5h_disabled = true
  else delete extra.auto_pause_5h_disabled
  if (context.autoPause7dDisabled.value) extra.auto_pause_7d_disabled = true
  else delete extra.auto_pause_7d_disabled
}

function applyCodexImageToolExtra(
  extra: UpdatePayload,
  context: EditAccountUpdatePayloadContext,
): void {
  delete extra.codex_image_generation_bridge_enabled
  switch (context.codexImageToolMode.value) {
    case 'enabled':
    case 'disabled':
      extra.codex_image_generation_bridge =
        context.codexImageToolMode.value === 'enabled'
      delete extra.codex_image_generation_explicit_tool_policy
      break
    case 'block':
      extra.codex_image_generation_explicit_tool_policy = 'strip'
      delete extra.codex_image_generation_bridge
      break
    default:
      delete extra.codex_image_generation_bridge
      delete extra.codex_image_generation_explicit_tool_policy
  }
}

function applyOpenAIExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (
    account.platform !== 'openai' ||
    !['oauth', 'setup-token', 'apikey'].includes(account.type)
  ) {
    return
  }
  const currentExtra = (account.extra as UpdatePayload | undefined) || {}
  const extra: UpdatePayload = { ...currentExtra }
  const hadCodexCLIOnlyEnabled = currentExtra.codex_cli_only === true

  if (account.type === 'oauth' || account.type === 'setup-token') {
    extra.openai_oauth_responses_websockets_v2_mode =
      context.openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled =
      isOpenAIWSModeEnabled(
        context.openaiOAuthResponsesWebSocketV2Mode.value,
      )
  } else {
    extra.openai_apikey_responses_websockets_v2_mode =
      context.openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled =
      isOpenAIWSModeEnabled(
        context.openaiAPIKeyResponsesWebSocketV2Mode.value,
      )
  }
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled

  // OpenAI OAuth/Codex model traffic may use a custom relay. Keep the value
  // in account extra while leaving setup-token/API-key accounts unchanged.
  if (account.type === 'oauth') {
    if (
      context.customBaseUrlEnabled.value &&
      context.customBaseUrl.value.trim()
    ) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = context.customBaseUrl.value.trim()
    } else {
      delete extra.custom_base_url_enabled
      delete extra.custom_base_url
    }
  }

  if (context.openaiPassthroughEnabled.value) {
    extra.openai_passthrough = true
  } else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  }
  if (account.type === 'oauth' && context.openaiFlattenNamespacesEnabled.value) {
    extra.openai_responses_flatten_namespaces = true
  } else {
    delete extra.openai_responses_flatten_namespaces
  }
  if (account.type === 'oauth') {
    if (context.codexFingerprintMode.value === 'off') {
      delete extra.codex_fingerprint_mode
    } else {
      extra.codex_fingerprint_mode = context.codexFingerprintMode.value
    }
    if (context.codexPrewarmContinuationEnabled.value) {
      extra.codex_prewarm_continuation_enabled = true
    } else {
      delete extra.codex_prewarm_continuation_enabled
    }
  }
  if (account.type !== 'oauth') {
    delete extra.codex_fingerprint_mode
  }
  if (account.type === 'apikey') {
    if (context.codexThinkingTagNormalizationEnabled.value) {
      extra.codex_thinking_tag_normalization_enabled = true
    } else {
      delete extra.codex_thinking_tag_normalization_enabled
    }
  } else {
    delete extra.codex_thinking_tag_normalization_enabled
  }
  if (context.isSparkShadow.value) {
    delete extra.openai_long_context_billing_enabled
  } else {
    extra.openai_long_context_billing_enabled =
      context.openAILongContextBillingEnabled.value
  }
  if (account.type === 'oauth') {
    if (context.tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (context.tlsFingerprintProfileId.value != null) {
        extra.tls_fingerprint_profile_id = context.tlsFingerprintProfileId.value
      } else {
        delete extra.tls_fingerprint_profile_id
      }
    } else {
      delete extra.enable_tls_fingerprint
      delete extra.tls_fingerprint_profile_id
    }
  }
  if (context.openAICompactMode.value === 'auto') {
    delete extra.openai_compact_mode
  } else {
    extra.openai_compact_mode = context.openAICompactMode.value
  }

  if (
    account.type === 'oauth' &&
    !context.isSparkShadow.value &&
    !context.isOpenAIPersonalAccessTokenAccount.value &&
    account.credentials?.auth_mode !== 'agentIdentity'
  ) {
    // Account updates merge extra JSONB keys, so an explicit false must override
    // a persisted legacy true value after the switch is turned off.
    extra.excel_bps_enabled = context.excelBpsEnabled.value
  }

  if (account.type === 'apikey') {
    if (
      !context.openAITextGenerationCapabilityEnabled.value ||
      context.openAIResponsesMode.value === 'auto'
    ) {
      delete extra.openai_responses_mode
    } else {
      extra.openai_responses_mode = context.openAIResponsesMode.value
    }
    if (context.openAIForceImageAPIEnabled.value) {
      extra.openai_force_image_api = true
    } else {
      delete extra.openai_force_image_api
    }
  }

  applyOpenAIAutoPauseExtra(extra, context)
  applyCodexImageToolExtra(extra, context)

  if (account.type === 'oauth' || account.type === 'setup-token') {
    if (context.codexCLIOnlyEnabled.value) {
      extra.codex_cli_only = true
    } else if (hadCodexCLIOnlyEnabled) {
      extra.codex_cli_only = false
    } else {
      delete extra.codex_cli_only
    }
    delete extra.codex_cli_only_allowed_clients
    if (
      context.codexCLIOnlyEnabled.value &&
      context.codexCLIOnlyAppServerEnabled.value
    ) {
      extra.codex_cli_only_allow_app_server = true
    } else {
      delete extra.codex_cli_only_allow_app_server
    }
  }
  payload.extra = extra
}

function applyQuotaExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (!supportsAccountQuota(account.type)) return
  const extra: UpdatePayload = {
    ...((payload.extra as UpdatePayload | undefined) ||
      (account.extra as UpdatePayload | undefined) ||
      {}),
  }
  if (context.editQuotaLimit.value != null && context.editQuotaLimit.value > 0) {
    extra.quota_limit = context.editQuotaLimit.value
  } else {
    delete extra.quota_limit
  }
  if (
    context.editQuotaDailyLimit.value != null &&
    context.editQuotaDailyLimit.value > 0
  ) {
    extra.quota_daily_limit = context.editQuotaDailyLimit.value
  } else {
    delete extra.quota_daily_limit
    delete extra.quota_daily_used
    delete extra.quota_daily_start
  }
  if (
    context.editQuotaWeeklyLimit.value != null &&
    context.editQuotaWeeklyLimit.value > 0
  ) {
    extra.quota_weekly_limit = context.editQuotaWeeklyLimit.value
  } else {
    delete extra.quota_weekly_limit
    delete extra.quota_weekly_used
    delete extra.quota_weekly_start
  }

  if (context.editDailyResetMode.value === 'fixed') {
    extra.quota_daily_reset_mode = 'fixed'
    extra.quota_daily_reset_hour = context.editDailyResetHour.value ?? 0
  } else {
    delete extra.quota_daily_reset_mode
    delete extra.quota_daily_reset_hour
  }
  if (context.editWeeklyResetMode.value === 'fixed') {
    extra.quota_weekly_reset_mode = 'fixed'
    extra.quota_weekly_reset_day = context.editWeeklyResetDay.value ?? 1
    extra.quota_weekly_reset_hour = context.editWeeklyResetHour.value ?? 0
  } else {
    delete extra.quota_weekly_reset_mode
    delete extra.quota_weekly_reset_day
    delete extra.quota_weekly_reset_hour
  }
  if (
    context.editDailyResetMode.value === 'fixed' ||
    context.editWeeklyResetMode.value === 'fixed'
  ) {
    extra.quota_reset_timezone = context.editResetTimezone.value || 'UTC'
  } else {
    delete extra.quota_reset_timezone
  }
  context.writeQuotaNotifyToExtra(extra, 'update')
  payload.extra = extra
}

function applyAPIKeyBalanceExtra(
  payload: UpdatePayload,
  account: Account,
  context: EditAccountUpdatePayloadContext,
): void {
  if (account.type !== 'apikey') return
  const extra: UpdatePayload = {
    ...((payload.extra as UpdatePayload | undefined) ||
      (account.extra as UpdatePayload | undefined) ||
      {}),
  }
  if (context.autoDisableOnUpstreamInsufficientBalance.value) {
    extra.auto_disable_on_upstream_insufficient_balance = true
  } else {
    delete extra.auto_disable_on_upstream_insufficient_balance
  }
  payload.extra = extra
}

export function buildEditAccountUpdatePayload(
  account: Account,
  context: EditAccountUpdatePayloadContext,
): UpdatePayload | null {
  const payload: UpdatePayload = { ...context.form }
  if (payload.proxy_id === null) payload.proxy_id = 0
  if (context.form.expires_at === null) payload.expires_at = 0
  const loadFactor = context.form.load_factor
  if (loadFactor == null || Number.isNaN(loadFactor) || loadFactor <= 0) {
    payload.load_factor = 0
  }
  payload.auto_pause_on_expired = context.autoPauseOnExpired.value
  if (isUpstreamBillingProbeEligible(account.platform, account.type)) {
    payload.upstream_billing_probe_enabled =
      context.upstreamBillingAutoProbeEnabled.value
    payload.upstream_billing_rate_sync_enabled =
      context.upstreamBillingRateSyncEnabled.value
    if (context.upstreamBillingRateSyncEnabled.value) {
      delete payload.rate_multiplier
    }
  }

  if (!applyAccountTypeCredentials(payload, account, context)) return null
  applyOAuthModelCredentials(payload, account, context)
  if (!applyGrokOAuthSettings(payload, account, context)) return null
  applyOpenAIPlanType(payload, account, context)
  applyAntigravityCredentials(payload, account, context)
  applyAntigravityExtra(payload, account, context)
  applyAnthropicOAuthExtra(payload, account, context)
  applyAnthropicAPIKeyExtra(payload, account, context)
  applyOpenAIExtra(payload, account, context)
  if (account.platform === 'openai' && context.openAIImageSizeTiers) {
    const extra = { ...((payload.extra as UpdatePayload | undefined) || account.extra || {}) }
    applyOpenAIImageSizeTiersToExtra(extra, context.openAIImageSizeTiers.value)
    payload.extra = extra
  }
  applyQuotaExtra(payload, account, context)
  applyAPIKeyBalanceExtra(payload, account, context)
  return payload
}
