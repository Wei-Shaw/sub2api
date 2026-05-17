import type { GroupPlatform } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.4'

export type CcSwitchClientType = 'claude' | 'gemini'

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
}

/**
 * Registry of platform-specific CC Switch import configurations.
 * Each entry defines: app name, endpoint suffix (appended to baseUrl), optional model,
 * and whether clientType should influence the app name.
 */
interface CcSwitchPlatformEntry {
  /** Default CC Switch app name */
  app: string
  /** Endpoint path suffix appended to baseUrl (e.g. '/antigravity') */
  endpointSuffix?: string
  /** Fixed model to include in deeplink */
  model?: string
  /** If true, clientType 'gemini' overrides app to 'gemini' */
  clientTypeAware?: boolean
}

const CC_SWITCH_PLATFORM_REGISTRY: Record<string, CcSwitchPlatformEntry> = {
  anthropic: { app: 'claude' },
  antigravity: { app: 'claude', endpointSuffix: '/antigravity', clientTypeAware: true },
  openai: { app: 'codex', model: OPENAI_CC_SWITCH_CODEX_MODEL },
  gemini: { app: 'gemini' },
}

const CC_SWITCH_DEFAULT_ENTRY: CcSwitchPlatformEntry = { app: 'claude' }

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  const entry = CC_SWITCH_PLATFORM_REGISTRY[platform || 'anthropic'] ?? CC_SWITCH_DEFAULT_ENTRY
  const app = (entry.clientTypeAware && clientType === 'gemini') ? 'gemini' : entry.app
  return {
    app,
    endpoint: entry.endpointSuffix ? `${baseUrl}${entry.endpointSuffix}` : baseUrl,
    ...(entry.model ? { model: entry.model } : {}),
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', btoa(input.usageScript)],
    ['usageAutoInterval', '30']
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
