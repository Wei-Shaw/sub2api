import { describe, expect, it } from 'vitest'
import { getCodexDefaultModel } from '@/constants/codexConfig'
import {
  CC_SWITCH_USAGE_SCRIPT,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it('omits the model parameter for unconfigured OpenAI imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it.each([
    ['openai', 'custom-openai-model'],
    ['grok', 'custom-grok-model']
  ] as const)('uses the configured Codex default model for %s imports', (platform, model) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType: 'claude',
        codexConfigDefaultModel: `  ${model}  `
      })
    )

    expect(params.get('model')).toBe(model)
    if (platform === 'openai') {
      const payload = JSON.parse(atob(params.get('config') || '')) as { config: string }
      expect(payload.config).toContain(`model = ${JSON.stringify(model)}\nweb_search = "live"`)
    }
  })

  it.each(['openai', 'grok'] as const)(
    'uses the shared %s default model for blank configuration',
    (platform) => {
      const params = paramsFromDeeplink(
        buildCcSwitchImportDeeplink({
          ...baseInput,
          platform,
          clientType: 'claude',
          codexConfigDefaultModel: '  '
        })
      )

      expect(params.get('model')).toBe(getCodexDefaultModel(platform) || null)
    }
  )

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
    expect(params.has('model')).toBe(false)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
    const payload = JSON.parse(atob(params.get('config') || '')) as { config: string }
    expect(params.get('configFormat')).toBe('json')
    expect(payload.config).toContain(`base_url = "${endpoint}"`)
    expect(payload.config).not.toMatch(/^model\s*=/m)
    expect(payload.config.split('[model_providers.')[0]).toContain('web_search = "live"')
    expect(payload.config).toContain('requires_openai_auth = false')
    expect(payload.config).toContain('experimental_bearer_token = "sk-test"')
    expect(payload.config).toContain('"x-openai-actor-authorization" = "local-image-extension"')
    // Global feature choices are preserved by CC Switch, not imported per provider.
    expect(payload.config).not.toContain('[features]')
  })

  it('encodes embedded Codex configuration as UTF-8 and escapes TOML strings', () => {
    const apiKey = 'sk-"test\\key'
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      apiKey,
      baseUrl: 'https://example.com/网关',
      platform: 'openai',
      clientType: 'claude'
    }))
    const bytes = Uint8Array.from(atob(params.get('config') || ''), (char) => char.charCodeAt(0))
    const payload = JSON.parse(new TextDecoder().decode(bytes)) as { config: string }
    expect(payload.config).toContain('base_url = "https://example.com/网关"')
    expect(payload.config).toContain(`experimental_bearer_token = ${JSON.stringify(apiKey)}`)
    expect(params.get('apiKey')).toBe(apiKey)
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
    expect(params.get('model')).toBe(getCodexDefaultModel('grok'))
    expect(params.has('config')).toBe(false)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType,
        codexConfigDefaultModel: 'configured-model'
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
    expect(params.has('config')).toBe(false)
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
    expect(params.has('config')).toBe(false)
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
