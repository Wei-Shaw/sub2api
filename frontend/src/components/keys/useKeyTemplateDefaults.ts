/**
 * Default templates for the "Use Key" modal.
 * Each template uses \${baseUrl} and \${apiKey} as placeholders.
 * Keys are in the format: "client/shell/file"
 */

export const useKeyTemplateDefaults: Record<string, string> = {
  // === Claude Code ===
  'claude/unix/terminal': `export ANTHROPIC_BASE_URL="\${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="\${apiKey}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`,

  'claude/unix/settings_json': `{
  "env": {
    "ANTHROPIC_BASE_URL": "\${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "\${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`,

  'claude/cmd/terminal': `set ANTHROPIC_BASE_URL=\${baseUrl}
set ANTHROPIC_AUTH_TOKEN=\${apiKey}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`,

  'claude/cmd/settings_json': `{
  "env": {
    "ANTHROPIC_BASE_URL": "\${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "\${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`,

  'claude/powershell/terminal': `$env:ANTHROPIC_BASE_URL="\${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="\${apiKey}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`,

  'claude/powershell/settings_json': `{
  "env": {
    "ANTHROPIC_BASE_URL": "\${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "\${apiKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`,

  // === Gemini CLI ===
  'gemini/unix/terminal': `export GOOGLE_GEMINI_BASE_URL="\${baseUrl}"
export GEMINI_API_KEY="\${apiKey}"
export GEMINI_MODEL="gemini-2.0-flash"  # 默认模型，可按需修改`,

  'gemini/cmd/terminal': `set GOOGLE_GEMINI_BASE_URL=\${baseUrl}
set GEMINI_API_KEY=\${apiKey}
set GEMINI_MODEL=gemini-2.0-flash`,

  'gemini/powershell/terminal': `$env:GOOGLE_GEMINI_BASE_URL="\${baseUrl}"
$env:GEMINI_API_KEY="\${apiKey}"
$env:GEMINI_MODEL="gemini-2.0-flash"  # 默认模型，可按需修改`,

  // === Codex CLI ===
  'codex/unix/config_toml': `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "\${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`,

  'codex/unix/auth_json': `{
  "OPENAI_API_KEY": "\${apiKey}"
}`,

  'codex/windows/config_toml': `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "\${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`,

  'codex/windows/auth_json': `{
  "OPENAI_API_KEY": "\${apiKey}"
}`,

  // === Codex CLI (WebSocket) ===
  'codex_ws/unix/config_toml': `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "\${baseUrl}"
wire_api = "responses"
supports_websockets = true
requires_openai_auth = true

[features]
responses_websockets_v2 = true`,

  'codex_ws/unix/auth_json': `{
  "OPENAI_API_KEY": "\${apiKey}"
}`,

  'codex_ws/windows/config_toml': `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "\${baseUrl}"
wire_api = "responses"
supports_websockets = true
requires_openai_auth = true

[features]
responses_websockets_v2 = true`,

  'codex_ws/windows/auth_json': `{
  "OPENAI_API_KEY": "\${apiKey}"
}`,

  // === OpenCode - Anthropic ===
  'opencode/anthropic/opencode_json': getOpenCodeDefaultTemplate('anthropic'),

  // === OpenCode - OpenAI ===
  'opencode/openai/opencode_json': getOpenCodeDefaultTemplate('openai'),

  // === OpenCode - Gemini ===
  'opencode/gemini/opencode_json': getOpenCodeDefaultTemplate('gemini'),

  // === OpenCode - Antigravity Claude ===
  'opencode/antigravity_claude/opencode_json': getOpenCodeDefaultTemplate('antigravity-claude'),

  // === OpenCode - Antigravity Gemini ===
  'opencode/antigravity_gemini/opencode_json': getOpenCodeDefaultTemplate('antigravity-gemini'),
}

function getOpenCodeDefaultTemplate(platform: string): string {
  const provider: Record<string, any> = {
    [platform]: {
      options: {
        baseURL: '\${baseUrl}',
        apiKey: '\${apiKey}'
      }
    }
  }

  const openaiModels = {
    'gpt-5.2': {
      name: 'GPT-5.2',
      limit: { context: 400000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'gpt-5.5': {
      name: 'GPT-5.5',
      limit: { context: 1050000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'gpt-5.4': {
      name: 'GPT-5.4',
      limit: { context: 1050000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'gpt-5.4-mini': {
      name: 'GPT-5.4 Mini',
      limit: { context: 400000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'gpt-5.3-codex-spark': {
      name: 'GPT-5.3 Codex Spark',
      limit: { context: 128000, output: 32000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'gpt-5.3-codex': {
      name: 'GPT-5.3 Codex',
      limit: { context: 400000, output: 128000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {}, xhigh: {} }
    },
    'codex-mini-latest': {
      name: 'Codex Mini',
      limit: { context: 200000, output: 100000 },
      options: { store: false },
      variants: { low: {}, medium: {}, high: {} }
    }
  }

  const geminiModels = {
    'gemini-2.0-flash': {
      name: 'Gemini 2.0 Flash',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }
    },
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }
    },
    'gemini-2.5-pro': {
      name: 'Gemini 2.5 Pro',
      limit: { context: 2097152, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3-flash-preview': {
      name: 'Gemini 3 Flash Preview',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] }
    },
    'gemini-3-pro-preview': {
      name: 'Gemini 3 Pro Preview',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3.1-pro-preview': {
      name: 'Gemini 3.1 Pro Preview',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    }
  }

  const antigravityGeminiModels = {
    'gemini-2.5-flash': {
      name: 'Gemini 2.5 Flash',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'disable' } }
    },
    'gemini-2.5-flash-lite': {
      name: 'Gemini 2.5 Flash Lite',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-2.5-flash-thinking': {
      name: 'Gemini 2.5 Flash (Thinking)',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3-flash': {
      name: 'Gemini 3 Flash',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3.1-pro-low': {
      name: 'Gemini 3.1 Pro Low',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3.1-pro-high': {
      name: 'Gemini 3.1 Pro High',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-2.5-flash-image': {
      name: 'Gemini 2.5 Flash Image',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image'], output: ['image'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'gemini-3.1-flash-image': {
      name: 'Gemini 3.1 Flash Image',
      limit: { context: 1048576, output: 65536 },
      modalities: { input: ['text', 'image'], output: ['image'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    }
  }

  const claudeModels = {
    'claude-opus-4-6-thinking': {
      name: 'Claude 4.6 Opus (Thinking)',
      limit: { context: 200000, output: 128000 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    },
    'claude-sonnet-4-6': {
      name: 'Claude 4.6 Sonnet',
      limit: { context: 200000, output: 64000 },
      modalities: { input: ['text', 'image', 'pdf'], output: ['text'] },
      options: { thinking: { budgetTokens: 24576, type: 'enabled' } }
    }
  }

  if (platform === 'gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].models = geminiModels
  } else if (platform === 'anthropic') {
    provider[platform].npm = '@ai-sdk/anthropic'
  } else if (platform === 'antigravity-claude') {
    provider[platform].npm = '@ai-sdk/anthropic'
    provider[platform].name = 'Antigravity (Claude)'
    provider[platform].models = claudeModels
  } else if (platform === 'antigravity-gemini') {
    provider[platform].npm = '@ai-sdk/google'
    provider[platform].name = 'Antigravity (Gemini)'
    provider[platform].models = antigravityGeminiModels
  } else if (platform === 'openai') {
    provider[platform].models = openaiModels
  }

  const agent =
    platform === 'openai'
      ? {
          build: { options: { store: false } },
          plan: { options: { store: false } }
        }
      : undefined

  return JSON.stringify(
    {
      provider,
      ...(agent ? { agent } : {}),
      $schema: 'https://opencode.ai/config.json'
    },
    null,
    2
  )
}

/**
 * UI selector definitions
 */
export interface TemplateSelectorOption {
  id: string
  label: string
}

export const CLIENT_OPTIONS: TemplateSelectorOption[] = [
  { id: 'claude', label: 'Claude Code' },
  { id: 'gemini', label: 'Gemini CLI' },
  { id: 'codex', label: 'Codex CLI' },
  { id: 'codex_ws', label: 'Codex CLI (WebSocket)' },
  { id: 'opencode', label: 'OpenCode' },
]

export const SYSTEM_OPTIONS: Record<string, TemplateSelectorOption[]> = {
  claude: [
    { id: 'unix', label: 'macOS / Linux' },
    { id: 'cmd', label: 'Windows CMD' },
    { id: 'powershell', label: 'PowerShell' },
  ],
  gemini: [
    { id: 'unix', label: 'macOS / Linux' },
    { id: 'cmd', label: 'Windows CMD' },
    { id: 'powershell', label: 'PowerShell' },
  ],
  codex: [
    { id: 'unix', label: 'macOS / Linux' },
    { id: 'windows', label: 'Windows' },
  ],
  codex_ws: [
    { id: 'unix', label: 'macOS / Linux' },
    { id: 'windows', label: 'Windows' },
  ],
  opencode: [
    { id: 'anthropic', label: 'Anthropic' },
    { id: 'openai', label: 'OpenAI' },
    { id: 'gemini', label: 'Gemini' },
    { id: 'antigravity_claude', label: 'Antigravity (Claude)' },
    { id: 'antigravity_gemini', label: 'Antigravity (Gemini)' },
  ],
}

export const FILE_OPTIONS: Record<string, Record<string, TemplateSelectorOption[]>> = {
  claude: {
    unix: [
      { id: 'terminal', label: 'Terminal' },
      { id: 'settings_json', label: '~/.claude/settings.json' },
    ],
    cmd: [
      { id: 'terminal', label: 'Command Prompt' },
      { id: 'settings_json', label: '%userprofile%\\.claude\\settings.json' },
    ],
    powershell: [
      { id: 'terminal', label: 'PowerShell' },
      { id: 'settings_json', label: '%userprofile%\\.claude\\settings.json' },
    ],
  },
  gemini: {
    unix: [{ id: 'terminal', label: 'Terminal' }],
    cmd: [{ id: 'terminal', label: 'Command Prompt' }],
    powershell: [{ id: 'terminal', label: 'PowerShell' }],
  },
  codex: {
    unix: [
      { id: 'config_toml', label: '~/.codex/config.toml' },
      { id: 'auth_json', label: '~/.codex/auth.json' },
    ],
    windows: [
      { id: 'config_toml', label: '%userprofile%\\.codex\\config.toml' },
      { id: 'auth_json', label: '%userprofile%\\.codex\\auth.json' },
    ],
  },
  codex_ws: {
    unix: [
      { id: 'config_toml', label: '~/.codex/config.toml' },
      { id: 'auth_json', label: '~/.codex/auth.json' },
    ],
    windows: [
      { id: 'config_toml', label: '%userprofile%\\.codex\\config.toml' },
      { id: 'auth_json', label: '%userprofile%\\.codex\\auth.json' },
    ],
  },
  opencode: {
    anthropic: [{ id: 'opencode_json', label: 'opencode.json' }],
    openai: [{ id: 'opencode_json', label: 'opencode.json' }],
    gemini: [{ id: 'opencode_json', label: 'opencode.json' }],
    antigravity_claude: [{ id: 'opencode_json', label: 'opencode.json (Claude)' }],
    antigravity_gemini: [{ id: 'opencode_json', label: 'opencode.json (Gemini)' }],
  },
}

/**
 * Get the storage key for a template.
 * Format: "client/system/file"
 */
export function getTemplateKey(client: string, system: string, file: string): string {
  return `${client}/${system}/${file}`
}

/**
 * Get the default template content for a given key.
 */
export function getDefaultTemplate(key: string): string | undefined {
  return useKeyTemplateDefaults[key]
}

/**
 * Render a template string by replacing placeholders.
 */
export function renderTemplate(template: string, baseUrl: string, apiKey: string): string {
  return template
    .replace(/\$\{baseUrl\}/g, baseUrl)
    .replace(/\$\{apiKey\}/g, apiKey)
}

/**
 * Parse custom templates from JSON string stored in settings.
 */
export function parseCustomTemplates(jsonStr: string): Record<string, string> {
  if (!jsonStr) return {}
  try {
    const parsed = JSON.parse(jsonStr)
    if (typeof parsed === 'object' && parsed !== null) {
      return parsed as Record<string, string>
    }
  } catch {
    // ignore parse errors
  }
  return {}
}

/**
 * Serialize custom templates to JSON string for storage.
 */
export function serializeCustomTemplates(templates: Record<string, string>): string {
  return JSON.stringify(templates)
}

/**
 * Get all template keys that exist in the defaults.
 */
export function getAllTemplateKeys(): string[] {
  return Object.keys(useKeyTemplateDefaults)
}

/**
 * Get human-readable label for a template key.
 */
export function getTemplateLabel(key: string): string {
  const parts = key.split('/')
  if (parts.length !== 3) return key
  const [client, system, file] = parts
  const clientLabel = CLIENT_OPTIONS.find(c => c.id === client)?.label || client
  const systemLabel = SYSTEM_OPTIONS[client]?.find(s => s.id === system)?.label || system
  const fileLabel = FILE_OPTIONS[client]?.[system]?.find(f => f.id === file)?.label || file
  return `${clientLabel} / ${systemLabel} / ${fileLabel}`
}
