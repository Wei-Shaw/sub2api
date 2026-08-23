import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import vi from '../locales/vi'
import zh from '../locales/zh'

function collectLeafPaths(node: unknown, path = '', out = new Set<string>()): Set<string> {
  if (node && typeof node === 'object' && !Array.isArray(node)) {
    for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
      collectLeafPaths(value, path ? `${path}.${key}` : key, out)
    }
    return out
  }

  out.add(path)
  return out
}

describe('locale key parity', () => {
  it('Vietnamese contains every Chinese message key', () => {
    const zhKeys = collectLeafPaths(zh)
    const viKeys = collectLeafPaths(vi)
    const missing = [...zhKeys].filter((key) => !viKeys.has(key))

    expect(missing).toEqual([])
  })

  it('Vietnamese preserves English message placeholders', () => {
    const enMessages = collectMessages(en)
    const viMessages = collectMessages(vi)
    const missing = Object.keys(enMessages).filter((key) => !(key in viMessages))
    expect(missing).toEqual([])
    const mismatches = Object.keys(enMessages).filter(
      (key) => JSON.stringify(placeholders(enMessages[key])) !== JSON.stringify(placeholders(viMessages[key]))
    )

    expect(mismatches).toEqual([])
  })

  it('preserves required technical locale literals', () => {
    const enMessages = collectMessages(en)
    const viMessages = collectMessages(vi)
    expect(viMessages['admin.accounts.oauth.openai.codexPatPlaceholder']).toBe(enMessages['admin.accounts.oauth.openai.codexPatPlaceholder'])
    expect(viMessages['admin.settings.gatewayForwarding.openaiCodexUserAgentPlaceholder']).toBe(
      enMessages['admin.settings.gatewayForwarding.openaiCodexUserAgentPlaceholder']
    )
    expect(viMessages['admin.settings.openaiFastPolicy.modelPatternPlaceholder']).toContain('gpt-5.6-sol')
    expect(viMessages['admin.settings.openaiFastPolicy.modelPatternPlaceholder']).toContain('gpt-5.6*')
    expect(viMessages['admin.settings.openaiFastPolicy.modelWhitelistHint']).toContain('gpt-5.6*')
  })
})

function collectMessages(node: unknown, path = '', out: Record<string, string> = {}): Record<string, string> {
  if (typeof node === 'string') {
    out[path] = node
    return out
  }

  if (node && typeof node === 'object' && !Array.isArray(node)) {
    for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
      collectMessages(value, path ? `${path}.${key}` : key, out)
    }
  }

  return out
}

function placeholders(message: string | undefined): string[] {
  return typeof message === 'string' ? [...message.matchAll(/\{[^{}]+\}/g)].map((match) => match[0]).sort() : []
}
