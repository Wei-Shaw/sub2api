import { describe, expect, it } from 'vitest'
import {
  MAGPIE_IMPORT_URL,
  buildMagpieImportLink,
  magpieSlug,
  resolveMagpieImportEndpoints,
  resolveMagpieProviderId
} from '@/utils/magpieImport'
import type { GroupPlatform } from '@/types'

function paramsFromLink(link: string): URLSearchParams {
  const [page, fragment = ''] = link.split('#')
  expect(page).toBe(MAGPIE_IMPORT_URL)
  return new URLSearchParams(fragment)
}

const baseInput = {
  baseUrl: 'https://api.example.com',
  providerName: 'Sub2API',
  apiKey: 'sk-test'
}

describe('magpieImport utils', () => {
  it('mirrors Magpie slug derivation', () => {
    expect(magpieSlug('My Relay')).toBe('my-relay')
    expect(magpieSlug('  Acme_Relay v2 ')).toBe('acme-relay-v2')
    expect(magpieSlug('鱼鱼连线')).toBe('')
  })

  it('keeps every parameter in the fragment and out of the query', () => {
    const link = buildMagpieImportLink({ ...baseInput, platform: 'anthropic' })

    expect(link.startsWith(`${MAGPIE_IMPORT_URL}#`)).toBe(true)
    expect(new URL(link).search).toBe('')

    const params = paramsFromLink(link)
    expect(params.get('name')).toBe('Sub2API')
    expect(params.get('key')).toBe('sk-test')
  })

  it('advertises every API the gateway bridges for Anthropic groups', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'anthropic' }))

    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
    expect(params.get('catalog')).toBe('anthropic')
  })

  it('defaults to the Anthropic layout when the key has no group', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: null }))

    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('catalog')).toBe('anthropic')
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('normalizes base URL %s to one root and exactly one /v1', (baseUrl) => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, baseUrl, platform: 'openai' }))

    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
  })

  it('keeps a sub-path deployment prefix', () => {
    const endpoints = resolveMagpieImportEndpoints('openai', 'https://api.example.com/sub2api/')

    expect(endpoints.anthropic).toBe('https://api.example.com/sub2api')
    expect(endpoints.chat).toBe('https://api.example.com/sub2api/v1')
    expect(endpoints.responses).toBe('https://api.example.com/sub2api/v1')
  })

  it.each([
    { platform: 'openai' as GroupPlatform, catalog: 'openai' },
    { platform: 'grok' as GroupPlatform, catalog: 'xai' },
    { platform: 'kimi' as GroupPlatform, catalog: 'moonshotai' },
    { platform: 'zhipu' as GroupPlatform, catalog: 'zhipuai' },
    { platform: 'deepseek' as GroupPlatform, catalog: 'deepseek' },
    { platform: 'minimax' as GroupPlatform, catalog: 'minimax' },
    { platform: 'opencode_go' as GroupPlatform, catalog: 'opencode-go' }
  ])('maps $platform groups to the $catalog models.dev catalog', ({ platform, catalog }) => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform }))

    expect(params.get('catalog')).toBe(catalog)
    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
  })

  it('omits the catalog for composite groups', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'composite' }))

    expect(params.has('catalog')).toBe(false)
    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
  })

  it('skips the Responses API for Gemini groups', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'gemini' }))

    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.has('responses')).toBe(false)
    expect(params.get('catalog')).toBe('google')
  })

  it.each([
    ['https://api.example.com', 'https://api.example.com/antigravity'],
    ['https://api.example.com///', 'https://api.example.com/antigravity'],
    ['https://api.example.com/sub2api/', 'https://api.example.com/sub2api/antigravity']
  ])('points Antigravity groups at the dedicated Anthropic route for %s', (baseUrl, anthropic) => {
    const params = paramsFromLink(
      buildMagpieImportLink({ ...baseInput, baseUrl, platform: 'antigravity' })
    )

    expect(params.get('anthropic')).toBe(anthropic)
    expect(params.has('chat')).toBe(false)
    expect(params.has('responses')).toBe(false)
    expect(params.has('catalog')).toBe(false)
  })

  it('lets Magpie derive the id when the name has ASCII letters or digits', () => {
    expect(resolveMagpieProviderId('鱼鱼连线 YYLX', 'https://app.yylx.io')).toBeUndefined()

    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'anthropic' }))
    expect(params.has('id')).toBe(false)
  })

  it('falls back to the API host as id when the name would slug to nothing', () => {
    expect(resolveMagpieProviderId('鱼鱼连线', 'https://app.yylx.io/v1/')).toBe('app-yylx-io')
    expect(resolveMagpieProviderId('鱼鱼连线', 'not a url')).toBe('sub2api')

    const params = paramsFromLink(
      buildMagpieImportLink({
        ...baseInput,
        baseUrl: 'https://app.yylx.io',
        providerName: '鱼鱼连线',
        platform: 'anthropic'
      })
    )
    expect(params.get('name')).toBe('鱼鱼连线')
    expect(params.get('id')).toBe('app-yylx-io')
  })

  it('passes the site pages through when given', () => {
    const params = paramsFromLink(
      buildMagpieImportLink({
        ...baseInput,
        platform: 'anthropic',
        website: 'https://console.example.com',
        keysUrl: 'https://console.example.com/keys'
      })
    )

    expect(params.get('website')).toBe('https://console.example.com')
    expect(params.get('keys')).toBe('https://console.example.com/keys')
  })

  it('leaves the site pages out when not given', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'anthropic' }))

    expect(params.has('website')).toBe(false)
    expect(params.has('keys')).toBe(false)
  })
})
