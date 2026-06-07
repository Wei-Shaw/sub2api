import { describe, expect, it } from 'vitest'
import {
  buildAPIKeyConfigScript,
  isConfigScriptClientAvailable
} from '../configScriptDownload'

function decodeBatchPayload(content: string): string {
  const encoded = content.match(/FromBase64String\('([^']+)'\)/)?.[1]
  if (!encoded) throw new Error('missing encoded payload')
  const binary = atob(encoded)
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

function extractClaudeBatchPayload(content: string): string {
  const marker = '__LOOK2EYE_CLAUDE_CODE_PS1__'
  const markerIndex = content.lastIndexOf(marker)
  if (markerIndex < 0) throw new Error('missing Claude Code payload marker')
  return content.slice(markerIndex + marker.length).trimStart()
}

function extractOpenCodeBatchPayload(content: string): string {
  const marker = '__LOOK2EYE_OPENCODE_PS1__'
  const markerIndex = content.lastIndexOf(marker)
  if (markerIndex < 0) throw new Error('missing OpenCode payload marker')
  return content.slice(markerIndex + marker.length).trimStart()
}

describe('configScriptDownload', () => {
  it('builds a macOS Codex shell script with look2eye config values', () => {
    const script = buildAPIKeyConfigScript({
      client: 'codex',
      os: 'mac',
      platform: 'openai',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-test-key'
    })

    expect(script.filename).toBe('look2eye-codex-config.sh')
    expect(script.content).toContain('#!/usr/bin/env sh')
    expect(script.content).toContain('look2eye Codex CLI 一键配置')
    expect(script.content).toContain("BASE_URL='https://api.look2eye.com'")
    expect(script.content).toContain("API_KEY='sk-test-key'")
    expect(script.content).toContain('CONFIG_PATH="$CODEX_DIR/config.toml"')
    expect(script.content).toContain('AUTH_PATH="$CODEX_DIR/auth.json"')
    expect(script.content).toContain('BACKUP_DIR="$CODEX_DIR/backups"')
    expect(script.content).toContain('restore|--restore|/restore')
    expect(script.content).toContain('base_url = "{{LOOK2EYE_BASE_URL}}"')
    expect(script.content).toContain('json.dumps({"OPENAI_API_KEY": api_key}')
    expect(script.content).toContain('stop_codex_processes')
  })

  it('builds a Windows Codex batch script with embedded setup payload', () => {
    const script = buildAPIKeyConfigScript({
      client: 'codex',
      os: 'win',
      platform: 'openai',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-win-key',
      siteName: 'Look2eye'
    })

    expect(script.filename).toBe('Look2eye-codex-config.bat')
    expect(script.content).toContain('@echo off')
    expect(script.content).toContain('LOOK2EYE_CODEX_EXIT')

    const payload = decodeBatchPayload(script.content)
    expect(payload).toContain("$BaseUrl = 'https://api.look2eye.com'")
    expect(payload).toContain("$ApiKey = 'sk-win-key'")
    expect(payload).toContain('$ConfigTomlTemplate')
    expect(payload).toContain('model_provider = "Look2eye"')
    expect(payload).toContain('Restore-CodexBackup')
    expect(payload).toContain('Stop-CodexProcesses')
  })

  it('builds a macOS Claude Code shell script that merges settings.json', () => {
    const script = buildAPIKeyConfigScript({
      client: 'claude',
      os: 'mac',
      platform: 'anthropic',
      baseUrl: 'https://api.example.com/v1/',
      apiKey: 'sk-claude-key',
      siteName: 'Look2eye'
    })

    expect(script.filename).toBe('Look2eye-claude-code-config.sh')
    expect(script.content).toContain('#!/usr/bin/env sh')
    expect(script.content).toContain('Look2eye Claude Code 配置已完成')
    expect(script.content).toContain("BASE_URL='https://api.look2eye.com'")
    expect(script.content).toContain("API_KEY='sk-claude-key'")
    expect(script.content).toContain('CLAUDE_DIR="$HOME/.claude"')
    expect(script.content).toContain('SETTINGS_PATH="$CLAUDE_DIR/settings.json"')
    expect(script.content).toContain('BACKUP_DIR="$CLAUDE_DIR/backups"')
    expect(script.content).toContain('settings = json.loads')
    expect(script.content).toContain('"ANTHROPIC_BASE_URL": base_url')
    expect(script.content).toContain('"ANTHROPIC_AUTH_TOKEN": api_key')
    expect(script.content).toContain('"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"')
    expect(script.content).toContain('"CLAUDE_CODE_ATTRIBUTION_HEADER": "0"')
    expect(script.content).not.toContain('.look2eye/claude-code.env')
  })

  it('builds a Windows Claude Code batch script from the marker-based template', () => {
    const script = buildAPIKeyConfigScript({
      client: 'claude',
      os: 'win',
      platform: 'antigravity',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-claude-key'
    })

    expect(script.filename).toBe('look2eye-claude-code-config.bat')
    expect(script.content).toContain('@echo off')
    expect(script.content).toContain('LOOK2EYE_SETUP_MARKER=__LOOK2EYE_CLAUDE_CODE_PS1__')
    expect(script.content).toContain('__LOOK2EYE_CLAUDE_CODE_PS1__')
    expect(script.content).toContain('LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER')

    const payload = extractClaudeBatchPayload(script.content)
    expect(payload).toContain("Applying $SiteName Claude Code config")
    expect(payload).toContain('https://api.look2eye.com')
    expect(payload).toContain('sk-claude-key')
    expect(payload).toContain('ConvertTo-OrderedMap')
    expect(payload).toContain('Join-Path $targetHome ".claude"')
    expect(payload).toContain('Join-Path $claudeDir "settings.json"')
    expect(payload).toContain('Join-Path $claudeDir "backups"')
    expect(payload).toContain('$envMap["ANTHROPIC_AUTH_TOKEN"] = $ApiKey')
    expect(payload).toContain('$envMap["CLAUDE_CODE_ATTRIBUTION_HEADER"] = "0"')
    expect(payload).toContain('Write-Utf8NoBom')
  })

  it('builds a macOS OpenCode shell script that merges opencode.json', () => {
    const script = buildAPIKeyConfigScript({
      client: 'opencode',
      os: 'mac',
      platform: 'openai',
      baseUrl: 'https://api.example.com/v1/',
      apiKey: 'sk-opencode-key',
      siteName: 'Look2eye'
    })

    expect(script.filename).toBe('Look2eye-opencode-config.sh')
    expect(script.content).toContain('#!/usr/bin/env sh')
    expect(script.content).toContain('Look2eye OpenCode 配置已完成')
    expect(script.content).toContain("BASE_URL='https://api.look2eye.com'")
    expect(script.content).toContain("API_KEY='sk-opencode-key'")
    expect(script.content).toContain('OPENCODE_DIR="$HOME/.config/opencode"')
    expect(script.content).toContain('CONFIG_PATH="$OPENCODE_DIR/opencode.json"')
    expect(script.content).toContain('BACKUP_DIR="$OPENCODE_DIR/backups"')
    expect(script.content).toContain('default_models = json.loads')
    expect(script.content).toContain('"gpt-5.5"')
    expect(script.content).toContain('options["baseURL"] = base_url')
    expect(script.content).toContain('options["apiKey"] = api_key')
    expect(script.content).toContain('opts["store"] = False')
    expect(script.content).not.toContain('Installing Look2eye OpenCode configuration')
  })

  it('builds a Windows OpenCode batch script from the marker-based template', () => {
    const script = buildAPIKeyConfigScript({
      client: 'opencode',
      os: 'win',
      platform: 'gemini',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-opencode-win-key'
    })

    expect(script.filename).toBe('look2eye-opencode-config.bat')
    expect(script.content).toContain('@echo off')
    expect(script.content).toContain('LOOK2EYE_SETUP_MARKER=__LOOK2EYE_OPENCODE_PS1__')
    expect(script.content).toContain('__LOOK2EYE_OPENCODE_PS1__')
    expect(script.content).toContain('LOOK2EYE_SETUP_LAUNCHED_FROM_EXPLORER')

    const payload = extractOpenCodeBatchPayload(script.content)
    expect(payload).toContain("Applying $SiteName OpenCode config")
    expect(payload).toContain("Join-Path (Join-Path $targetHome \".config\") \"opencode\"")
    expect(payload).toContain('Join-Path $openCodeDir "opencode.json"')
    expect(payload).toContain('Join-Path $openCodeDir "backups"')
    expect(payload).toContain("$BaseUrl = 'https://api.look2eye.com'")
    expect(payload).toContain("$ApiKey = 'sk-opencode-win-key'")
    expect(payload).toContain('$ModelsJson')
    expect(payload).toContain('"gpt-5.5"')
    expect(payload).toContain('$options["baseURL"] = $BaseUrl.Trim().TrimEnd("/")')
    expect(payload).toContain('$options["apiKey"] = $ApiKey')
    expect(payload).toContain('Ensure-AgentStoreFalse')
    expect(payload).toContain('Write-Utf8NoBom')
  })

  it('allows config scripts for every grouped platform', () => {
    expect(isConfigScriptClientAvailable({ client: 'codex', platform: 'openai' })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'codex', platform: 'gemini' })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'claude', platform: 'openai' })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'claude', platform: 'openai', allowMessagesDispatch: true })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'opencode', platform: 'gemini' })).toBe(true)
  })
})
