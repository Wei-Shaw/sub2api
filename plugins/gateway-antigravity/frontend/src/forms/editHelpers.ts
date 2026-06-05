/**
 * Antigravity form edit-mode helpers.
 *
 * Separated from useAntigravityForm to keep file sizes manageable.
 * These functions handle populating form refs from existing account
 * data and building update payloads.
 */
import type { Ref } from 'vue'
import type {
  ModelMapping,
  EditFormPayload,
  SdkAccount,
  TempUnschedRuleForm,
} from '@sub2api/plugin-sdk'
import {
  applyInterceptWarmup,
  loadInterceptWarmupFromCredentials,
  loadTempUnschedFromCredentials,
  applyTempUnschedToCredentials,
} from '@sub2api/plugin-sdk'
import { buildModelMappingObject } from './modelMapping'

export interface AntigravityFormRefs {
  antigravityAccountType: Ref<'oauth' | 'upstream'>
  upstreamBaseUrl: Ref<string>
  editUpstreamApiKey: Ref<string>
  antigravityModelMappings: Ref<ModelMapping[]>
  mixedScheduling: Ref<boolean>
  allowOverages: Ref<boolean>
  interceptWarmupRequests: Ref<boolean>
  tempUnschedEnabled: Ref<boolean>
  tempUnschedRules: Ref<TempUnschedRuleForm[]>
}

export function initFromAccount(
  account: SdkAccount,
  refs: AntigravityFormRefs,
): void {
  const credentials = account.credentials
  const extra = account.extra
  loadInterceptWarmupFromCredentials(credentials, refs.interceptWarmupRequests)
  loadTempUnschedFromCredentials(
    credentials, refs.tempUnschedEnabled, refs.tempUnschedRules
  )

  if (account.type === 'upstream') {
    refs.antigravityAccountType.value = 'upstream'
    refs.upstreamBaseUrl.value = (credentials?.base_url as string) || ''
    refs.editUpstreamApiKey.value = ''
  }
  refs.mixedScheduling.value = extra?.mixed_scheduling === true
  refs.allowOverages.value = extra?.allow_overages === true

  const rawMapping = credentials?.model_mapping as Record<string, string> | undefined
  if (rawMapping && typeof rawMapping === 'object') {
    refs.antigravityModelMappings.value = Object.entries(rawMapping)
      .map(([from, to]) => ({ from, to }))
    return
  }
  const rawWhitelist = credentials?.model_whitelist
  if (Array.isArray(rawWhitelist) && rawWhitelist.length > 0) {
    refs.antigravityModelMappings.value = rawWhitelist
      .map(v => String(v).trim()).filter(v => v.length > 0)
      .map(m => ({ from: m, to: m }))
  } else {
    refs.antigravityModelMappings.value = []
  }
}

export function getEditPayload(
  account: SdkAccount,
  refs: AntigravityFormRefs,
): EditFormPayload {
  const currentCreds = (account.credentials) || {}
  const newCreds: Record<string, unknown> = { ...currentCreds }
  applyInterceptWarmup(newCreds, refs.interceptWarmupRequests.value, 'edit')
  const unschedResult = applyTempUnschedToCredentials(
    newCreds, refs.tempUnschedEnabled.value, refs.tempUnschedRules.value
  )
  if (!unschedResult.valid) return { credentials: undefined, error: unschedResult.error }

  if (account.type === 'upstream') {
    newCreds.base_url = refs.upstreamBaseUrl.value.trim()
    if (refs.editUpstreamApiKey.value.trim()) {
      newCreds.api_key = refs.editUpstreamApiKey.value.trim()
    }
  }
  delete newCreds.model_whitelist
  delete newCreds.model_mapping
  const mm = buildModelMappingObject(refs.antigravityModelMappings.value)
  if (mm) newCreds.model_mapping = mm

  const currentExtra = (account.extra) || {}
  const newExtra: Record<string, unknown> = { ...currentExtra }
  if (refs.mixedScheduling.value) newExtra.mixed_scheduling = true
  else delete newExtra.mixed_scheduling
  if (refs.allowOverages.value) newExtra.allow_overages = true
  else delete newExtra.allow_overages

  return { credentials: newCreds, extra: newExtra }
}
