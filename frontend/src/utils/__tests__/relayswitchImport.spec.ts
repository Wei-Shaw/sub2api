import { describe, expect, it } from 'vitest'
import { buildRelaySwitchProviderImportDeeplink } from '@/utils/relayswitchImport'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

function decodePayload(encodedPayload: string): unknown {
  const base64 = encodedPayload.replace(/-/g, '+').replace(/_/g, '/')
  const padding = base64.length % 4 === 0 ? '' : '='.repeat(4 - (base64.length % 4))
  const binary = atob(base64 + padding)
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))

  return JSON.parse(new TextDecoder().decode(bytes))
}

describe('relayswitchImport utils', () => {
  it('builds a provider import deeplink with the required RelaySwitch shape', () => {
    const deeplink = buildRelaySwitchProviderImportDeeplink({
      name: 'Sub2API',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-test'
    })
    const params = paramsFromDeeplink(deeplink)

    expect(deeplink.startsWith('relay-switch://v1/import?')).toBe(true)
    expect(params.get('resource')).toBe('provider')
    expect(decodePayload(params.get('payload') || '')).toEqual({
      name: 'Sub2API',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-test'
    })
  })

  it('encodes non-ascii provider names as utf-8 base64url', () => {
    const params = paramsFromDeeplink(
      buildRelaySwitchProviderImportDeeplink({
        name: '测试站点',
        baseUrl: 'https://api.example.com',
        apiKey: 'sk-test'
      })
    )

    expect(decodePayload(params.get('payload') || '')).toEqual({
      name: '测试站点',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-test'
    })
  })

  it('uses url-safe base64 without padding', () => {
    const params = paramsFromDeeplink(
      buildRelaySwitchProviderImportDeeplink({
        name: 'Sub2API',
        baseUrl: 'https://api.example.com',
        apiKey: 'sk-test'
      })
    )
    const payload = params.get('payload') || ''

    expect(payload).toMatch(/^[A-Za-z0-9_-]+$/)
    expect(payload).not.toContain('=')
    expect(payload).not.toContain('+')
    expect(payload).not.toContain('/')
  })
})
