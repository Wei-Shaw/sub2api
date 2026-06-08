/**
 * CLI snippet generators driven by PlatformCliConfig.
 *
 * Pure functions — no Vue reactivity, no DOM.  Consumed by UseKeyModal.
 */
import type { PlatformCliConfig, OpencodeProviderConfig } from './platformCliConfigs'

// ---------- shared types ----------

export interface FileConfig {
  path: string
  content: string
  hint?: string
  highlighted?: string
}

// ---------- URL helpers ----------

function ensureV1(url: string): string {
  const trimmed = url.replace(/\/+$/, '')
  return trimmed.endsWith('/v1') ? trimmed : `${trimmed}/v1`
}

function ensureV1Beta(url: string): string {
  const trimmed = url.replace(/\/+$/, '')
  return trimmed.endsWith('/v1beta') ? trimmed : `${trimmed}/v1beta`
}

export function resolveBaseRoot(baseUrl: string): string {
  return baseUrl.replace(/\/v1\/?$/, '').replace(/\/+$/, '')
}

function resolveProviderUrl(baseRoot: string, cfg: OpencodeProviderConfig): string {
  const suffixed = `${baseRoot}${cfg.urlSuffix}`.replace(/\/+$/, '')
  return cfg.urlVersion === 'v1beta' ? ensureV1Beta(suffixed) : ensureV1(suffixed)
}

// ---------- syntax highlight tokens ----------

const esc = (v: string) => v
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&#39;')
const tok = (cls: string, v: string) => `<span class="${cls}">${esc(v)}</span>`
const kw = (v: string) => tok('text-emerald-300', v)
const vr = (v: string) => tok('text-sky-200', v)
const op = (v: string) => tok('text-slate-400', v)
const st = (v: string) => tok('text-amber-200', v)
const cm = (v: string) => tok('text-slate-500', v)

// ---------- Claude Code (Anthropic env) ----------

export function generateClaudeCodeFiles(
  cfg: PlatformCliConfig,
  baseUrl: string,
  apiKey: string,
  shell: string
): FileConfig[] {
  const envBaseUrl = cfg.claudeCodeEnvBaseUrl ?? 'ANTHROPIC_BASE_URL'
  const envApiKey = cfg.claudeCodeEnvApiKey ?? 'ANTHROPIC_AUTH_TOKEN'
  const extras = cfg.claudeCodeExtraEnv ?? {}
  const suffixedUrl = cfg.baseUrlSuffix ? `${baseUrl}${cfg.baseUrlSuffix}` : baseUrl

  let path: string
  let content: string
  switch (shell) {
    case 'unix':
      path = 'Terminal'
      content = buildExportBlock('export ', '=', '"', suffixedUrl, apiKey, envBaseUrl, envApiKey, extras)
      break
    case 'cmd':
      path = 'Command Prompt'
      content = buildExportBlock('set ', '=', '', suffixedUrl, apiKey, envBaseUrl, envApiKey, extras)
      break
    case 'powershell':
      path = 'PowerShell'
      content = buildPsBlock(suffixedUrl, apiKey, envBaseUrl, envApiKey, extras)
      break
    default:
      path = 'Terminal'
      content = ''
  }

  const vscodeExtras = cfg.claudeCodeVscodeExtraEnv ?? {}
  const vscodeSettingsPath = shell === 'unix'
    ? '~/.claude/settings.json'
    : '%userprofile%\\.claude\\settings.json'

  const envBlock: Record<string, string> = {
    [envBaseUrl]: suffixedUrl,
    [envApiKey]: apiKey,
    ...vscodeExtras
  }
  const vscodeContent = JSON.stringify({ env: envBlock }, null, 2)

  return [
    { path, content },
    { path: vscodeSettingsPath, content: vscodeContent, hint: 'VSCode Claude Code' }
  ]
}

function buildExportBlock(
  prefix: string, assign: string, quote: string,
  url: string, key: string,
  envUrl: string, envKey: string,
  extras: Record<string, string>
): string {
  const q = quote
  const lines = [
    `${prefix}${envUrl}${assign}${q}${url}${q}`,
    `${prefix}${envKey}${assign}${q}${key}${q}`
  ]
  for (const [k, v] of Object.entries(extras)) {
    lines.push(`${prefix}${k}${assign}${q}${v}${q}`)
  }
  return lines.join('\n')
}

function buildPsBlock(
  url: string, key: string,
  envUrl: string, envKey: string,
  extras: Record<string, string>
): string {
  const lines = [
    `$env:${envUrl}="${url}"`,
    `$env:${envKey}="${key}"`
  ]
  for (const [k, v] of Object.entries(extras)) {
    lines.push(`$env:${k}="${v}"`)
  }
  return lines.join('\n')
}

// ---------- Gemini CLI ----------

export function generateGeminiCliFile(
  cfg: PlatformCliConfig,
  baseUrl: string,
  apiKey: string,
  shell: string,
  modelCommentText: string
): FileConfig {
  const envBaseUrl = cfg.geminiCliEnvBaseUrl ?? 'GOOGLE_GEMINI_BASE_URL'
  const envApiKey = cfg.geminiCliEnvApiKey ?? 'GEMINI_API_KEY'
  const model = cfg.geminiCliDefaultModel ?? 'gemini-2.0-flash'
  const suffixedUrl = cfg.baseUrlSuffix ? `${baseUrl}${cfg.baseUrlSuffix}` : baseUrl

  let path: string
  let content: string
  let highlighted: string

  switch (shell) {
    case 'unix':
      path = 'Terminal'
      content = `export ${envBaseUrl}="${suffixedUrl}"\nexport ${envApiKey}="${apiKey}"\nexport GEMINI_MODEL="${model}"  # ${modelCommentText}`
      highlighted = `${kw('export')} ${vr(envBaseUrl)}${op('=')}${st(`"${suffixedUrl}"`)}\n${kw('export')} ${vr(envApiKey)}${op('=')}${st(`"${apiKey}"`)}\n${kw('export')} ${vr('GEMINI_MODEL')}${op('=')}${st(`"${model}"`)}  ${cm(`# ${modelCommentText}`)}`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set ${envBaseUrl}=${suffixedUrl}\nset ${envApiKey}=${apiKey}\nset GEMINI_MODEL=${model}`
      highlighted = `${kw('set')} ${vr(envBaseUrl)}${op('=')}${st(suffixedUrl)}\n${kw('set')} ${vr(envApiKey)}${op('=')}${st(apiKey)}\n${kw('set')} ${vr('GEMINI_MODEL')}${op('=')}${st(model)}\n${cm(`REM ${modelCommentText}`)}`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:${envBaseUrl}="${suffixedUrl}"\n$env:${envApiKey}="${apiKey}"\n$env:GEMINI_MODEL="${model}"  # ${modelCommentText}`
      highlighted = `${kw('$env:')}${vr(envBaseUrl)}${op('=')}${st(`"${suffixedUrl}"`)}\n${kw('$env:')}${vr(envApiKey)}${op('=')}${st(`"${apiKey}"`)}\n${kw('$env:')}${vr('GEMINI_MODEL')}${op('=')}${st(`"${model}"`)}  ${cm(`# ${modelCommentText}`)}`
      break
    default:
      path = 'Terminal'
      content = ''
      highlighted = ''
  }

  return { path, content, highlighted }
}

// ---------- Codex CLI ----------

export function generateCodexFiles(
  baseUrl: string,
  apiKey: string,
  shell: string,
  model: string,
  websocket: boolean,
  configHint: string
): FileConfig[] {
  const isWindows = shell === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  let configContent = `model_provider = "OpenAI"
model = "${model}"
review_model = "${model}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"${websocket ? '\nsupports_websockets = true' : ''}
requires_openai_auth = true`

  if (websocket) {
    configContent += `\n\n[features]\nresponses_websockets_v2 = true`
  }

  const authContent = `{\n  "OPENAI_API_KEY": "${apiKey}"\n}`

  return [
    { path: `${configDir}/config.toml`, content: configContent, hint: configHint },
    { path: `${configDir}/auth.json`, content: authContent }
  ]
}

// ---------- Opencode ----------

export function generateOpencodeFiles(
  cfgs: OpencodeProviderConfig[],
  baseRoot: string,
  apiKey: string,
  hintText: string
): FileConfig[] {
  return cfgs.map(cfg => {
    const url = resolveProviderUrl(baseRoot, cfg)
    const provider: Record<string, unknown> = {
      [cfg.providerId]: {
        options: { baseURL: url, apiKey },
        ...(cfg.npm ? { npm: cfg.npm } : {}),
        ...(cfg.name ? { name: cfg.name } : {}),
        ...(cfg.models ? { models: cfg.models } : {})
      }
    }
    const content = JSON.stringify(
      {
        provider,
        ...(cfg.agent ? { agent: cfg.agent } : {}),
        $schema: 'https://opencode.ai/config.json'
      },
      null,
      2
    )
    return { path: cfg.pathLabel ?? 'opencode.json', content, hint: hintText }
  })
}
