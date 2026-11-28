import { describe, expect, it } from 'vitest'
import {
  buildAiSwitchImportDeeplink,
  isAiSwitchImportSupported,
  resolveAiSwitchImportConfig
} from '../aiSwitchImport'

describe('resolveAiSwitchImportConfig', () => {
  it('maps anthropic groups to the claude app with the root URL', () => {
    expect(resolveAiSwitchImportConfig('anthropic', 'https://gw.example.com')).toEqual({
      app: 'claude',
      endpoint: 'https://gw.example.com'
    })
  })

  it('maps openai groups to codex with a /v1 endpoint', () => {
    expect(resolveAiSwitchImportConfig('openai', 'https://gw.example.com')).toEqual({
      app: 'codex',
      endpoint: 'https://gw.example.com/v1'
    })
  })

  it('does not double the /v1 suffix', () => {
    expect(
      resolveAiSwitchImportConfig('openai', 'https://gw.example.com/v1/')?.endpoint
    ).toBe('https://gw.example.com/v1')
  })

  it('maps gemini groups to the gemini app with the root URL', () => {
    expect(resolveAiSwitchImportConfig('gemini', 'https://gw.example.com')).toEqual({
      app: 'gemini',
      endpoint: 'https://gw.example.com'
    })
  })

  it('returns null for platforms whose protocol no AI Switch app dialect matches', () => {
    for (const platform of ['grok', 'antigravity', 'composite', 'kimi', undefined, null]) {
      expect(resolveAiSwitchImportConfig(platform as never, 'https://gw.example.com')).toBeNull()
    }
  })
})

describe('isAiSwitchImportSupported', () => {
  it('accepts anthropic, openai and gemini groups only', () => {
    expect(isAiSwitchImportSupported('anthropic')).toBe(true)
    expect(isAiSwitchImportSupported('openai')).toBe(true)
    expect(isAiSwitchImportSupported('gemini')).toBe(true)
    expect(isAiSwitchImportSupported('grok')).toBe(false)
    expect(isAiSwitchImportSupported('composite')).toBe(false)
    expect(isAiSwitchImportSupported(undefined)).toBe(false)
  })
})

describe('buildAiSwitchImportDeeplink', () => {
  it('builds an aiswitch:// provider import link', () => {
    const url = buildAiSwitchImportDeeplink({
      baseUrl: 'https://gw.example.com',
      platform: 'openai',
      providerName: 'My Site',
      apiKey: 'sk-abc123'
    })
    expect(url.startsWith('aiswitch://v1/import?')).toBe(true)
    const params = new URL(url).searchParams
    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('name')).toBe('My Site')
    expect(params.get('endpoint')).toBe('https://gw.example.com/v1')
    expect(params.get('apiKey')).toBe('sk-abc123')
    expect(params.get('homepage')).toBe('https://gw.example.com')
  })

  it('throws for unsupported platforms instead of emitting a broken link', () => {
    expect(() =>
      buildAiSwitchImportDeeplink({
        baseUrl: 'https://gw.example.com',
        platform: 'grok',
        providerName: 'X',
        apiKey: 'sk-abc123'
      })
    ).toThrow()
  })
})
