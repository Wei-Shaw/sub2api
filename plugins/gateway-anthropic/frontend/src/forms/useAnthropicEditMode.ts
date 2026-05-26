/**
 * Anthropic form edit-mode logic: populate form from existing account
 * and build update payloads. Split from main composable for line count.
 */
import type { Ref } from 'vue'
import type { SdkAccount, EditFormPayload, ModelMapping } from '@sub2api/plugin-sdk'
import {
  loadModelMappingFromCredentials, applyModelMappingToCredentials,
  loadPoolModeFromCredentials, applyPoolModeToCredentials,
  loadCustomErrorCodesFromCredentials, applyCustomErrorCodesToCredentials,
  loadTempUnschedFromCredentials, applyTempUnschedToCredentials,
  loadInterceptWarmupFromCredentials, applyInterceptWarmup,
  type TempUnschedRuleForm,
} from '@sub2api/plugin-sdk'
import type { useAnthropicBedrockForm } from './useAnthropicBedrockForm'
import type { useAnthropicOAuthForm } from './useAnthropicOAuthForm'

export type AddMethod = 'oauth' | 'setup-token'

export interface EditModeRefs {
  addMethod: Ref<AddMethod>
  apiKeyBaseUrl: Ref<string>
  editApiKey: Ref<string>
  vertexLocation: Ref<string>
  anthropicPassthroughEnabled: Ref<boolean>
  webSearchEmulationMode: Ref<string>
  syncToStreamMode: Ref<string>
  interceptWarmupRequests: Ref<boolean>
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

export function initFromAccount(
  account: SdkAccount,
  refs: EditModeRefs,
  bedrock: ReturnType<typeof useAnthropicBedrockForm>,
  oauthQuota: ReturnType<typeof useAnthropicOAuthForm>,
  resetFn: () => void,
): void {
  resetFn()
  const credentials = account.credentials
  const extra = account.extra
  loadTempUnschedFromCredentials(credentials, refs.tempUnschedEnabled, refs.tempUnschedRules)
  loadInterceptWarmupFromCredentials(credentials, refs.interceptWarmupRequests)
  refs.syncToStreamMode.value = extra?.sync_to_stream === 'enabled' ? 'enabled' : 'default'

  if (account.type === 'apikey') {
    refs.apiKeyBaseUrl.value = (credentials?.base_url as string) || 'https://api.anthropic.com'
    refs.editApiKey.value = ''
    refs.anthropicPassthroughEnabled.value = extra?.anthropic_passthrough === true
    refs.webSearchEmulationMode.value = (extra?.web_search_emulation as string) || 'default'
    loadModelMappingFromCredentials(credentials, refs.modelRestrictionMode, refs.allowedModels, refs.modelMappings)
    loadPoolModeFromCredentials(credentials, refs.poolModeEnabled, refs.poolModeRetryCount)
    loadCustomErrorCodesFromCredentials(credentials, refs.customErrorCodesEnabled, refs.selectedErrorCodes)
  } else if (account.type === 'bedrock') {
    bedrock.initBedrockFromAccount(account)
    loadModelMappingFromCredentials(credentials, refs.modelRestrictionMode, refs.allowedModels, refs.modelMappings)
    loadPoolModeFromCredentials(credentials, refs.poolModeEnabled, refs.poolModeRetryCount)
    loadCustomErrorCodesFromCredentials(credentials, refs.customErrorCodesEnabled, refs.selectedErrorCodes)
  } else if (account.type === 'service_account') {
    refs.vertexLocation.value = (credentials?.location as string) || 'us-east5'
    loadModelMappingFromCredentials(credentials, refs.modelRestrictionMode, refs.allowedModels, refs.modelMappings)
  } else if (account.type === 'oauth' || account.type === 'setup-token') {
    refs.addMethod.value = account.type as AddMethod
    oauthQuota.initOAuthFromAccount(account)
  }
}

export function getEditPayload(
  account: SdkAccount,
  refs: EditModeRefs,
  bedrock: ReturnType<typeof useAnthropicBedrockForm>,
  oauthQuota: ReturnType<typeof useAnthropicOAuthForm>,
): EditFormPayload {
  const newCreds: Record<string, unknown> = { ...(account.credentials || {}) }
  const newExtra: Record<string, unknown> = { ...(account.extra || {}) }
  const unschedResult = applyTempUnschedToCredentials(newCreds, refs.tempUnschedEnabled.value, refs.tempUnschedRules.value)
  if (!unschedResult.valid) return { credentials: undefined, error: unschedResult.error }
  applyInterceptWarmup(newCreds, refs.interceptWarmupRequests.value, 'edit')

  if (account.type === 'apikey') {
    newCreds.base_url = refs.apiKeyBaseUrl.value.trim() || 'https://api.anthropic.com'
    if (refs.editApiKey.value.trim()) newCreds.api_key = refs.editApiKey.value.trim()
    applyModelMappingToCredentials(newCreds, refs.modelRestrictionMode.value, refs.allowedModels.value, refs.modelMappings.value)
    applyPoolModeToCredentials(newCreds, refs.poolModeEnabled.value, refs.poolModeRetryCount.value)
    applyCustomErrorCodesToCredentials(newCreds, refs.customErrorCodesEnabled.value, refs.selectedErrorCodes.value)
    if (refs.anthropicPassthroughEnabled.value) newExtra.anthropic_passthrough = true
    else delete newExtra.anthropic_passthrough
    if (refs.webSearchEmulationMode.value !== 'default') newExtra.web_search_emulation = refs.webSearchEmulationMode.value
    else delete newExtra.web_search_emulation
  } else if (account.type === 'bedrock') {
    bedrock.applyBedrockEditCredentials(newCreds)
    applyModelMappingToCredentials(newCreds, refs.modelRestrictionMode.value, refs.allowedModels.value, refs.modelMappings.value)
    applyPoolModeToCredentials(newCreds, refs.poolModeEnabled.value, refs.poolModeRetryCount.value)
    applyCustomErrorCodesToCredentials(newCreds, refs.customErrorCodesEnabled.value, refs.selectedErrorCodes.value)
  } else if (account.type === 'service_account') {
    newCreds.location = refs.vertexLocation.value.trim() || 'us-east5'
    applyModelMappingToCredentials(newCreds, refs.modelRestrictionMode.value, refs.allowedModels.value, refs.modelMappings.value)
  } else if (account.type === 'oauth' || account.type === 'setup-token') {
    oauthQuota.applyOAuthEditExtra(newExtra)
  }

  if (account.type !== 'bedrock') {
    if (refs.syncToStreamMode.value === 'default') delete newExtra.sync_to_stream
    else newExtra.sync_to_stream = refs.syncToStreamMode.value
  }
  return { credentials: newCreds, extra: newExtra }
}
