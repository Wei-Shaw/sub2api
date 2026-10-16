import type { GroupPlatform } from '@/types'
import { clientExportModel, type ClientExportConfig } from '@/utils/groupClientExport'
import { buildCodexModelCatalogUrl } from '@/api/codex'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

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
  clientExportConfig?: ClientExportConfig
}

/**
 * Balance query CC Switch runs against the imported provider. CC Switch fills
 * `{{baseUrl}}` with the provider's base URL as stored — Codex and Grok imports
 * may carry a trailing `/v1` (Grok normalizes it via `withV1Endpoint`), and users
 * may edit it either way afterwards — then evaluates the script, so the URL
 * strips an existing `/v1` instead of blindly appending one (`/v1/v1/usage`
 * is a 404 and CC Switch shows "query failed").
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

/** Preserve native client/endpoint selection while applying only supported model defaults. */
export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string,
  config?: ClientExportConfig
): CcSwitchImportConfig {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: `${baseUrl.replace(/\/+$/, '')}/antigravity`,
        ...(clientType === 'gemini' && config?.gemini_model ? { model: config.gemini_model } : {})
      }
    case 'openai':
      return {
        app: 'codex',
        // CC Switch's Codex provider appends the OpenAI-compatible path itself.
        // Passing /v1 here can make the client request /v1/v1/....
        endpoint: withoutTrailingSlashes(baseUrl),
        model: clientExportModel(config?.codex_model, OPENAI_CC_SWITCH_CODEX_MODEL)
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl,
        ...(config?.gemini_model ? { model: config.gemini_model } : {})
      }
    case 'grok':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: clientExportModel(config?.grok_model, GROK_CC_SWITCH_MODEL)
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
  }
}

/** Encode Unicode script/config text as UTF-8 before URLSearchParams escapes it. */
function encodeBase64(value: string): string {
  return btoa(Array.from(new TextEncoder().encode(value), byte => String.fromCharCode(byte)).join(''))
}

/** Serialize only provider fields CC Switch preserves, keeping protocol and key injection automatic. */
function nativeImportPayload(input: CcSwitchImportDeeplinkInput, native: CcSwitchImportConfig): object | undefined {
  const config = input.clientExportConfig
  if (native.app === 'claude' && config?.claude_traffic) {
    return { env: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: config.claude_traffic === 'enabled' ? '0' : '1' } }
  }
  if (native.app !== 'codex' || !config || !(config.codex_model || config.codex_reasoning_effort || config.codex_auth_mode || config.codex_catalog_mode)) return undefined
  const apiKeyMode = config.codex_auth_mode === 'api-key'
  const lines = [
    'model_provider = "sub2api"',
    `model = ${JSON.stringify(native.model)}`,
    `review_model = ${JSON.stringify(native.model)}`,
    ...(config.codex_reasoning_effort ? [`model_reasoning_effort = ${JSON.stringify(config.codex_reasoning_effort)}`] : []),
    'disable_response_storage = true',
    '',
    '[model_providers.sub2api]',
    `name = ${JSON.stringify(input.providerName)}`,
    `base_url = ${JSON.stringify(native.endpoint)}`,
    'wire_api = "responses"',
    `requires_openai_auth = ${!apiKeyMode}`,
    ...(apiKeyMode ? [`experimental_bearer_token = ${JSON.stringify(input.apiKey)}`, 'http_headers = { "x-openai-actor-authorization" = "local-image-extension" }'] : []),
    ...(config.codex_catalog_mode !== 'file' ? [`model_catalog_url = ${JSON.stringify(buildCodexModelCatalogUrl(native.endpoint))}`] : [])
  ]
  return { ...(apiKeyMode ? {} : { auth: { OPENAI_API_KEY: input.apiKey } }), config: lines.join('\n') }
}

/** Build native import defaults selected by the administrator; disabled usage omits the script. */
export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl, input.clientExportConfig)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json']
  ]

  if (input.clientExportConfig?.cc_usage_mode !== 'disabled') {
    entries.push(['usageEnabled', 'true'], ['usageScript', encodeBase64(input.usageScript)], ['usageAutoInterval', input.clientExportConfig?.cc_usage_interval || '30'])
  } else {
    entries.push(['usageEnabled', 'false'])
  }
  const payload = nativeImportPayload(input, config)
  if (payload) entries.push(['config', encodeBase64(JSON.stringify(payload))])

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
