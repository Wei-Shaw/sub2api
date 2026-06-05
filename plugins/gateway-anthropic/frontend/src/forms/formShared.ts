/**
 * Shared model mapping and credentials utility used by the main
 * useAnthropicForm composable. Extracted to keep each file < 200 lines.
 */
import type { ModelMapping } from '@sub2api/plugin-sdk'
import {
  normalizePoolModeRetryCount,
  applyInterceptWarmup as sdkApplyInterceptWarmup,
  applyTempUnschedToCredentials,
  buildModelMappingObject,
  type TempUnschedRuleForm,
} from '@sub2api/plugin-sdk'

export { buildModelMappingObject }

/** Apply model mapping, pool mode, custom error codes, and intercept warmup to credentials. */
export function applySharedCredentials(
  creds: Record<string, unknown>,
  opts: {
    modelRestrictionMode: 'whitelist' | 'mapping'
    allowedModels: string[]
    modelMappings: ModelMapping[]
    poolModeEnabled: boolean
    poolModeRetryCount: number
    customErrorCodesEnabled: boolean
    selectedErrorCodes: number[]
    interceptWarmupRequests: boolean
    tempUnschedEnabled: boolean
    tempUnschedRules: TempUnschedRuleForm[]
  }
): void {
  const mapping = buildModelMappingObject(opts.modelRestrictionMode, opts.allowedModels, opts.modelMappings)
  if (mapping) creds.model_mapping = mapping
  if (opts.poolModeEnabled) {
    creds.pool_mode = true
    creds.pool_mode_retry_count = normalizePoolModeRetryCount(opts.poolModeRetryCount)
  }
  if (opts.customErrorCodesEnabled) {
    creds.custom_error_codes_enabled = true
    creds.custom_error_codes = [...opts.selectedErrorCodes]
  }
  applyTempUnschedToCredentials(creds, opts.tempUnschedEnabled, opts.tempUnschedRules)
  sdkApplyInterceptWarmup(creds, opts.interceptWarmupRequests, 'create')
}

export { sdkApplyInterceptWarmup as applyInterceptWarmup }

