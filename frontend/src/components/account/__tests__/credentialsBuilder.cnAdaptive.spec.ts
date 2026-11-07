import { describe, expect, it } from 'vitest'

import { cnSupportsNativeResponses, defaultCNAdaptiveBaseUrls, defaultCNBaseUrl, isMultiProtocolApiKeyPlatform } from '../credentialsBuilder'

describe('cnSupportsNativeResponses', () => {
  it('is true for DeepSeek, Kimi, and MiniMax', () => {
    expect(cnSupportsNativeResponses('deepseek')).toBe(true)
    expect(cnSupportsNativeResponses('kimi')).toBe(true)
    expect(cnSupportsNativeResponses('minimax')).toBe(true)
    expect(cnSupportsNativeResponses('zhipu')).toBe(false)
    expect(cnSupportsNativeResponses('openai')).toBe(false)
  })
})

describe('defaultCNAdaptiveBaseUrls', () => {
  it('resolves Kimi endpoints by account mode', () => {
    expect(defaultCNAdaptiveBaseUrls('kimi', 'payg')).toEqual({
      chat_completions: 'https://api.moonshot.cn/v1',
      anthropic: 'https://api.moonshot.cn/anthropic',
      responses: 'https://api.moonshot.cn/v1'
    })
    expect(defaultCNAdaptiveBaseUrls('kimi', 'coding')).toEqual({
      chat_completions: 'https://api.kimi.com/coding/v1',
      anthropic: 'https://api.kimi.com/coding',
      responses: 'https://api.kimi.com/coding/v1'
    })
  })

  it('resolves GLM endpoints by account mode', () => {
    expect(defaultCNAdaptiveBaseUrls('zhipu', 'payg')).toEqual({
      chat_completions: 'https://open.bigmodel.cn/api/paas/v4',
      anthropic: 'https://open.bigmodel.cn/api/anthropic',
      responses: ''
    })
    expect(defaultCNAdaptiveBaseUrls('zhipu', 'coding')).toEqual({
      chat_completions: 'https://open.bigmodel.cn/api/coding/paas/v4',
      anthropic: 'https://open.bigmodel.cn/api/anthropic',
      responses: ''
    })
  })

  it('includes all three native DeepSeek endpoints', () => {
    expect(defaultCNAdaptiveBaseUrls('deepseek', 'payg')).toEqual({
      chat_completions: 'https://api.deepseek.com',
      anthropic: 'https://api.deepseek.com/anthropic',
      responses: 'https://api.deepseek.com'
    })
  })

  it('uses the same MiniMax CN endpoints for payg and coding', () => {
    const expected = {
      chat_completions: 'https://api.minimaxi.com/v1',
      anthropic: 'https://api.minimaxi.com/anthropic',
      responses: 'https://api.minimaxi.com/v1'
    }
    expect(defaultCNAdaptiveBaseUrls('minimax', 'payg')).toEqual(expected)
    expect(defaultCNAdaptiveBaseUrls('minimax', 'coding')).toEqual(expected)
  })
})

describe('ollama_cloud base url presets (E11 + E12)', () => {
  it('advertises native responses support', () => {
    expect(cnSupportsNativeResponses('ollama_cloud')).toBe(true)
  })

  it('is recognized as a multi-protocol apikey platform', () => {
    expect(isMultiProtocolApiKeyPlatform('ollama_cloud')).toBe(true)
  })

  // anthropic 预设不带 /v1：出站由后端拼 /v1/messages，预设带 /v1 会拼成
  // /v1/v1/messages 静默 404（本分支早前修过的真实缺陷）。
  it.each([
    ['anthropic', 'https://ollama.com'],
    ['chat_completions', 'https://ollama.com/v1'],
    ['responses', 'https://ollama.com/v1']
  ])('resolves the %s preset without depending on account mode', (protocol, expected) => {
    expect(defaultCNBaseUrl('ollama_cloud', 'payg', protocol)).toBe(expected)
    expect(defaultCNBaseUrl('ollama_cloud', 'coding', protocol)).toBe(expected)
  })

  it('keeps the anthropic preset free of the /v1 suffix', () => {
    const anthropicPreset = defaultCNBaseUrl('ollama_cloud', 'payg', 'anthropic')
    expect(anthropicPreset).toBe('https://ollama.com')
    expect(anthropicPreset.includes('/v1')).toBe(false)
  })

  it('fills all three native endpoints for adaptive mode', () => {
    expect(defaultCNAdaptiveBaseUrls('ollama_cloud', 'payg')).toEqual({
      chat_completions: 'https://ollama.com/v1',
      anthropic: 'https://ollama.com',
      responses: 'https://ollama.com/v1'
    })
  })
})
