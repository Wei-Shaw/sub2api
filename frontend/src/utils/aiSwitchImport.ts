import type { GroupPlatform } from '@/types'

export type AiSwitchApp = 'claude' | 'codex' | 'gemini'

export interface AiSwitchImportConfig {
  /** AI Switch app the deep link targets; null = platform unsupported. */
  app: AiSwitchApp
  endpoint: string
}

export interface AiSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  providerName: string
  apiKey: string
}

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

function withoutTrailingSlashes(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '')
}

/**
 * Maps a Sub2API group platform to an AI Switch app dialect.
 *
 * Only offered where the gateway endpoint matches an AI Switch deep-link app:
 * - anthropic groups serve /v1/messages          → app=claude (root URL)
 * - openai groups serve /v1/responses            → app=codex ({root}/v1; AI Switch appends /responses)
 * - gemini groups serve /v1beta                  → app=gemini (root URL; Gemini CLI appends /v1beta)
 *
 * Grok groups require the Responses protocol, which no AI Switch deep-link app
 * maps to, so Grok — and unknown platforms — resolve to null.
 */
export function resolveAiSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  baseUrl: string
): AiSwitchImportConfig | null {
  switch (platform) {
    case 'anthropic':
      return { app: 'claude', endpoint: withoutTrailingSlashes(baseUrl) }
    case 'openai':
      return { app: 'codex', endpoint: withV1Endpoint(baseUrl) }
    case 'gemini':
      return { app: 'gemini', endpoint: withoutTrailingSlashes(baseUrl) }
    default:
      return null
  }
}

export function buildAiSwitchImportDeeplink(input: AiSwitchImportDeeplinkInput): string {
  const config = resolveAiSwitchImportConfig(input.platform, input.baseUrl)
  if (!config) {
    throw new Error(`AI Switch import is not supported for platform: ${input.platform}`)
  }
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['homepage', input.baseUrl]
  ]
  return `aiswitch://v1/import?${new URLSearchParams(entries).toString()}`
}

export function isAiSwitchImportSupported(platform: GroupPlatform | undefined | null): boolean {
  return (
    platform === 'anthropic' || platform === 'openai' || platform === 'gemini'
  )
}
