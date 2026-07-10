import type { GroupPlatform, ClaudeCodeDefaultModels } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'

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
  // Per-group Claude Code tier→model mapping; only non-empty tiers are emitted.
  claudeCodeModels?: ClaudeCodeDefaultModels
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: `${baseUrl}/antigravity`
      }
    case 'openai':
      return {
        app: 'codex',
        endpoint: baseUrl,
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
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

  // Claude Code remaps its haiku/sonnet/opus tiers to upstream models; pass
  // only the non-empty ones as CC Switch deeplink params (one per
  // ANTHROPIC_DEFAULT_*_MODEL env var). No mapping = upstream natively
  // understands Claude model names.
  if (config.app === 'claude' && input.claudeCodeModels) {
    const m = input.claudeCodeModels
    if (m.opus) entries.push(['opusModel', m.opus])
    if (m.sonnet) entries.push(['sonnetModel', m.sonnet])
    if (m.haiku) entries.push(['haikuModel', m.haiku])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
