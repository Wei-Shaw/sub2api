import { describe, expect, it } from 'vitest'
import {
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-5.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it('adds the Codex model parameter for OpenAI imports', () => {
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
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
    // Codex imports must not carry Claude tier model params.
    expect(params.has('haikuModel')).toBe(false)
    expect(params.has('opusModel')).toBe(false)
    expect(params.has('sonnetModel')).toBe(false)
  })

  it('omits Claude tier params for Anthropic imports with no mapping configured', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'anthropic',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('claude')
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
    expect(params.has('haikuModel')).toBe(false)
    expect(params.has('sonnetModel')).toBe(false)
    expect(params.has('opusModel')).toBe(false)
  })

  it('emits configured Claude tier params for Anthropic imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'anthropic',
        clientType: 'claude',
        claudeCodeModels: {
          haiku: 'glm-4.7',
          sonnet: 'glm-5.2[1m]',
          opus: 'glm-5.2[1m]'
        }
      })
    )

    expect(params.get('haikuModel')).toBe('glm-4.7')
    expect(params.get('sonnetModel')).toBe('glm-5.2[1m]')
    expect(params.get('opusModel')).toBe('glm-5.2[1m]')
  })

  it('emits only the non-empty Claude tier params', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'anthropic',
        clientType: 'claude',
        claudeCodeModels: { sonnet: 'glm-5.2[1m]' }
      })
    )

    expect(params.has('haikuModel')).toBe(false)
    expect(params.get('sonnetModel')).toBe('glm-5.2[1m]')
    expect(params.has('opusModel')).toBe(false)
  })

  it('does not add model params for Gemini imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'gemini',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
    expect(params.has('haikuModel')).toBe(false)
    expect(params.has('sonnetModel')).toBe(false)
    expect(params.has('opusModel')).toBe(false)
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
    expect(params.has('haikuModel')).toBe(false)
  })
})
