import type { GroupPlatform, ModelAllowlist } from '@/types'

export interface ClientExportConfig {
  codex_model?: string
  gemini_model?: string
  grok_model?: string
  codex_reasoning_effort?: string
  codex_auth_mode?: string
  codex_catalog_mode?: string
  claude_traffic?: string
  cc_usage_mode?: string
  cc_usage_interval?: string
}

export type ClientExportModelField = 'codex_model' | 'gemini_model' | 'grok_model'
export type ClientExportClient = 'codex' | 'gemini' | 'claude' | 'grok' | 'ccswitch'

/** Keep the platform's native client first and separate independent templates. */
export function clientExportClients(platform: GroupPlatform): ClientExportClient[] {
  if (platform === 'typesafe') return []
  if (platform === 'openai') return ['codex', 'claude', 'ccswitch']
  if (platform === 'gemini') return ['gemini', 'codex', 'ccswitch']
  if (platform === 'antigravity') return ['claude', 'gemini', 'codex', 'ccswitch']
  if (platform === 'grok') return ['grok', 'claude', 'codex', 'ccswitch']
  return ['claude', 'codex', 'ccswitch']
}

/** Enumerate settings preserved by the native copy/import path; never accept raw config. */
export function clientExportPresets(client: ClientExportClient, platform: GroupPlatform): Partial<Record<keyof ClientExportConfig, string[]>> {
  if (client === 'codex') return {
    codex_reasoning_effort: ['low', 'medium', 'high', 'xhigh'],
    ...(platform === 'openai' ? { codex_auth_mode: ['legacy', 'api-key'] } : {}),
    codex_catalog_mode: ['remote', 'file']
  }
  if (client === 'claude') return { claude_traffic: ['disabled', 'enabled'] }
  if (client === 'ccswitch') return { cc_usage_mode: ['enabled', 'disabled'], cc_usage_interval: ['0', '5', '15', '30', '60'] }
  return {}
}

export interface ClientExportOptions {
  codex_models: string[]
  gemini_models: string[]
  grok_models: string[]
}

/** Match the server's glob allowlist without accepting a glob as a selected model. */
export function filterClientExportModels(models: string[], allowlist?: ModelAllowlist): string[] {
  return [...new Set(models)].filter(model => {
    if (!model || model.includes('*')) return false
    if (!allowlist?.enabled) return true
    return allowlist.models.some(pattern => {
      const expression = pattern.trim().split('*').map(part => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('.*')
      return new RegExp(`^${expression}$`, 'i').test(model)
    })
  })
}

/** Return only fields that correspond to a native template model slot. */
export function clientExportFields(platform: GroupPlatform): ClientExportModelField[] {
  if (platform === 'typesafe') return []
  const fields: ClientExportModelField[] = ['codex_model']
  if (platform === 'gemini' || platform === 'antigravity') fields.push('gemini_model')
  if (platform === 'grok') fields.push('grok_model')
  return fields
}

/** Resolve a configured model, keeping each client's native default when unset. */
export function clientExportModel(configured: string | undefined, nativeModel: string): string {
  return configured || nativeModel
}
