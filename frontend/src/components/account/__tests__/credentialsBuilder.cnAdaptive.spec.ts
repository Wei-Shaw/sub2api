import { describe, expect, it } from 'vitest'

import {
  cnProtocolSiblingBaseUrl,
  defaultCNAdaptiveBaseUrls,
  mimoTokenPlanBaseUrls,
  mimoTokenPlanRegionFromURL
} from '../credentialsBuilder'

describe('defaultCNAdaptiveBaseUrls', () => {
  it('resolves Kimi endpoints by account mode', () => {
    expect(defaultCNAdaptiveBaseUrls('kimi', 'payg')).toEqual({
      chat_completions: 'https://api.moonshot.cn/v1',
      anthropic: 'https://api.moonshot.cn/anthropic',
      responses: ''
    })
    expect(defaultCNAdaptiveBaseUrls('kimi', 'coding')).toEqual({
      chat_completions: 'https://api.kimi.com/coding/v1',
      anthropic: 'https://api.kimi.com/coding',
      responses: ''
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

  it('includes all native MiniMax and MiMo endpoints', () => {
    expect(defaultCNAdaptiveBaseUrls('minimax', 'coding')).toEqual({
      chat_completions: 'https://api.minimaxi.com/v1',
      anthropic: 'https://api.minimaxi.com/anthropic',
      responses: 'https://api.minimaxi.com/v1'
    })
    expect(defaultCNAdaptiveBaseUrls('mimo', 'payg')).toEqual({
      chat_completions: 'https://api.xiaomimimo.com/v1',
      anthropic: 'https://api.xiaomimimo.com/anthropic',
      responses: 'https://api.xiaomimimo.com/v1'
    })
  })

  it('preserves official MiMo Token Plan regions across protocols', () => {
    expect(cnProtocolSiblingBaseUrl(
      'https://token-plan-sgp.xiaomimimo.com/v1', 'mimo', 'coding', 'anthropic'
    )).toBe('https://token-plan-sgp.xiaomimimo.com/anthropic')
    expect(mimoTokenPlanBaseUrls('eu').responses).toBe('https://token-plan-ams.xiaomimimo.com/v1')
    expect(mimoTokenPlanRegionFromURL('https://token-plan-sgp.xiaomimimo.com/v1')).toBe('sgp')
  })

  it('moves the official PAYG origin to Token Plan CN when switching modes', () => {
    expect(cnProtocolSiblingBaseUrl(
      'https://api.xiaomimimo.com/v1', 'mimo', 'coding', 'chat_completions'
    )).toBe('https://token-plan-cn.xiaomimimo.com/v1')
  })

  it('preserves a custom relay origin and path prefix when switching protocols', () => {
    expect(cnProtocolSiblingBaseUrl(
      'https://relay.example.com/vendor/mimo/v1', 'mimo', 'coding', 'anthropic'
    )).toBe('https://relay.example.com/vendor/mimo/anthropic')
    expect(cnProtocolSiblingBaseUrl(
      'https://relay.example.com/vendor/mimo/anthropic', 'mimo', 'coding', 'responses'
    )).toBe('https://relay.example.com/vendor/mimo/v1')
    expect(mimoTokenPlanRegionFromURL('https://relay.example.com/v1')).toBeNull()
  })
})
