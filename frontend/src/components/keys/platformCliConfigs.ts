/**
 * Registry of platform-specific CLI configuration data.
 * Unknown / plugin platforms fall back to the Anthropic-compatible default.
 */
import {
  openaiModels, geminiModels, claudeModels,
  antigravityGeminiModels, openaiAgent
} from './opencodeModelCatalogs'

// ---------- opencode model definitions ----------

export interface OpencodeModelDef {
  name: string
  limit: { context: number; output: number }
  options?: Record<string, unknown>
  modalities?: { input: string[]; output: string[] }
  variants?: Record<string, Record<string, unknown>>
}

// ---------- config interface ----------

export interface PlatformCliConfig {
  /** Canonical platform id. */
  platform: string
  /** Suffix appended to the site origin to form the platform base URL (e.g. '/anthropic'). */
  baseUrlSuffix: string
  /** Show Claude Code tab. */
  claudeCodeSupported: boolean
  /** Show Codex CLI tab. */
  codexCliSupported: boolean
  /** Show Codex CLI (WebSocket) tab. */
  codexWsSupported: boolean
  /** Show Gemini CLI tab. */
  geminiCliSupported: boolean
  /** Show Opencode tab. */
  opencodeSupported: boolean
  /** Default active client tab id when this platform is selected. */
  defaultClientTab: string
  /** Whether Claude Code tab requires allowMessagesDispatch gate. */
  claudeCodeRequiresDispatch: boolean

  // --- Claude Code env vars ---
  claudeCodeEnvBaseUrl?: string   // e.g. 'ANTHROPIC_BASE_URL'
  claudeCodeEnvApiKey?: string    // e.g. 'ANTHROPIC_AUTH_TOKEN'
  /** Extra env vars emitted in Claude Code snippets. */
  claudeCodeExtraEnv?: Record<string, string>
  /** VSCode settings.json env block extras. */
  claudeCodeVscodeExtraEnv?: Record<string, string>

  // --- Gemini CLI env vars ---
  geminiCliEnvBaseUrl?: string
  geminiCliEnvApiKey?: string
  geminiCliDefaultModel?: string

  // --- Opencode ---
  opencodeConfigs: OpencodeProviderConfig[]

  // --- Codex CLI ---
  codexModel?: string
  codexWireApi?: string

  // --- i18n keys ---
  descriptionKey: string
  noteKey: string
  /** Per-client-tab overrides for description / note i18n keys. */
  descriptionKeyOverrides?: Record<string, string>
  noteKeyOverrides?: Record<string, string>
  /** Per-shell-tab overrides for note i18n keys. */
  noteShellOverrides?: Record<string, string>
}

export interface OpencodeProviderConfig {
  /** Provider id used as JSON key. */
  providerId: string
  /** npm package (e.g. '@ai-sdk/anthropic'). */
  npm?: string
  /** Display name override in the provider block. */
  name?: string
  /** Models map. */
  models?: Record<string, OpencodeModelDef>
  /** Agent block (codex-style). */
  agent?: Record<string, unknown>
  /** How to compute baseUrl from site origin. 'v1' appends /v1, 'v1beta' appends /v1beta. */
  urlSuffix: string
  urlVersion: 'v1' | 'v1beta'
  /** Label shown in the file header (e.g. 'opencode.json (Claude)'). */
  pathLabel?: string
}

// ---------- builtin platform configs ----------

const anthropicConfig: PlatformCliConfig = {
  platform: 'anthropic',
  baseUrlSuffix: '',
  claudeCodeSupported: true,
  codexCliSupported: false,
  codexWsSupported: false,
  geminiCliSupported: false,
  opencodeSupported: true,
  defaultClientTab: 'claude',
  claudeCodeRequiresDispatch: false,
  claudeCodeEnvBaseUrl: 'ANTHROPIC_BASE_URL',
  claudeCodeEnvApiKey: 'ANTHROPIC_AUTH_TOKEN',
  claudeCodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1' },
  claudeCodeVscodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1', CLAUDE_CODE_ATTRIBUTION_HEADER: '0' },
  descriptionKey: 'keys.useKeyModal.description',
  noteKey: 'keys.useKeyModal.note',
  opencodeConfigs: [
    { providerId: 'anthropic', npm: '@ai-sdk/anthropic', urlSuffix: '', urlVersion: 'v1' }
  ]
}

const openaiConfig: PlatformCliConfig = {
  platform: 'openai',
  baseUrlSuffix: '',
  claudeCodeSupported: true,
  codexCliSupported: true,
  codexWsSupported: true,
  geminiCliSupported: false,
  opencodeSupported: true,
  defaultClientTab: 'codex',
  claudeCodeRequiresDispatch: true,
  claudeCodeEnvBaseUrl: 'ANTHROPIC_BASE_URL',
  claudeCodeEnvApiKey: 'ANTHROPIC_AUTH_TOKEN',
  claudeCodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1' },
  claudeCodeVscodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1', CLAUDE_CODE_ATTRIBUTION_HEADER: '0' },
  codexModel: 'gpt-5.4',
  codexWireApi: 'responses',
  descriptionKey: 'keys.useKeyModal.openai.description',
  noteKey: 'keys.useKeyModal.openai.note',
  descriptionKeyOverrides: { claude: 'keys.useKeyModal.description' },
  noteKeyOverrides: { claude: 'keys.useKeyModal.note' },
  noteShellOverrides: { windows: 'keys.useKeyModal.openai.noteWindows' },
  opencodeConfigs: [
    { providerId: 'openai', models: openaiModels, agent: openaiAgent, urlSuffix: '', urlVersion: 'v1' }
  ]
}

const geminiConfig: PlatformCliConfig = {
  platform: 'gemini',
  baseUrlSuffix: '',
  claudeCodeSupported: false,
  codexCliSupported: false,
  codexWsSupported: false,
  geminiCliSupported: true,
  opencodeSupported: true,
  defaultClientTab: 'gemini',
  claudeCodeRequiresDispatch: false,
  geminiCliEnvBaseUrl: 'GOOGLE_GEMINI_BASE_URL',
  geminiCliEnvApiKey: 'GEMINI_API_KEY',
  geminiCliDefaultModel: 'gemini-2.0-flash',
  descriptionKey: 'keys.useKeyModal.gemini.description',
  noteKey: 'keys.useKeyModal.gemini.note',
  opencodeConfigs: [
    { providerId: 'gemini', npm: '@ai-sdk/google', models: geminiModels, urlSuffix: '', urlVersion: 'v1beta' }
  ]
}

const antigravityConfig: PlatformCliConfig = {
  platform: 'antigravity',
  baseUrlSuffix: '/antigravity',
  claudeCodeSupported: true,
  codexCliSupported: false,
  codexWsSupported: false,
  geminiCliSupported: true,
  opencodeSupported: true,
  defaultClientTab: 'claude',
  claudeCodeRequiresDispatch: false,
  claudeCodeEnvBaseUrl: 'ANTHROPIC_BASE_URL',
  claudeCodeEnvApiKey: 'ANTHROPIC_AUTH_TOKEN',
  claudeCodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1' },
  claudeCodeVscodeExtraEnv: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1', CLAUDE_CODE_ATTRIBUTION_HEADER: '0' },
  geminiCliEnvBaseUrl: 'GOOGLE_GEMINI_BASE_URL',
  geminiCliEnvApiKey: 'GEMINI_API_KEY',
  geminiCliDefaultModel: 'gemini-2.0-flash',
  descriptionKey: 'keys.useKeyModal.antigravity.description',
  noteKey: 'keys.useKeyModal.antigravity.claudeNote',
  noteKeyOverrides: { gemini: 'keys.useKeyModal.antigravity.geminiNote' },
  opencodeConfigs: [
    { providerId: 'antigravity-claude', npm: '@ai-sdk/anthropic', name: 'Antigravity (Claude)', models: claudeModels, urlSuffix: '/antigravity', urlVersion: 'v1', pathLabel: 'opencode.json (Claude)' },
    { providerId: 'antigravity-gemini', npm: '@ai-sdk/google', name: 'Antigravity (Gemini)', models: antigravityGeminiModels, urlSuffix: '/antigravity', urlVersion: 'v1beta', pathLabel: 'opencode.json (Gemini)' }
  ]
}

// ---------- registry ----------

const builtinConfigs: Record<string, PlatformCliConfig> = {
  anthropic: anthropicConfig,
  openai: openaiConfig,
  gemini: geminiConfig,
  antigravity: antigravityConfig
}

/**
 * Look up the CLI config for a platform.
 * If cliConfigOverride is provided (from PlatformDeclaration.frontend_meta.cli_config),
 * it is merged over the Anthropic-compatible default.
 * Otherwise falls back to the hardcoded builtin registry.
 * Unknown platforms get an Anthropic-compatible default.
 */
export function getPlatformCliConfig(
  platform: string,
  cliConfigOverride?: Partial<PlatformCliConfig>,
): PlatformCliConfig {
  if (cliConfigOverride && Object.keys(cliConfigOverride).length > 0) {
    return { ...anthropicConfig, platform, ...cliConfigOverride }
  }
  return builtinConfigs[platform] ?? { ...anthropicConfig, platform }
}
