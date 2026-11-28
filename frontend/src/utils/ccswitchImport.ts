import type { GroupPlatform } from '@/types'
import { buildCodexApiKeyAuthConfig } from '@/utils/codexAuthConfig'

export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

export type CcSwitchClientType =
  | 'claude'
  | 'claude-desktop'
  | 'codex'
  | 'dsh'
  | 'gemini'
  | 'grokbuild'

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
  apiFormat?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
  model?: string
}

/**
 * Balance query CC Switch runs against the imported provider. CC Switch fills
 * `{{baseUrl}}` with the provider's base URL as stored. Some imported clients
 * store `/v1` in that URL and others do not, so the script removes an existing
 * `/v1` before appending `/v1/usage`; otherwise CC Switch requests
 * `/v1/v1/usage` and shows "query failed".
 */
export const CC_SWITCH_USAGE_SCRIPT = `({
    request: {
      url: "{{baseUrl}}".replace(/\\/+$/, "").replace(/\\/v1$/, "") + "/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

function withoutTrailingSlashes(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '')
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string,
  model?: string
): CcSwitchImportConfig {
  const sourcePlatform = platform || 'anthropic'

  switch (clientType) {
    case 'codex':
      return {
        app: 'codex',
        endpoint: withoutTrailingSlashes(baseUrl),
        model,
        apiFormat: sourcePlatform === 'anthropic' ? 'anthropic' : 'openai_responses'
      }
    case 'claude':
      return {
        app: 'claude',
        endpoint: sourcePlatform === 'antigravity'
          ? `${withoutTrailingSlashes(baseUrl)}/antigravity`
          : baseUrl,
        model,
        apiFormat: 'anthropic'
      }
    case 'claude-desktop':
      return {
        app: 'claude-desktop',
        endpoint: baseUrl,
        model,
        apiFormat: 'anthropic'
      }
    case 'dsh':
      return {
        app: 'dsh',
        endpoint: sourcePlatform === 'anthropic' ? baseUrl : withV1Endpoint(baseUrl),
        model,
        apiFormat: sourcePlatform === 'anthropic' ? 'anthropic' : 'openai_chat'
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: sourcePlatform === 'antigravity'
          ? `${withoutTrailingSlashes(baseUrl)}/antigravity`
          : baseUrl
      }
    case 'grokbuild':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: model || GROK_CC_SWITCH_MODEL
      }
    default: {
      const unreachableClientType: never = clientType
      throw new Error(`Unsupported CC Switch client: ${unreachableClientType}`)
    }
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(
    input.platform,
    input.clientType,
    input.baseUrl,
    input.model
  )
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

  if (config.apiFormat) {
    entries.push(['apiFormat', config.apiFormat])
  }

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  if (config.app === 'codex') {
    // CC Switch's embedded-config import preserves provider auth and headers.
    // It deliberately leaves global [features] settings to the user.
    const codexConfig = `model_provider = "sub2api"
${config.model ? `model = ${JSON.stringify(config.model)}\n` : ''}web_search = "live"

[model_providers.sub2api]
name = "Sub2API"
base_url = ${JSON.stringify(config.endpoint)}
wire_api = "responses"
${buildCodexApiKeyAuthConfig(input.apiKey)}
`
    const payload = JSON.stringify({ config: codexConfig })
    const bytes = new TextEncoder().encode(payload)
    entries.push(['config', btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''))])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
