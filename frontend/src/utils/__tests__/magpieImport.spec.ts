import { describe, expect, it } from 'vitest'
import {
  buildMagpieImportLink,
  resolveMagpieImportEndpoints
} from '@/utils/magpieImport'
import type { GroupPlatform } from '@/types'

function paramsFromLink(link: string): URLSearchParams {
  return new URLSearchParams(link.split('#')[1] || '')
}

describe('magpieImport utils', () => {
  const baseUrl = 'https://api.example.com/'

  it.each([
    { platform: 'anthropic' as GroupPlatform, expected: { anthropic: 'https://api.example.com' } },
    {
      platform: 'openai' as GroupPlatform,
      expected: {
        chat: 'https://api.example.com/v1',
        responses: 'https://api.example.com/v1'
      }
    },
    { platform: 'gemini' as GroupPlatform, expected: { gemini: 'https://api.example.com/v1beta' } },
    {
      platform: 'antigravity' as GroupPlatform,
      expected: {
        anthropic: 'https://api.example.com/antigravity',
        gemini: 'https://api.example.com/antigravity/v1beta'
      }
    }
  ])('maps $platform to Magpie API endpoints', ({ platform, expected }) => {
    expect(resolveMagpieImportEndpoints(platform, baseUrl)).toEqual(expected)
  })

  it('normalizes a base URL that already includes /v1', () => {
    expect(resolveMagpieImportEndpoints('openai', 'https://api.example.com/v1/')).toEqual({
      chat: 'https://api.example.com/v1',
      responses: 'https://api.example.com/v1'
    })
  })

  it('keeps the API key in the URL fragment', () => {
    const link = buildMagpieImportLink({
      baseUrl,
      platform: 'openai',
      providerName: 'Sub2API / default',
      apiKey: 'sk-test'
    })
    const params = paramsFromLink(link)

    expect(link).toMatch(/^https:\/\/usemagpie\.ai\/import#/)
    expect(params.get('name')).toBe('Sub2API / default')
    expect(params.get('key')).toBe('sk-test')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
  })
})
