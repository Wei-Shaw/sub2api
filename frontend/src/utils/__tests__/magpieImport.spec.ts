import { describe, expect, it } from 'vitest'
import {
  MAGPIE_IMPORT_URL,
  MAGPIE_NAME_MAX_BYTES,
  buildMagpieImportLink,
  magpieSlug,
  resolveMagpieImportEndpoints,
  resolveMagpieProviderId,
  resolveMagpieProviderName
} from '@/utils/magpieImport'
import type { GroupPlatform } from '@/types'

function paramsFromLink(link: string | null): URLSearchParams {
  expect(link).not.toBeNull()
  const [page, fragment = ''] = link!.split('#')
  expect(page).toBe(MAGPIE_IMPORT_URL)
  return new URLSearchParams(fragment)
}

const baseInput = {
  baseUrl: 'https://api.example.com',
  siteName: 'Sub2API',
  keyId: 42,
  keyName: 'laptop',
  apiKey: 'sk-test',
  allowMessagesDispatch: true
}

describe('magpieImport utils', () => {
  it('mirrors Magpie slug derivation', () => {
    expect(magpieSlug('My Relay')).toBe('my-relay')
    expect(magpieSlug('  Acme_Relay v2 ')).toBe('acme-relay-v2')
    expect(magpieSlug('鱼鱼连线')).toBe('')
  })

  it('keeps every parameter in the fragment and out of the query', () => {
    const link = buildMagpieImportLink({ ...baseInput, platform: 'anthropic' })

    expect(link!.startsWith(`${MAGPIE_IMPORT_URL}#`)).toBe(true)
    expect(new URL(link!).search).toBe('')

    const params = paramsFromLink(link)
    expect(params.get('name')).toBe('Sub2API-laptop')
    expect(params.get('id')).toBe('sub2api-42')
    expect(params.get('key')).toBe('sk-test')
  })

  it('advertises every API the gateway bridges for Anthropic groups', () => {
    const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'anthropic' }))

    expect(params.get('anthropic')).toBe('https://api.example.com')
    expect(params.get('chat')).toBe('https://api.example.com/v1')
    expect(params.get('responses')).toBe('https://api.example.com/v1')
    expect(params.get('catalog')).toBe('anthropic')
  })

  it.each([
    { platform: undefined },
    { platform: null },
    { platform: 'typesafe' as const },
    { platform: 'anthropic' as const, claudeCodeOnly: true },
    { platform: 'openai' as const, claudeCodeOnly: true }
  ])('does not import incompatible keys: %j', (options) => {
    expect(buildMagpieImportLink({ ...baseInput, ...options })).toBeNull()
  })

  it.each(['openai', 'composite'] as const)('honors Messages dispatch for %s groups', (platform) => {
    for (const allowMessagesDispatch of [undefined, false, true]) {
      const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform, allowMessagesDispatch }))
      expect(params.has('anthropic')).toBe(allowMessagesDispatch === true)
      expect(params.get('chat')).toBe('https://api.example.com/v1')
      expect(params.get('responses')).toBe('https://api.example.com/v1')
    }
  })

  it.each(['grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'] as const)(
    'retains native Messages for %s without the dispatch switch', (platform) => {
      const params = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform, allowMessagesDispatch: false }))
      expect(params.get('anthropic')).toBe('https://api.example.com')
    }
  )

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
    const endpoints = resolveMagpieImportEndpoints('openai', 'https://api.example.com/sub2api/', true)

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

  it('gives every key a provider id of its own', () => {
    const first = paramsFromLink(buildMagpieImportLink({ ...baseInput, platform: 'anthropic' }))
    const second = paramsFromLink(
      buildMagpieImportLink({ ...baseInput, keyId: 43, keyName: 'laptop', apiKey: 'sk-other', platform: 'anthropic' })
    )
    const again = paramsFromLink(
      buildMagpieImportLink({ ...baseInput, keyName: 'renamed', platform: 'openai' })
    )

    expect(first.get('id')).toBe('sub2api-42')
    expect(second.get('id')).toBe('sub2api-43')
    expect(again.get('id')).toBe('sub2api-42')
  })

  it('prefixes the provider id with the site name slug', () => {
    expect(resolveMagpieProviderId('鱼鱼连线 YYLX', 'https://app.yylx.io', 7)).toBe('yylx-7')
  })

  it('falls back to the API host as id prefix when the name would slug to nothing', () => {
    expect(resolveMagpieProviderId('鱼鱼连线', 'https://app.yylx.io/v1/', 7)).toBe('app-yylx-io-7')
    expect(resolveMagpieProviderId('鱼鱼连线', 'not a url', 7)).toBe('sub2api-7')

    const params = paramsFromLink(
      buildMagpieImportLink({
        ...baseInput,
        baseUrl: 'https://app.yylx.io',
        siteName: '鱼鱼连线',
        keyName: '工作',
        platform: 'anthropic'
      })
    )
    expect(params.get('name')).toBe('鱼鱼连线-工作')
    expect(params.get('id')).toBe('app-yylx-io-42')
  })

  it('names the provider after the site alone when the key name is blank', () => {
    expect(resolveMagpieProviderName(' Sub2API ', '  ')).toBe('Sub2API')
  })

  it('keeps the name within the bytes Magpie keeps, on a character boundary', () => {
    const name = resolveMagpieProviderName('鱼鱼连线', '很长的密钥名称'.repeat(10))
    const bytes = new TextEncoder().encode(name).length

    expect(bytes).toBeLessThanOrEqual(MAGPIE_NAME_MAX_BYTES)
    expect(bytes).toBeGreaterThan(MAGPIE_NAME_MAX_BYTES - 3)
    expect(name.startsWith('鱼鱼连线-很长的密钥名称')).toBe(true)
  })

  it('keeps a name within the limit unchanged', () => {
    expect(resolveMagpieProviderName('Sub2API', 'k'.repeat(10))).toBe(`Sub2API-${'k'.repeat(10)}`)
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
