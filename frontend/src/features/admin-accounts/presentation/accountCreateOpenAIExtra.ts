import type { Ref } from 'vue'
import { isOpenAIWSModeEnabled, type OpenAIWSMode } from '@/core/utils/openaiWsMode'
import { applyOpenAIImageSizeTiersToExtra, type OpenAIImageSizeTier } from './openaiImageSizeTiers'

interface CreateOpenAIExtraState {
  form: { platform: string; type: string }
  openAIImageSizeTiers: Ref<OpenAIImageSizeTier[]>
  accountCategory: Ref<string>
  openaiOAuthResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  openaiAPIKeyResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  openaiPassthroughEnabled: Ref<boolean>
  openaiFlattenNamespacesEnabled: Ref<boolean>
  codexPrewarmContinuationEnabled: Ref<boolean>
  codexFingerprintMode: Ref<string>
  customBaseUrlEnabled: Ref<boolean>
  customBaseUrl: Ref<string>
  codexThinkingTagNormalizationEnabled: Ref<boolean>
  openAILongContextBillingEnabled: Ref<boolean>
  codexCLIOnlyEnabled: Ref<boolean>
  codexCLIOnlyAppServerEnabled: Ref<boolean>
  openAICompactMode: Ref<string>
  openAITextGenerationCapabilityEnabled: Ref<boolean>
  openAIResponsesMode: Ref<string>
  openAIForceImageAPIEnabled: Ref<boolean>
}

export function buildCreateOpenAIExtra(state: CreateOpenAIExtraState, base?: Record<string, unknown>): Record<string, unknown> | undefined {
  const { form, openAIImageSizeTiers, accountCategory, openaiOAuthResponsesWebSocketV2Mode, openaiAPIKeyResponsesWebSocketV2Mode, openaiPassthroughEnabled, openaiFlattenNamespacesEnabled, codexPrewarmContinuationEnabled, codexFingerprintMode, customBaseUrlEnabled, customBaseUrl, codexThinkingTagNormalizationEnabled, openAILongContextBillingEnabled, codexCLIOnlyEnabled, codexCLIOnlyAppServerEnabled, openAICompactMode, openAITextGenerationCapabilityEnabled, openAIResponsesMode, openAIForceImageAPIEnabled } = state

  if (form.platform !== 'openai') {
    return base
  }

  const extra: Record<string, unknown> = { ...(base || {}) }
  applyOpenAIImageSizeTiersToExtra(extra, openAIImageSizeTiers.value)
  if (accountCategory.value === 'oauth-based') {
    extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthResponsesWebSocketV2Mode.value)
  } else if (accountCategory.value === 'apikey') {
    extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyResponsesWebSocketV2Mode.value)
  }
  // 清理兼容旧键，统一改用分类型开关。
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled
  if (openaiPassthroughEnabled.value) {
    extra.openai_passthrough = true
  } else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  }
  if (form.type === 'oauth' && openaiFlattenNamespacesEnabled.value) {
    extra.openai_responses_flatten_namespaces = true
  } else {
    delete extra.openai_responses_flatten_namespaces
  }
  if (form.type === 'oauth' && codexPrewarmContinuationEnabled.value) {
    extra.codex_prewarm_continuation_enabled = true
  } else {
    delete extra.codex_prewarm_continuation_enabled
  }
  if (form.type === 'oauth' && codexFingerprintMode.value !== 'off') {
    extra.codex_fingerprint_mode = codexFingerprintMode.value
  } else {
    delete extra.codex_fingerprint_mode
  }

  // OpenAI OAuth/Codex model traffic may use a custom relay.  Keep this in
  // the OpenAI extra builder so authorization-code, refresh-token, and
  // imported-session creation paths all persist the same setting.
  if (
    form.type === 'oauth' &&
    customBaseUrlEnabled.value &&
    customBaseUrl.value.trim()
  ) {
    extra.custom_base_url_enabled = true
    extra.custom_base_url = customBaseUrl.value.trim()
  } else if (form.type === 'oauth') {
    delete extra.custom_base_url_enabled
    delete extra.custom_base_url
  }
  if (form.type === 'apikey' && codexThinkingTagNormalizationEnabled.value) {
    extra.codex_thinking_tag_normalization_enabled = true
  } else {
    delete extra.codex_thinking_tag_normalization_enabled
  }
  extra.openai_long_context_billing_enabled = openAILongContextBillingEnabled.value

  if (accountCategory.value === 'oauth-based' && codexCLIOnlyEnabled.value) {
    extra.codex_cli_only = true
  } else {
    delete extra.codex_cli_only
  }
  delete extra.codex_cli_only_allowed_clients
  if (
    accountCategory.value === 'oauth-based' &&
    codexCLIOnlyEnabled.value &&
    codexCLIOnlyAppServerEnabled.value
  ) {
    extra.codex_cli_only_allow_app_server = true
  } else {
    delete extra.codex_cli_only_allow_app_server
  }
  if (openAICompactMode.value !== 'auto') {
    extra.openai_compact_mode = openAICompactMode.value
  } else {
    delete extra.openai_compact_mode
  }

  if (
    accountCategory.value === 'apikey' &&
    openAITextGenerationCapabilityEnabled.value &&
    openAIResponsesMode.value !== 'auto'
  ) {
    extra.openai_responses_mode = openAIResponsesMode.value
  } else {
    delete extra.openai_responses_mode
  }
  if (accountCategory.value === 'apikey' && openAIForceImageAPIEnabled.value) {
    extra.openai_force_image_api = true
  } else {
    delete extra.openai_force_image_api
  }

  return Object.keys(extra).length > 0 ? extra : undefined
}
