import type { GroupPlatform } from '@/types'

export const CONFIG_SCRIPT_SITE_NAME = 'look2eye'

export type ConfigScriptClient = 'codex' | 'claude' | 'opencode'
export type ConfigScriptOS = 'mac' | 'win'

export interface ConfigScriptInput {
  client: ConfigScriptClient
  os?: ConfigScriptOS
  platform?: GroupPlatform | null
  baseUrl: string
  apiKey: string
  siteName?: string
  allowMessagesDispatch?: boolean
}

interface ConfigFile {
  path: string
  content: string
}

interface ConfigPayload {
  label: string
  files: ConfigFile[]
  env?: Record<string, string>
}

const trimTrailingSlash = (value: string) => value.replace(/\/+$/, '')

const stripV1Suffix = (value: string) => trimTrailingSlash(value).replace(/\/v1\/?$/, '')

const ensureSuffix = (value: string, suffix: string) => {
  const trimmed = trimTrailingSlash(value)
  return trimmed.endsWith(suffix) ? trimmed : `${trimmed}${suffix}`
}

const resolveBaseUrl = (baseUrl: string) => trimTrailingSlash(baseUrl || window.location.origin)

const resolveAPIBase = (baseUrl: string) => ensureSuffix(stripV1Suffix(resolveBaseUrl(baseUrl)), '/v1')

const resolveGeminiBase = (baseUrl: string) => ensureSuffix(stripV1Suffix(resolveBaseUrl(baseUrl)), '/v1beta')

const resolveAntigravityBase = (baseUrl: string) =>
  ensureSuffix(`${stripV1Suffix(resolveBaseUrl(baseUrl))}/antigravity`, '/v1')

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function json(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

function buildCodexPayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>>): ConfigPayload {
  const configToml = `# ${input.siteName} Codex CLI configuration
model_provider = "OpenAI"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${resolveBaseUrl(input.baseUrl)}"
wire_api = "responses"
requires_openai_auth = true

[features]
goals = true`

  const authJSON = json({
    OPENAI_API_KEY: input.apiKey
  })

  return {
    label: 'Codex CLI',
    files: [
      { path: '.codex/config.toml', content: configToml },
      { path: '.codex/auth.json', content: authJSON }
    ]
  }
}

function buildClaudePayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): ConfigPayload {
  const baseUrl = input.platform === 'antigravity'
    ? resolveAntigravityBase(input.baseUrl)
    : resolveBaseUrl(input.baseUrl)
  const env = {
    ANTHROPIC_BASE_URL: baseUrl,
    ANTHROPIC_AUTH_TOKEN: input.apiKey,
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    CLAUDE_CODE_ATTRIBUTION_HEADER: '0'
  }
  const envContent = `export ANTHROPIC_BASE_URL=${shellQuote(baseUrl)}
export ANTHROPIC_AUTH_TOKEN=${shellQuote(input.apiKey)}
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export CLAUDE_CODE_ATTRIBUTION_HEADER=0`
  const settingsJSON = json({
    env
  })

  return {
    label: 'Claude Code',
    env,
    files: [
      { path: '.look2eye/claude-code.env', content: `# ${input.siteName} Claude Code environment\n${envContent}\n` },
      { path: '.claude/settings.json', content: settingsJSON }
    ]
  }
}

function buildOpenCodePayload(input: Required<Pick<ConfigScriptInput, 'baseUrl' | 'apiKey' | 'siteName'>> & Pick<ConfigScriptInput, 'platform'>): ConfigPayload {
  const platform = input.platform || 'anthropic'
  const providerID = platform === 'antigravity' ? 'antigravity-claude' : platform
  const baseURL = platform === 'gemini'
    ? resolveGeminiBase(input.baseUrl)
    : platform === 'antigravity'
      ? resolveAntigravityBase(input.baseUrl)
      : resolveAPIBase(input.baseUrl)
  const model = platform === 'gemini'
    ? 'gemini-2.0-flash'
    : platform === 'openai'
      ? 'gpt-5.5'
      : 'claude-opus-4-6-thinking'

  const config = {
    $schema: 'https://opencode.ai/config.json',
    provider: {
      [providerID]: {
        npm: platform === 'gemini' ? '@ai-sdk/google' : platform === 'openai' ? '@ai-sdk/openai' : '@ai-sdk/anthropic',
        name: input.siteName,
        options: {
          baseURL,
          apiKey: input.apiKey
        },
        models: {
          [model]: {
            name: model
          }
        }
      }
    }
  }

  return {
    label: 'OpenCode',
    files: [
      { path: '.config/opencode/opencode.json', content: json(config) }
    ]
  }
}

function buildPayload(input: ConfigScriptInput): ConfigPayload {
  const base = {
    baseUrl: input.baseUrl,
    apiKey: input.apiKey,
    siteName: input.siteName || CONFIG_SCRIPT_SITE_NAME
  }

  switch (input.client) {
    case 'codex':
      return buildCodexPayload(base)
    case 'claude':
      return buildClaudePayload({ ...base, platform: input.platform })
    case 'opencode':
      return buildOpenCodePayload({ ...base, platform: input.platform })
  }
}

function buildShellScript(payload: ConfigPayload, siteName: string): string {
  const writeFileCommands = payload.files.map((file) => {
    const target = `$HOME/${file.path}`
    const dir = file.path.split('/').slice(0, -1).join('/')
    return `mkdir -p "$HOME/${dir}"
cat > "${target}" <<'LOOK2EYE_CONFIG_EOF'
${file.content}
LOOK2EYE_CONFIG_EOF`
  }).join('\n\n')

  const profileCommands = payload.files.some((file) => file.path === '.look2eye/claude-code.env')
    ? `
PROFILE="$HOME/.zshrc"
if [ -n "\${BASH_VERSION:-}" ]; then
  PROFILE="$HOME/.bashrc"
fi
touch "$PROFILE"
SOURCE_LINE='. "$HOME/.look2eye/claude-code.env"'
if ! grep -qxF "$SOURCE_LINE" "$PROFILE"; then
  printf '\\n# ${siteName} Claude Code\\n%s\\n' "$SOURCE_LINE" >> "$PROFILE"
fi`
    : ''

  return `#!/usr/bin/env sh
set -eu

echo "Installing ${siteName} ${payload.label} configuration..."

${writeFileCommands}${profileCommands}

echo "Done. Restart your terminal or source your shell profile if environment variables were updated."
`
}

function buildPowerShellScript(payload: ConfigPayload, siteName: string): string {
  const writeFileCommands = payload.files.map((file) => {
    const windowsPath = file.path.replace(/\//g, '\\')
    return `$target = Join-Path $env:USERPROFILE ${JSON.stringify(windowsPath)}
New-Item -ItemType Directory -Force -Path (Split-Path $target) | Out-Null
@'
${file.content}
'@ | Set-Content -Encoding UTF8 -Path $target`
  }).join('\n\n')

  const envCommands = payload.env
    ? Object.entries(payload.env)
      .map(([key, value]) => `[Environment]::SetEnvironmentVariable(${JSON.stringify(key)}, ${JSON.stringify(value)}, 'User')`)
      .join('\n')
    : ''

  return `$ErrorActionPreference = 'Stop'
Write-Host "Installing ${siteName} ${payload.label} configuration..."

${writeFileCommands}${envCommands ? `\n\n${envCommands}` : ''}

Write-Host "Done. Restart your terminal for environment variable changes to take effect."
`
}

function toBase64UTF8(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary)
}

function buildBatchScript(payload: ConfigPayload, siteName: string): string {
  const psScript = buildPowerShellScript(payload, siteName)
  const encoded = toBase64UTF8(psScript)
  return `@echo off
setlocal
echo Installing ${siteName} ${payload.label} configuration...
powershell -NoProfile -ExecutionPolicy Bypass -Command "$script = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('${encoded}')); $path = Join-Path $env:TEMP 'look2eye-config.ps1'; Set-Content -Encoding UTF8 -Path $path -Value $script; & $path"
if errorlevel 1 (
  echo Installation failed.
  pause
  exit /b 1
)
echo Done.
pause
`
}

export function getConfigScriptOS(): ConfigScriptOS {
  const nav = navigator as Navigator & { userAgentData?: { platform?: string } }
  const platform = nav.userAgentData?.platform || navigator.platform || ''
  return /win/i.test(platform) ? 'win' : 'mac'
}

export function buildAPIKeyConfigScript(input: ConfigScriptInput): { filename: string; content: string; os: ConfigScriptOS } {
  const os = input.os || getConfigScriptOS()
  const siteName = input.siteName || CONFIG_SCRIPT_SITE_NAME
  const payload = buildPayload({ ...input, siteName })
  const filenameClient = input.client === 'claude' ? 'claude-code' : input.client
  const extension = os === 'win' ? 'bat' : 'sh'
  const content = os === 'win' ? buildBatchScript(payload, siteName) : buildShellScript(payload, siteName)

  return {
    filename: `${siteName}-${filenameClient}-config.${extension}`,
    content,
    os
  }
}

export function isConfigScriptClientAvailable(input: Pick<ConfigScriptInput, 'client' | 'platform' | 'allowMessagesDispatch'>): boolean {
  switch (input.client) {
    case 'codex':
      return input.platform === 'openai'
    case 'claude':
      return input.platform === 'anthropic' || input.platform === 'antigravity' || (input.platform === 'openai' && input.allowMessagesDispatch === true)
    case 'opencode':
      return !!input.platform
  }
}

export function downloadAPIKeyConfigScript(input: ConfigScriptInput): void {
  const script = buildAPIKeyConfigScript(input)
  const mime = script.os === 'win' ? 'application/x-bat' : 'text/x-shellscript'
  const blob = new Blob([script.content], { type: `${mime};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = script.filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
