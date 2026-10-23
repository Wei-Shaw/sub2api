import { describe, expect, it } from 'vitest'
import {
  CC_SWITCH_USAGE_SCRIPT,
  GROK_CC_SWITCH_MODEL,
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

describe('administrator CC Switch template presets', () => {
  const input = { baseUrl: 'https://example.com', platform: 'openai' as const, clientType: 'claude' as const, providerName: '中文分组', apiKey: 'sk-test', usageScript: '查询余额' }

  /** Decode the actual link payload, including non-ASCII site names. */
  function payload(params: URLSearchParams) {
    return JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(params.get('config')!), character => character.charCodeAt(0))))
  }

  it('exports model, review model, effort, auth and same-origin catalog as native Codex config', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, clientExportConfig: { codex_model: 'gpt-6-sol', codex_reasoning_effort: 'high', codex_auth_mode: 'api-key', codex_catalog_mode: 'remote', cc_usage_interval: '15' } }))
    const value = payload(params)
    expect(value.auth).toBeUndefined()
    expect(value.config).toContain('name = "中文分组"')
    expect(value.config).toContain('review_model = "gpt-6-sol"')
    expect(value.config).toContain('model_reasoning_effort = "high"')
    expect(value.config).toContain('requires_openai_auth = false')
    expect(value.config).toContain('experimental_bearer_token = "sk-test"')
    expect(value.config).toContain('model_catalog_url = "https://example.com/v1/models"')
    expect(value.config).not.toContain('[features]')
    expect(params.get('usageAutoInterval')).toBe('15')
    expect(new TextDecoder().decode(Uint8Array.from(atob(params.get('usageScript')!), c => c.charCodeAt(0)))).toBe('查询余额')
  })

  it('omits usage script and timer when disabled, and never exports a local file path', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, clientExportConfig: { cc_usage_mode: 'disabled', cc_usage_interval: '5', codex_catalog_mode: 'file' } }))
    expect(params.has('usageScript')).toBe(false)
    expect(params.has('usageAutoInterval')).toBe(false)
    expect(params.get('usageEnabled')).toBe('false')
    const value = payload(params)
    expect(value.auth).toEqual({ OPENAI_API_KEY: 'sk-test' })
    expect(value.config).not.toContain('model_catalog_')
  })

  it.each(['gemini', 'antigravity'] as const)('uses the Gemini slot for %s native Gemini imports', platform => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, platform, clientType: 'gemini', clientExportConfig: { gemini_model: 'gemini-2.5-pro', codex_model: 'other-client' } }))
    expect(params.get('app')).toBe('gemini')
    expect(params.get('model')).toBe('gemini-2.5-pro')
    expect(params.has('config')).toBe(false)
  })

  it('exports Claude traffic policy as an environment setting without forcing Claude role models', () => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...input, platform: 'anthropic', clientExportConfig: { claude_traffic: 'enabled' } }))
    expect(payload(params)).toEqual({ env: { CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '0' } })
    expect(params.has('model')).toBe(false)
  })
})

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.5')
  })

  it('defaults Grok Build imports to the current Grok model', () => {
    expect(GROK_CC_SWITCH_MODEL).toBe('grok-4.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it.each([
    ['https://api.example.com', 'https://api.example.com'],
    ['https://api.example.com/', 'https://api.example.com'],
    ['https://api.example.com/v1', 'https://api.example.com/v1'],
    ['https://api.example.com/v1/', 'https://api.example.com/v1']
  ])('keeps Codex imports on the configured endpoint for base URL %s', (baseUrl, endpoint) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(endpoint)
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'grok',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe(GROK_CC_SWITCH_MODEL)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
  })

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/antigravity`)
    expect(params.has('model')).toBe(false)
  })
})

describe('CC Switch usage script', () => {
  // Mirrors CC Switch: substitute the template vars as text, evaluate, read request.url.
  function usageUrlFor(baseUrl: string): string {
    const script = CC_SWITCH_USAGE_SCRIPT.split('{{baseUrl}}').join(baseUrl).split('{{apiKey}}').join('sk-test')
    // eslint-disable-next-line no-new-func
    const config = new Function(`return ${script}`)() as { request: { url: string } }
    return config.request.url
  }

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('queries exactly one /v1/usage for base URL %s', (baseUrl) => {
    expect(usageUrlFor(baseUrl)).toBe('https://api.example.com/v1/usage')
  })

  it('works against the endpoint every platform import stores', () => {
    for (const platform of ['anthropic', 'openai', 'grok', 'gemini'] as GroupPlatform[]) {
      const endpoint = paramsFromDeeplink(
        buildCcSwitchImportDeeplink({
          baseUrl: 'https://api.example.com',
          platform,
          clientType: platform === 'gemini' ? 'gemini' : 'claude',
          providerName: 'Sub2API',
          apiKey: 'sk-test',
          usageScript: CC_SWITCH_USAGE_SCRIPT
        })
      ).get('endpoint') as string
      expect(usageUrlFor(endpoint)).toBe('https://api.example.com/v1/usage')
    }
  })
})
