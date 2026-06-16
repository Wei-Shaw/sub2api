/**
 * OpenAI form edit-mode helpers.
 * Split from useOpenAIForm to keep files under 200 lines.
 */
import type { Ref } from 'vue'
import type { EditFormPayload, ModelMapping, SdkAccount } from '@sub2api/plugin-sdk'
import {
  loadModelMappingFromCredentials,
  loadPoolModeFromCredentials,
  loadCustomErrorCodesFromCredentials,
  loadTempUnschedFromCredentials,
  loadCompactModelMappingsFromCredentials,
  applyModelMappingToCredentials,
  applyPoolModeToCredentials,
  applyCustomErrorCodesToCredentials,
  applyTempUnschedToCredentials,
  type TempUnschedRuleForm,
} from '@sub2api/plugin-sdk'
import {
  OPENAI_WS_MODE_OFF,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeFromExtra,
  type OpenAIWSMode,
} from '../utils/openaiWsMode'
import { buildModelMappingObject } from '@sub2api/plugin-sdk'

type OpenAICompactMode = 'auto' | 'force_on' | 'force_off'
type CodexImageGenBridgeMode = 'inherit' | 'enabled' | 'disabled'

export interface OpenAIFormEditRefs {
  openaiPassthroughEnabled: Ref<boolean>
  openAICompactMode: Ref<OpenAICompactMode>
  openaiOAuthWSMode: Ref<OpenAIWSMode>
  openaiAPIKeyWSMode: Ref<OpenAIWSMode>
  codexCLIOnlyEnabled: Ref<boolean>
  codexImageGenerationBridgeMode: Ref<CodexImageGenBridgeMode>
  apiKeyBaseUrl: Ref<string>
  editApiKey: Ref<string>
  openAICompactModelMappings: Ref<ModelMapping[]>
  modelRestrictionMode: Ref<'whitelist' | 'mapping'>
  allowedModels: Ref<string[]>
  modelMappings: Ref<ModelMapping[]>
  poolModeEnabled: Ref<boolean>
  poolModeRetryCount: Ref<number>
  customErrorCodesEnabled: Ref<boolean>
  selectedErrorCodes: Ref<number[]>
  tempUnschedEnabled: Ref<boolean>
  tempUnschedRules: Ref<TempUnschedRuleForm[]>
}

export function initFromAccount(account: SdkAccount, refs: OpenAIFormEditRefs): void {
  const credentials = account.credentials as Record<string, unknown> | undefined
  const extra = account.extra as Record<string, unknown> | undefined
  loadTempUnschedFromCredentials(credentials, refs.tempUnschedEnabled, refs.tempUnschedRules)
  refs.openaiPassthroughEnabled.value = extra?.openai_passthrough === true || extra?.openai_oauth_passthrough === true
  refs.openAICompactMode.value = (extra?.openai_compact_mode as OpenAICompactMode) || 'auto'
  refs.openaiOAuthWSMode.value = resolveOpenAIWSModeFromExtra(extra, {
    modeKey: 'openai_oauth_responses_websockets_v2_mode',
    enabledKey: 'openai_oauth_responses_websockets_v2_enabled',
    fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
    defaultMode: OPENAI_WS_MODE_OFF,
  })
  refs.openaiAPIKeyWSMode.value = resolveOpenAIWSModeFromExtra(extra, {
    modeKey: 'openai_apikey_responses_websockets_v2_mode',
    enabledKey: 'openai_apikey_responses_websockets_v2_enabled',
    fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
    defaultMode: OPENAI_WS_MODE_OFF,
  })
  refs.codexCLIOnlyEnabled.value = account.type === 'oauth' ? (extra?.codex_cli_only === true) : false
  const bridgeVal = typeof extra?.codex_image_generation_bridge === 'boolean'
    ? extra.codex_image_generation_bridge
    : extra?.codex_image_generation_bridge_enabled
  if (bridgeVal === true) refs.codexImageGenerationBridgeMode.value = 'enabled'
  else if (bridgeVal === false) refs.codexImageGenerationBridgeMode.value = 'disabled'
  else refs.codexImageGenerationBridgeMode.value = 'inherit'
  loadCompactModelMappingsFromCredentials(credentials, refs.openAICompactModelMappings)
  if (account.type === 'apikey') {
    refs.apiKeyBaseUrl.value = (credentials?.base_url as string) || 'https://api.openai.com'
    refs.editApiKey.value = ''
    loadModelMappingFromCredentials(credentials, refs.modelRestrictionMode, refs.allowedModels, refs.modelMappings)
    loadPoolModeFromCredentials(credentials, refs.poolModeEnabled, refs.poolModeRetryCount)
    loadCustomErrorCodesFromCredentials(credentials, refs.customErrorCodesEnabled, refs.selectedErrorCodes)
  } else if (account.type === 'oauth') {
    loadModelMappingFromCredentials(credentials, refs.modelRestrictionMode, refs.allowedModels, refs.modelMappings)
  }
}

export function getEditPayload(
  account: SdkAccount,
  refs: OpenAIFormEditRefs,
): EditFormPayload {
  const currentCreds = (account.credentials as Record<string, unknown>) || {}
  const newCreds: Record<string, unknown> = { ...currentCreds }
  const currentExtra = (account.extra as Record<string, unknown>) || {}
  const newExtra: Record<string, unknown> = { ...currentExtra }
  const unschedResult = applyTempUnschedToCredentials(newCreds, refs.tempUnschedEnabled.value, refs.tempUnschedRules.value)
  if (!unschedResult.valid) return { credentials: undefined, error: unschedResult.error }
  const shouldApply = !refs.openaiPassthroughEnabled.value
  if (account.type === 'apikey' || account.type === 'oauth') {
    applyModelMappingToCredentials(
      newCreds, refs.modelRestrictionMode.value, refs.allowedModels.value,
      refs.modelMappings.value, !shouldApply, currentCreds.model_mapping,
    )
    const cm = buildModelMappingObject('mapping', [], refs.openAICompactModelMappings.value) ?? undefined
    if (cm) newCreds.compact_model_mapping = cm
    else delete newCreds.compact_model_mapping
  }
  if (account.type === 'apikey') {
    newCreds.base_url = refs.apiKeyBaseUrl.value.trim() || 'https://api.openai.com'
    if (refs.editApiKey.value.trim()) newCreds.api_key = refs.editApiKey.value.trim()
    applyPoolModeToCredentials(newCreds, refs.poolModeEnabled.value, refs.poolModeRetryCount.value)
    applyCustomErrorCodesToCredentials(newCreds, refs.customErrorCodesEnabled.value, refs.selectedErrorCodes.value)
  }
  if (account.type === 'oauth') {
    newExtra.openai_oauth_responses_websockets_v2_mode = refs.openaiOAuthWSMode.value
    newExtra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(refs.openaiOAuthWSMode.value)
  } else if (account.type === 'apikey') {
    newExtra.openai_apikey_responses_websockets_v2_mode = refs.openaiAPIKeyWSMode.value
    newExtra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(refs.openaiAPIKeyWSMode.value)
  }
  delete newExtra.responses_websockets_v2_enabled
  delete newExtra.openai_ws_enabled
  if (refs.openaiPassthroughEnabled.value) newExtra.openai_passthrough = true
  else { delete newExtra.openai_passthrough; delete newExtra.openai_oauth_passthrough }
  if (refs.openAICompactMode.value !== 'auto') newExtra.openai_compact_mode = refs.openAICompactMode.value
  else delete newExtra.openai_compact_mode
  delete newExtra.codex_image_generation_bridge_enabled
  if (refs.codexImageGenerationBridgeMode.value === 'inherit') {
    delete newExtra.codex_image_generation_bridge
  } else {
    newExtra.codex_image_generation_bridge = refs.codexImageGenerationBridgeMode.value === 'enabled'
  }
  if (account.type === 'oauth') {
    const hadCodex = currentExtra.codex_cli_only === true
    if (refs.codexCLIOnlyEnabled.value) newExtra.codex_cli_only = true
    else if (hadCodex) newExtra.codex_cli_only = false
    else delete newExtra.codex_cli_only
  }
  return { credentials: newCreds, extra: newExtra }
}