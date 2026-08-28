import { describe, expect, it } from 'vitest'
import { getUserIsolationCapability, supportsUserIsolation } from '../userIsolation'

describe('supportsUserIsolation', () => {
  it.each([
    ['anthropic', 'apikey', undefined, undefined, true],
    ['openai', 'apikey', undefined, undefined, true],
    ['deepseek', 'apikey', 'payg', 'adaptive', true],
    ['zhipu', 'apikey', 'payg', 'chat_completions', true],
    ['zhipu', 'apikey', 'coding', 'chat_completions', true],
    ['zhipu', 'apikey', 'payg', 'anthropic', true],
    ['kimi', 'apikey', 'payg', 'chat_completions', true],
    ['openai', 'oauth', undefined, undefined, true],
    ['openai', 'setup-token', undefined, undefined, true],
    ['grok', 'apikey', undefined, undefined, true],
    ['grok', 'oauth', undefined, undefined, true],
    ['anthropic', 'bedrock', undefined, undefined, false],
    ['gemini', 'apikey', undefined, undefined, false],
    ['antigravity', 'oauth', undefined, undefined, false]
  ] as const)('%s %s support is %s', (platform, type, accountMode, apiProtocol, expected) => {
    expect(supportsUserIsolation(platform, type, accountMode, apiProtocol)).toBe(expected)
  })

  it.each([
    ['openai oauth', 'openai', 'oauth', undefined, undefined, false, 'oauth'],
    ['kimi payg chat', 'kimi', 'apikey', 'payg', 'chat_completions', false, null],
    ['kimi payg anthropic', 'kimi', 'apikey', 'payg', 'anthropic', true, null],
    ['kimi coding chat', 'kimi', 'apikey', 'coding', 'chat_completions', true, 'coding_plan'],
    ['zhipu coding adaptive', 'zhipu', 'apikey', 'coding', 'adaptive', true, 'coding_plan']
  ] as const)(
    '%s exposes the expected warnings',
    (_name, platform, type, accountMode, apiProtocol, experimental, risk) => {
      expect(getUserIsolationCapability(platform, type, accountMode, apiProtocol)).toMatchObject({
        available: true,
        experimental,
        risk
      })
    }
  )
})
