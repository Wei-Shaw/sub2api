import { describe, expect, it } from 'vitest'

import { cnSupportsNativeResponses, defaultCNAdaptiveBaseUrls, defaultCNBaseUrl, CN_BASE_URL_PRESETS } from '../credentialsBuilder'

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
      responses: 'https://open.bigmodel.cn/api/v1'
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


describe('Zhipu native Responses', () => {
  it('offers native Responses only for Coding Plan and uses the dedicated endpoint', () => {
    expect(cnSupportsNativeResponses('zhipu', 'coding')).toBe(true)
    expect(cnSupportsNativeResponses('zhipu', 'payg')).toBe(false)
    expect(defaultCNBaseUrl('zhipu', 'coding', 'responses')).toBe('https://open.bigmodel.cn/api/v1')
    expect(defaultCNBaseUrl('zhipu', 'payg', 'responses')).toBe('')
    expect(CN_BASE_URL_PRESETS.zhipu.filter(p => p.protocol === 'responses')).toEqual([
      { mode: 'coding', protocol: 'responses', label: 'GLM Coding Responses', url: 'https://open.bigmodel.cn/api/v1' }
    ])
  })
})
