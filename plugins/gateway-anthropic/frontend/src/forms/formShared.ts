/**
 * Shared model mapping and credentials utility used by the main
 * useAnthropicForm composable. Extracted to keep each file < 200 lines.
 */
import type { ModelMapping } from '@sub2api/plugin-sdk'
import {
  normalizePoolModeRetryCount,
  applyInterceptWarmup as sdkApplyInterceptWarmup,
} from '@sub2api/plugin-sdk'

/**
 * Build a model_mapping object from whitelist or mapping mode.
 * Inlined from host useModelWhitelist to avoid host-internal import.
 */
export function buildModelMappingObject(
  mode: 'whitelist' | 'mapping',
  allowedModels: string[],
  modelMappings: ModelMapping[]
): Record<string, string> | null {
  const mapping: Record<string, string> = {}
  if (mode === 'whitelist') {
    for (const model of allowedModels) {
      if (!model.includes('*')) mapping[model] = model
    }
  } else {
    for (const m of modelMappings) {
      const from = m.from.trim()
      const to = m.to.trim()
      if (!from || !to) continue
      if (from.indexOf('*') !== -1 && from.indexOf('*') !== from.length - 1) continue
      if (to.includes('*')) continue
      mapping[from] = to
    }
  }
  return Object.keys(mapping).length > 0 ? mapping : null
}

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
  sdkApplyInterceptWarmup(creds, opts.interceptWarmupRequests, 'create')
}

export { sdkApplyInterceptWarmup as applyInterceptWarmup }

