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
    expect(script.content).toContain('Installing look2eye Codex CLI configuration')
    expect(script.content).toContain('base_url = "https://api.example.com/v1"')
    expect(script.content).toContain('"OPENAI_API_KEY": "sk-test-key"')
    expect(script.content).toContain('$HOME/.codex/config.toml')
  })

  it('builds a Windows Claude Code batch script with replaced API node and key', () => {
    const script = buildAPIKeyConfigScript({
      client: 'claude',
      os: 'win',
      platform: 'antigravity',
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk-claude-key'
    })

    expect(script.filename).toBe('look2eye-claude-code-config.bat')
    expect(script.content).toContain('@echo off')
    expect(script.content).toContain('Installing look2eye Claude Code configuration')

    const payload = decodeBatchPayload(script.content)
    expect(payload).toContain('Installing look2eye Claude Code configuration')
    expect(payload).toContain('https://api.example.com/antigravity/v1')
    expect(payload).toContain('sk-claude-key')
    expect(payload).toContain("ANTHROPIC_AUTH_TOKEN")
  })

  it('limits client availability by platform', () => {
    expect(isConfigScriptClientAvailable({ client: 'codex', platform: 'openai' })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'codex', platform: 'gemini' })).toBe(false)
    expect(isConfigScriptClientAvailable({ client: 'claude', platform: 'openai' })).toBe(false)
    expect(isConfigScriptClientAvailable({ client: 'claude', platform: 'openai', allowMessagesDispatch: true })).toBe(true)
    expect(isConfigScriptClientAvailable({ client: 'opencode', platform: 'gemini' })).toBe(true)
  })
})
